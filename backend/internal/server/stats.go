package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/waifu-im/waifu-api/backend/internal/db"
)

type userCounts struct {
	total, apiKey, jwt int64
}

type statsCollector struct {
	pool *db.Pool
	log  *slog.Logger

	mu       sync.Mutex
	daily    map[string]int64
	total    int64
	users    map[int64]*userCounts
	keysUsed map[int64]time.Time

	lastKeyWrite map[int64]time.Time

	maxUsers int
}

func newStatsCollector(pool *db.Pool, log *slog.Logger) *statsCollector {
	return &statsCollector{
		pool:         pool,
		log:          log,
		daily:        map[string]int64{},
		users:        map[int64]*userCounts{},
		keysUsed:     map[int64]time.Time{},
		lastKeyWrite: map[int64]time.Time{},
		maxUsers:     200000,
	}
}

func (c *statsCollector) record(userID int64, isAPIKey, isJwt bool, apiKeyID *int64) {
	if c == nil {
		return
	}
	now := time.Now().UTC()
	day := now.Format("2006-01-02")

	c.mu.Lock()
	defer c.mu.Unlock()
	c.daily[day]++
	c.total++
	if userID > 0 {
		uc, ok := c.users[userID]
		if !ok {
			if len(c.users) >= c.maxUsers {
				return
			}
			uc = &userCounts{}
			c.users[userID] = uc
		}
		uc.total++
		uc.apiKey += boolToInt(isAPIKey)
		uc.jwt += boolToInt(isJwt)
	}
	if apiKeyID != nil {
		if last, ok := c.lastKeyWrite[*apiKeyID]; !ok || now.Sub(last) >= time.Minute {
			c.keysUsed[*apiKeyID] = now
			c.lastKeyWrite[*apiKeyID] = now
		}
	}
}

func (c *statsCollector) swap() (map[string]int64, int64, map[int64]*userCounts, map[int64]time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	daily, total, users, keys := c.daily, c.total, c.users, c.keysUsed
	c.daily = map[string]int64{}
	c.total = 0
	c.users = map[int64]*userCounts{}
	c.keysUsed = map[int64]time.Time{}
	now := time.Now().UTC()
	for k, t := range c.lastKeyWrite {
		if now.Sub(t) > 10*time.Minute {
			delete(c.lastKeyWrite, k)
		}
	}
	return daily, total, users, keys
}

func (c *statsCollector) restore(daily map[string]int64, total int64, users map[int64]*userCounts, keys map[int64]time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for d, n := range daily {
		c.daily[d] += n
	}
	c.total += total
	for id, uc := range users {
		cur, ok := c.users[id]
		if !ok {
			if len(c.users) >= c.maxUsers {
				continue
			}
			c.users[id] = uc
			continue
		}
		cur.total += uc.total
		cur.apiKey += uc.apiKey
		cur.jwt += uc.jwt
	}
	for id, t := range keys {
		if cur, ok := c.keysUsed[id]; !ok || t.After(cur) {
			c.keysUsed[id] = t
		}
	}
}

func (c *statsCollector) flush(ctx context.Context) error {
	daily, total, users, keys := c.swap()
	if total == 0 && len(keys) == 0 {
		return nil
	}
	err := c.write(ctx, daily, total, users, keys)
	if err != nil {
		c.restore(daily, total, users, keys)
	}
	return err
}

func (c *statsCollector) write(ctx context.Context, daily map[string]int64, total int64, users map[int64]*userCounts, keys map[int64]time.Time) error {
	return pgx.BeginFunc(ctx, c.pool, func(tx pgx.Tx) error {
		for day, n := range daily {
			if _, err := tx.Exec(ctx, `
				INSERT INTO "DailyStats" ("Date","RequestCount") VALUES ($1::date, $2)
				ON CONFLICT ("Date") DO UPDATE SET "RequestCount" = "DailyStats"."RequestCount" + EXCLUDED."RequestCount"`,
				day, n); err != nil {
				return err
			}
		}
		if total > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO "GlobalStats" ("Key","Value") VALUES ('TotalRequests', $1)
				ON CONFLICT ("Key") DO UPDATE SET "Value" = "GlobalStats"."Value" + EXCLUDED."Value"`, total); err != nil {
				return err
			}
		}
		if len(users) > 0 {
			ids := make([]int64, 0, len(users))
			totals := make([]int64, 0, len(users))
			apiKeys := make([]int64, 0, len(users))
			jwts := make([]int64, 0, len(users))
			for id, uc := range users {
				ids = append(ids, id)
				totals = append(totals, uc.total)
				apiKeys = append(apiKeys, uc.apiKey)
				jwts = append(jwts, uc.jwt)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE "Users" u
				SET "RequestCount" = u."RequestCount" + v.total,
					"ApiKeyRequestCount" = u."ApiKeyRequestCount" + v.apikey,
					"JwtRequestCount" = u."JwtRequestCount" + v.jwt
				FROM unnest($1::bigint[], $2::bigint[], $3::bigint[], $4::bigint[]) AS v(id, total, apikey, jwt)
				WHERE u."Id" = v.id`, ids, totals, apiKeys, jwts); err != nil {
				return err
			}
		}
		if len(keys) > 0 {
			ids := make([]int64, 0, len(keys))
			stamps := make([]time.Time, 0, len(keys))
			for id, t := range keys {
				ids = append(ids, id)
				stamps = append(stamps, t)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE "ApiKeys" k SET "LastUsedAt" = v.ts
				FROM unnest($1::bigint[], $2::timestamptz[]) AS v(id, ts)
				WHERE k."Id" = v.id AND (k."LastUsedAt" IS NULL OR k."LastUsedAt" < v.ts)`, ids, stamps); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *statsCollector) run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := c.flush(flushCtx); err != nil {
				c.log.Warn("final stats flush failed", "error", err)
			}
			cancel()
			return
		case <-t.C:
			flushCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := c.flush(flushCtx); err != nil {
				c.log.Warn("stats flush failed", "error", err)
			}
			cancel()
		}
	}
}
