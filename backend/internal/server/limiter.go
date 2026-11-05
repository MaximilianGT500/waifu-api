package server

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/redisx"
)

var gcraScript = redisx.NewScript(`
local interval = tonumber(ARGV[1])
local tolerance = tonumber(ARGV[2])
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
local tat = tonumber(redis.call('GET', KEYS[1]))
if not tat or tat < now then
  tat = now
end
local newTat = tat + interval
local allowAt = newTat - tolerance
if now < allowAt then
  return {0, allowAt - now, 0}
end
redis.call('SET', KEYS[1], newTat, 'PX', math.ceil((newTat - now) / 1000) + 1)
return {1, 0, math.floor((tolerance - (newTat - now)) / interval)}
`)

const remoteBackoff = 5 * time.Second

type rateLimiter struct {
	name      string
	local     *limiterStore
	remote    *redisx.Client
	keyPrefix string
	interval  int64
	tolerance int64
	timeout   time.Duration
	log       *slog.Logger
	downUntil atomic.Int64
	lastWarn  atomic.Int64
}

func newRateLimiter(name string, rpm, burst, maxEntries int, remote *redisx.Client, prefix string, timeout time.Duration, log *slog.Logger) *rateLimiter {
	local := newLimiterStore(name, rpm, burst, maxEntries)
	interval := int64(60_000_000 / local.rpm)
	if timeout <= 0 {
		timeout = 150 * time.Millisecond
	}
	return &rateLimiter{
		name:      name,
		local:     local,
		remote:    remote,
		keyPrefix: prefix + "rl:" + name + ":",
		interval:  interval,
		tolerance: interval * int64(local.burst),
		timeout:   timeout,
		log:       log,
	}
}

func (l *rateLimiter) limit() int { return l.local.rpm }

func (l *rateLimiter) shared() bool { return l.remote != nil }

func (l *rateLimiter) take(ctx context.Context, a netip.Addr) limitResult {
	if l.remote == nil || time.Now().UnixNano() < l.downUntil.Load() {
		return l.local.takeAddr(a, time.Now())
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.timeout)
	defer cancel()
	res, err := l.takeRemote(rctx, rateLimitKey(a))
	if err == nil {
		return res
	}
	l.downUntil.Store(time.Now().Add(remoteBackoff).UnixNano())
	if last := l.lastWarn.Load(); time.Since(time.Unix(0, last)) > time.Minute && l.lastWarn.CompareAndSwap(last, time.Now().UnixNano()) {
		l.log.Warn("shared rate limit store unavailable, limiting per instance", "limiter", l.name, "error", err)
	}
	return l.local.takeAddr(a, time.Now())
}

func (l *rateLimiter) takeRemote(ctx context.Context, key string) (limitResult, error) {
	r, err := gcraScript.Run(ctx, l.remote, []string{l.keyPrefix + key},
		strconv.FormatInt(l.interval, 10), strconv.FormatInt(l.tolerance, 10))
	if err != nil {
		return limitResult{}, err
	}
	if len(r.Array) != 3 {
		return limitResult{}, errors.New("unexpected rate limit reply")
	}
	allowed, err1 := r.Array[0].Int64()
	waitMicros, err2 := r.Array[1].Int64()
	remaining, err3 := r.Array[2].Int64()
	if err1 != nil || err2 != nil || err3 != nil {
		return limitResult{}, errors.New("unexpected rate limit reply")
	}
	if allowed == 1 {
		return limitResult{allowed: true, remaining: int(max(remaining, 0))}, nil
	}
	wait := time.Duration(waitMicros) * time.Microsecond
	if wait <= 0 {
		wait = time.Second
	}
	return limitResult{allowed: false, retryAfter: wait}, nil
}
