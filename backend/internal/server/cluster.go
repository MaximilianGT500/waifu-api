package server

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/waifu-im/waifu-api/backend/internal/db"
)

const (
	imageLockClass  = "waifu-image-files"
	leaderLockClass = "waifu-scheduler"
)

type cluster struct {
	locks       *db.Pool
	local       keyedMutex
	log         *slog.Logger
	leader      atomic.Bool
	retryLeader time.Duration
	checkLeader time.Duration
}

func newCluster(locks *db.Pool, log *slog.Logger) *cluster {
	return &cluster{locks: locks, log: log, retryLeader: 15 * time.Second, checkLeader: 10 * time.Second}
}

func (c *cluster) distributed() bool { return c.locks != nil }

type lockSlot struct {
	c    *cluster
	conn *pgxpool.Conn
	once sync.Once
}

func (c *cluster) reserve(ctx context.Context) (*lockSlot, error) {
	slot := &lockSlot{c: c}
	if c.locks == nil {
		return slot, nil
	}
	conn, err := c.locks.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	slot.conn = conn
	return slot, nil
}

func (sl *lockSlot) release() {
	sl.once.Do(func() {
		if sl.conn != nil {
			sl.conn.Release()
		}
	})
}

func (sl *lockSlot) advisory(ctx context.Context, id int64) (func(), error) {
	if sl.conn == nil {
		return sl.release, nil
	}
	if _, err := sl.conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1), ($2::bigint % 2147483647)::int)`, imageLockClass, id); err != nil {
		sl.discard()
		return nil, err
	}
	return func() {
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released bool
		err := sl.conn.QueryRow(uctx, `SELECT pg_advisory_unlock(hashtext($1), ($2::bigint % 2147483647)::int)`, imageLockClass, id).Scan(&released)
		if err != nil || !released {
			sl.discard()
			return
		}
		sl.release()
	}, nil
}

func (sl *lockSlot) discard() {
	sl.once.Do(func() {
		if sl.conn == nil {
			return
		}
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sl.conn.Conn().Close(cctx)
		sl.conn.Release()
	})
}

func (sl *lockSlot) lockNew(ctx context.Context, id int64) (func(), error) {
	unlockLocal, err := sl.c.local.lock(ctx, id)
	if err != nil {
		sl.release()
		return nil, err
	}
	unlockShared, err := sl.advisory(ctx, id)
	if err != nil {
		unlockLocal()
		return nil, err
	}
	return combineUnlock(unlockShared, unlockLocal), nil
}

func (c *cluster) lockImage(ctx context.Context, id int64) (func(), error) {
	unlockLocal, err := c.local.lock(ctx, id)
	if err != nil {
		return nil, err
	}
	slot, err := c.reserve(ctx)
	if err != nil {
		unlockLocal()
		return nil, err
	}
	unlockShared, err := slot.advisory(ctx, id)
	if err != nil {
		unlockLocal()
		return nil, err
	}
	return combineUnlock(unlockShared, unlockLocal), nil
}

func combineUnlock(first, second func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			first()
			second()
		})
	}
}

func (c *cluster) isLeader() bool { return c.locks == nil || c.leader.Load() }

func (c *cluster) runLeader(ctx context.Context, lead func(context.Context)) {
	if c.locks == nil {
		c.leader.Store(true)
		lead(ctx)
		return
	}
	for ctx.Err() == nil {
		c.lead(ctx, lead)
		t := time.NewTimer(c.retryLeader)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func (c *cluster) lead(ctx context.Context, lead func(context.Context)) {
	conn, err := c.locks.Acquire(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			c.log.Warn("leader election: no database connection", "error", err)
		}
		return
	}
	slot := &lockSlot{c: c, conn: conn}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1), 0)`, leaderLockClass).Scan(&acquired); err != nil {
		slot.discard()
		return
	}
	if !acquired {
		slot.release()
		return
	}
	c.leader.Store(true)
	c.log.Info("leading scheduled jobs")

	leadCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		lead(leadCtx)
	}()
	ticker := time.NewTicker(c.checkLeader)
	for running := true; running; {
		select {
		case <-ctx.Done():
			running = false
		case <-done:
			running = false
		case <-ticker.C:
			pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
			if err := conn.Ping(pctx); err != nil && ctx.Err() == nil {
				c.log.Warn("leadership lost", "error", err)
				running = false
			}
			pcancel()
		}
	}
	ticker.Stop()
	cancel()
	<-done
	c.leader.Store(false)

	uctx, ucancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ucancel()
	var released bool
	if err := conn.QueryRow(uctx, `SELECT pg_advisory_unlock(hashtext($1), 0)`, leaderLockClass).Scan(&released); err != nil || !released {
		slot.discard()
		return
	}
	slot.release()
}
