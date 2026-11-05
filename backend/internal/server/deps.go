package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
)

type ObjectStore interface {
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	PublicURL(key string) string
	List(ctx context.Context, fn func(key string) bool) (bool, error)
}

type DiscordAuthenticator interface {
	ExchangeCode(ctx context.Context, code string) (string, error)
	GetUserProfile(ctx context.Context, accessToken string) (*auth.DiscordUser, error)
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type cdnPurger struct {
	cfg    *config.Config
	store  ObjectStore
	log    *slog.Logger
	client *http.Client
	queue  chan []string
}

func newCDNPurger(cfg *config.Config, store ObjectStore, log *slog.Logger) *cdnPurger {
	return &cdnPurger{
		cfg:    cfg,
		store:  store,
		log:    log,
		client: &http.Client{Timeout: 30 * time.Second},
		queue:  make(chan []string, 256),
	}
}

func (p *cdnPurger) enabled() bool {
	return p != nil && p.cfg.Cloudflare.ZoneID != "" && p.cfg.Cloudflare.APIToken != ""
}

func (p *cdnPurger) purge(keys []string) {
	if !p.enabled() || len(keys) == 0 {
		return
	}
	select {
	case p.queue <- keys:
	default:
		p.log.Warn("cdn purge queue full, dropping purge", "keys", len(keys))
	}
}

func (p *cdnPurger) run(ctx context.Context) {
	sendCtx := context.WithoutCancel(ctx)
	for {
		select {
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(sendCtx, 10*time.Second)
			defer cancel()
			for {
				select {
				case keys := <-p.queue:
					if drainCtx.Err() != nil {
						p.log.Warn("cdn purge dropped at shutdown", "keys", len(keys))
						continue
					}
					p.send(drainCtx, keys)
				default:
					return
				}
			}
		case keys := <-p.queue:
			p.send(sendCtx, keys)
		}
	}
}

func (p *cdnPurger) variantURL(key string, width int) string {
	original := p.store.PublicURL(key)
	path := p.cfg.ImgTransform.Path
	u, err := url.Parse(original)
	if err != nil || path == "" {
		return ""
	}
	sep := "/"
	last := path[len(path)-1:]
	switch {
	case last == "," || last == "/":
		sep = ""
	case strings.Contains(path, "="):
		sep = ","
	}
	param := p.cfg.ImgTransform.WidthParam
	if param == "" {
		param = "width"
	}
	return u.Scheme + "://" + u.Host + path + sep + param + "=" + strconv.Itoa(width) + u.EscapedPath()
}

func (p *cdnPurger) urls(keys []string) []string {
	files := make([]string, 0, len(keys)*3)
	seen := map[string]bool{}
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			files = append(files, u)
		}
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		add(p.store.PublicURL(k))
		if p.cfg.ImgTransform.Enabled {
			add(p.variantURL(k, p.cfg.ImgTransform.CardWidth))
			add(p.variantURL(k, p.cfg.ImgTransform.PreviewWidth))
		}
	}
	return files
}

func (p *cdnPurger) send(ctx context.Context, keys []string) {
	files := p.urls(keys)
	for start := 0; start < len(files); start += 30 {
		end := start + 30
		if end > len(files) {
			end = len(files)
		}
		body, _ := json.Marshal(map[string]any{"files": files[start:end]})
		reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
			"https://api.cloudflare.com/client/v4/zones/"+url.PathEscape(p.cfg.Cloudflare.ZoneID)+"/purge_cache",
			bytes.NewReader(body))
		if err != nil {
			cancel()
			return
		}
		req.Header.Set("Authorization", "Bearer "+p.cfg.Cloudflare.APIToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			p.log.Warn("cdn purge failed", "error", err)
			cancel()
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			p.log.Warn("cdn purge rejected", "status", resp.StatusCode)
		}
		cancel()
	}
}

type objectJanitor struct {
	store   ObjectStore
	log     *slog.Logger
	queue   chan string
	backoff []time.Duration
	guard   func(ctx context.Context, key string) (release func(), inUse bool, err error)
	deleted func(key string)
}

func newObjectJanitor(store ObjectStore, log *slog.Logger) *objectJanitor {
	return &objectJanitor{
		store:   store,
		log:     log,
		queue:   make(chan string, 1024),
		backoff: []time.Duration{2 * time.Second, 10 * time.Second, time.Minute, 5 * time.Minute},
	}
}

func (j *objectJanitor) remove(ctx context.Context, key string) {
	if key == "" {
		return
	}
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	err := j.store.Delete(dctx, key)
	cancel()
	if err == nil {
		j.notifyDeleted(key)
		return
	}
	j.log.Warn("storage delete failed, retrying in the background", "key", key, "error", err)
	select {
	case j.queue <- key:
	default:
		j.log.Error("storage delete queue full; the file stays an orphan until removed via the storage diff", "key", key)
	}
}

func (j *objectJanitor) notifyDeleted(key string) {
	if j.deleted != nil {
		j.deleted(key)
	}
}

const maxPendingDeletes = 4096

type pendingDelete struct {
	key     string
	attempt int
	due     time.Time
}

func (j *objectJanitor) run(ctx context.Context) {
	var pending []pendingDelete
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		wait := time.Hour
		now := time.Now()
		for _, p := range pending {
			wait = min(wait, max(p.due.Sub(now), 0))
		}
		timer.Reset(wait)

		select {
		case <-ctx.Done():
			for {
				select {
				case key := <-j.queue:
					pending = append(pending, pendingDelete{key: key})
				default:
					for _, p := range pending {
						j.log.Error("shutdown before a storage delete succeeded; the file is listed as an orphan in the storage diff", "key", p.key)
					}
					return
				}
			}
		case key := <-j.queue:
			if len(pending) >= maxPendingDeletes {
				j.log.Error("too many failed storage deletes; the file stays an orphan until removed via the storage diff", "key", key)
				continue
			}
			pending = append(pending, pendingDelete{key: key, due: time.Now().Add(j.backoff[0])})
		case <-timer.C:
			pending = j.retryDue(ctx, pending)
		}
	}
}

func (j *objectJanitor) retryDue(ctx context.Context, pending []pendingDelete) []pendingDelete {
	now := time.Now()
	kept := pending[:0]
	postpone := false
	for _, p := range pending {
		if p.due.After(now) {
			kept = append(kept, p)
			continue
		}
		if postpone {
			p.due = now.Add(j.backoff[p.attempt])
			kept = append(kept, p)
			continue
		}
		release := func() {}
		if j.guard != nil {
			gctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			rel, inUse, err := j.guard(gctx, p.key)
			cancel()
			if err != nil {
				j.log.Warn("checking a pending storage delete failed, retrying later", "key", p.key, "error", err)
				postpone = true
				p.due = now.Add(j.backoff[p.attempt])
				kept = append(kept, p)
				continue
			}
			if inUse {
				rel()
				j.log.Info("pending storage delete skipped, the file is in use again", "key", p.key)
				continue
			}
			release = rel
		}
		dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := j.store.Delete(dctx, p.key)
		cancel()
		release()
		if err == nil {
			j.log.Info("storage delete succeeded after retry", "key", p.key)
			j.notifyDeleted(p.key)
			continue
		}
		postpone = true
		p.attempt++
		if p.attempt >= len(j.backoff) {
			j.log.Error("storage delete failed permanently; the file is listed as an orphan in the storage diff", "key", p.key, "error", err)
			continue
		}
		p.due = now.Add(j.backoff[p.attempt])
		kept = append(kept, p)
	}
	return kept
}
