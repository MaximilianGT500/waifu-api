package server

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
)

type ipResolver struct {
	trusted []netip.Prefix
	header  string
	log     *slog.Logger
	cfHint  sync.Once
}

func newIPResolver(trusted []netip.Prefix, header string) *ipResolver {
	return &ipResolver{trusted: trusted, header: http.CanonicalHeaderKey(strings.TrimSpace(header))}
}

func (ir *ipResolver) isTrusted(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	for _, p := range ir.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (ir *ipResolver) resolve(r *http.Request) (netip.Addr, bool) {
	peer := parseHostAddr(r.RemoteAddr)
	trusted := ir.isTrusted(peer)
	ir.hintCloudflare(r, peer, trusted)
	if !trusted {
		return peer, false
	}

	values := r.Header.Values("X-Forwarded-For")
	var parts []string
	if len(values) > 0 {
		parts = strings.Split(strings.Join(values, ","), ",")
	}

	if ir.header != "" && ir.headerFromTrustedHop(parts) {
		if v := strings.TrimSpace(r.Header.Get(ir.header)); v != "" {
			if i := strings.LastIndex(v, ","); i >= 0 {
				v = v[i+1:]
			}
			if a := parseHostAddr(strings.TrimSpace(v)); a.IsValid() {
				return a, true
			}
		}
	}

	if len(parts) == 0 {
		return peer, true
	}
	client := peer
	for i := len(parts) - 1; i >= 0; i-- {
		a := parseHostAddr(strings.TrimSpace(parts[i]))
		if !a.IsValid() {
			break
		}
		client = a
		if !ir.isTrusted(a) {
			break
		}
	}
	return client, true
}

func (ir *ipResolver) headerFromTrustedHop(forwardedFor []string) bool {
	if len(forwardedFor) == 0 {
		return true
	}
	return ir.isTrusted(parseHostAddr(strings.TrimSpace(forwardedFor[len(forwardedFor)-1])))
}

func (ir *ipResolver) hintCloudflare(r *http.Request, peer netip.Addr, peerTrusted bool) {
	if ir.log == nil || r.Header.Get("Cf-Connecting-Ip") == "" {
		return
	}
	if peerTrusted && ir.header == "Cf-Connecting-Ip" {
		return
	}
	ir.cfHint.Do(func() {
		ir.log.Warn("request carries CF-Connecting-IP, but the header is not used: behind Cloudflare set "+
			"REAL_IP_HEADER=CF-Connecting-IP and add the Cloudflare ranges (or your reverse proxy) to TRUSTED_PROXIES, "+
			"otherwise visitors share the rate limit of the Cloudflare edge address", "peer", peer.String())
	})
}

func forwardedHTTPS(r *http.Request, trustedHop bool) bool {
	if r.TLS != nil {
		return true
	}
	if !trustedHop {
		return false
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.Index(proto, ","); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

func parseHostAddr(s string) netip.Addr {
	if s == "" {
		return netip.Addr{}
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap()
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
			return a.Unmap()
		}
	}
	if a, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return a.Unmap()
	}
	return netip.Addr{}
}

func rateLimitKey(a netip.Addr) string {
	return rateLimitKeyPrefix(a, 64)
}

func rateLimitKeyPrefix(a netip.Addr, bits int) string {
	if !a.IsValid() {
		return "unknown"
	}
	if a.Is6() {
		p, err := a.Prefix(bits)
		if err == nil {
			return p.String()
		}
	}
	return a.String()
}
