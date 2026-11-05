package redisx

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxBulkLen       = 1 << 20
	maxArrayLen      = 1024
	maxReplyNest     = 4
	maxLineLen       = 64 << 10
	maxReplyElements = 1024
	maxReplyBytes    = 2 << 20
)

var (
	ErrClosed   = errors.New("redisx: client closed")
	ErrNil      = errors.New("redisx: nil reply")
	errProtocol = errors.New("redisx: protocol error")
)

type Error string

func (e Error) Error() string { return string(e) }

type Reply struct {
	Kind  byte
	Str   string
	Int   int64
	Array []Reply
	Nil   bool
}

func (r Reply) Int64() (int64, error) {
	switch {
	case r.Nil:
		return 0, ErrNil
	case r.Kind == ':':
		return r.Int, nil
	case r.Kind == '$' || r.Kind == '+':
		return strconv.ParseInt(r.Str, 10, 64)
	}
	return 0, fmt.Errorf("redisx: unexpected reply type %q", r.Kind)
}

type Options struct {
	PoolSize    int
	DialTimeout time.Duration
	IdleTimeout time.Duration
}

type Client struct {
	network  string
	addr     string
	tls      *tls.Config
	username string
	password string
	db       int
	dial     time.Duration
	idleFor  time.Duration

	sem  chan struct{}
	mu   sync.Mutex
	idle []*conn
	done chan struct{}
	once sync.Once
}

type conn struct {
	nc       net.Conn
	r        *bufio.Reader
	w        *bufio.Writer
	lastUsed time.Time
}

func New(rawURL string, opts Options) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("redisx: invalid URL: %w", err)
	}
	c := &Client{network: "tcp", dial: opts.DialTimeout, idleFor: opts.IdleTimeout, done: make(chan struct{})}
	switch u.Scheme {
	case "redis", "valkey":
	case "rediss", "valkeys":
		c.tls = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	default:
		return nil, fmt.Errorf("redisx: unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("redisx: missing host")
	}
	port := u.Port()
	if port == "" {
		port = "6379"
	}
	c.addr = net.JoinHostPort(host, port)
	if u.User != nil {
		c.username = u.User.Username()
		c.password, _ = u.User.Password()
		if _, hasPass := u.User.Password(); !hasPass {
			c.password, c.username = c.username, ""
		}
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 15 {
			return nil, fmt.Errorf("redisx: invalid database %q", p)
		}
		c.db = n
	}
	if c.dial <= 0 {
		c.dial = 2 * time.Second
	}
	if c.idleFor <= 0 {
		c.idleFor = 4 * time.Minute
	}
	size := opts.PoolSize
	if size <= 0 {
		size = 16
	}
	c.sem = make(chan struct{}, size)
	return c, nil
}

func (c *Client) Addr() string { return c.addr }

func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
		c.mu.Lock()
		idle := c.idle
		c.idle = nil
		c.mu.Unlock()
		for _, cn := range idle {
			_ = cn.nc.Close()
			<-c.sem
		}
	})
}

func (c *Client) Do(ctx context.Context, args ...string) (Reply, error) {
	if len(args) == 0 {
		return Reply{}, errors.New("redisx: empty command")
	}
	for attempt := 0; ; attempt++ {
		cn, reused, err := c.get(ctx)
		if err != nil {
			return Reply{}, err
		}
		reply, err := cn.roundTrip(ctx, args)
		var serverErr Error
		if err == nil || errors.As(err, &serverErr) {
			c.put(cn)
			return reply, err
		}
		c.discard(cn)
		if !reused || attempt > 0 || ctx.Err() != nil {
			return Reply{}, err
		}
	}
}

func (c *Client) Ping(ctx context.Context) error {
	r, err := c.Do(ctx, "PING")
	if err != nil {
		return err
	}
	if r.Str != "PONG" {
		return fmt.Errorf("redisx: unexpected PING reply %q", r.Str)
	}
	return nil
}

func (c *Client) get(ctx context.Context) (*conn, bool, error) {
	select {
	case <-c.done:
		return nil, false, ErrClosed
	default:
	}
	c.mu.Lock()
	now := time.Now()
	for len(c.idle) > 0 {
		cn := c.idle[len(c.idle)-1]
		c.idle = c.idle[:len(c.idle)-1]
		if now.Sub(cn.lastUsed) < c.idleFor {
			c.mu.Unlock()
			return cn, true, nil
		}
		_ = cn.nc.Close()
		<-c.sem
	}
	c.mu.Unlock()

	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-c.done:
		return nil, false, ErrClosed
	}
	cn, err := c.connect(ctx)
	if err != nil {
		<-c.sem
		return nil, false, err
	}
	return cn, false, nil
}

func (c *Client) put(cn *conn) {
	cn.lastUsed = time.Now()
	select {
	case <-c.done:
		c.discard(cn)
		return
	default:
	}
	c.mu.Lock()
	c.idle = append(c.idle, cn)
	c.mu.Unlock()
}

func (c *Client) discard(cn *conn) {
	_ = cn.nc.Close()
	<-c.sem
}

func (c *Client) connect(ctx context.Context) (*conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, c.dial)
	defer cancel()
	var nc net.Conn
	var err error
	d := &net.Dialer{KeepAlive: 30 * time.Second}
	if c.tls != nil {
		nc, err = (&tls.Dialer{NetDialer: d, Config: c.tls}).DialContext(dialCtx, c.network, c.addr)
	} else {
		nc, err = d.DialContext(dialCtx, c.network, c.addr)
	}
	if err != nil {
		return nil, err
	}
	cn := &conn{nc: nc, r: bufio.NewReaderSize(nc, 4096), w: bufio.NewWriterSize(nc, 4096)}
	if c.password != "" {
		args := []string{"AUTH", c.password}
		if c.username != "" {
			args = []string{"AUTH", c.username, c.password}
		}
		if _, err := cn.roundTrip(dialCtx, args); err != nil {
			_ = nc.Close()
			return nil, fmt.Errorf("redisx: authentication failed: %w", err)
		}
	}
	if c.db != 0 {
		if _, err := cn.roundTrip(dialCtx, []string{"SELECT", strconv.Itoa(c.db)}); err != nil {
			_ = nc.Close()
			return nil, err
		}
	}
	return cn, nil
}

func (cn *conn) roundTrip(ctx context.Context, args []string) (Reply, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	if err := cn.nc.SetDeadline(deadline); err != nil {
		return Reply{}, err
	}
	if err := writeCommand(cn.w, args); err != nil {
		return Reply{}, err
	}
	if err := cn.w.Flush(); err != nil {
		return Reply{}, err
	}
	reply, err := readReply(cn.r, 0, &replyBudget{elements: maxReplyElements, bytes: maxReplyBytes})
	if err != nil {
		return Reply{}, err
	}
	if reply.Kind == '-' {
		return reply, Error(reply.Str)
	}
	return reply, nil
}

func writeCommand(w *bufio.Writer, args []string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n", len(a)); err != nil {
			return err
		}
		if _, err := w.WriteString(a); err != nil {
			return err
		}
		if _, err := w.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return nil
}

func readLine(r *bufio.Reader) (string, error) {
	var sb strings.Builder
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			return "", err
		}
		sb.Write(chunk)
		if sb.Len() > maxLineLen {
			return "", errProtocol
		}
		if !isPrefix {
			return sb.String(), nil
		}
	}
}

type replyBudget struct {
	elements int
	bytes    int
}

func (b *replyBudget) spend(elements, bytes int) bool {
	b.elements -= elements
	b.bytes -= bytes
	return b.elements >= 0 && b.bytes >= 0
}

func readReply(r *bufio.Reader, depth int, b *replyBudget) (Reply, error) {
	if depth > maxReplyNest {
		return Reply{}, errProtocol
	}
	line, err := readLine(r)
	if err != nil {
		return Reply{}, err
	}
	if line == "" || !b.spend(1, len(line)+2) {
		return Reply{}, errProtocol
	}
	kind, body := line[0], line[1:]
	switch kind {
	case '+', '-':
		return Reply{Kind: kind, Str: body}, nil
	case ':':
		n, err := strconv.ParseInt(body, 10, 64)
		if err != nil {
			return Reply{}, errProtocol
		}
		return Reply{Kind: kind, Int: n}, nil
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil || n < -1 || n > maxBulkLen {
			return Reply{}, errProtocol
		}
		if n == -1 {
			return Reply{Kind: kind, Nil: true}, nil
		}
		if !b.spend(0, n+2) {
			return Reply{}, errProtocol
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return Reply{}, err
		}
		if buf[n] != '\r' || buf[n+1] != '\n' {
			return Reply{}, errProtocol
		}
		return Reply{Kind: kind, Str: string(buf[:n])}, nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil || n < -1 || n > maxArrayLen {
			return Reply{}, errProtocol
		}
		if n == -1 {
			return Reply{Kind: kind, Nil: true}, nil
		}
		if n > b.elements {
			return Reply{}, errProtocol
		}
		items := make([]Reply, 0, min(n, 16))
		for i := 0; i < n; i++ {
			item, err := readReply(r, depth+1, b)
			if err != nil {
				return Reply{}, err
			}
			items = append(items, item)
		}
		return Reply{Kind: kind, Array: items}, nil
	}
	return Reply{}, errProtocol
}

type Script struct {
	src string
	sha string
}

func NewScript(src string) *Script {
	sum := sha1.Sum([]byte(src))
	return &Script{src: src, sha: hex.EncodeToString(sum[:])}
}

func (s *Script) Run(ctx context.Context, c *Client, keys []string, args ...string) (Reply, error) {
	cmd := make([]string, 0, 3+len(keys)+len(args))
	cmd = append(cmd, "EVALSHA", s.sha, strconv.Itoa(len(keys)))
	cmd = append(cmd, keys...)
	cmd = append(cmd, args...)
	reply, err := c.Do(ctx, cmd...)
	var serverErr Error
	if errors.As(err, &serverErr) && strings.HasPrefix(string(serverErr), "NOSCRIPT") {
		cmd[0], cmd[1] = "EVAL", s.src
		return c.Do(ctx, cmd...)
	}
	return reply, err
}
