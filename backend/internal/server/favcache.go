package server

import (
	"sync"
	"time"
)

const favoriteCountsMaxItems = 200_000

type favoriteCounts struct {
	ttl      time.Duration
	versions *dataVersion
	maxItems int

	mu      sync.Mutex
	stamp   uint64
	expires time.Time
	counts  map[int64]int64
}

func newFavoriteCounts(ttl time.Duration, versions *dataVersion, maxItems int) *favoriteCounts {
	return &favoriteCounts{ttl: ttl, versions: versions, maxItems: maxItems}
}

func (c *favoriteCounts) enabled() bool { return c != nil && c.ttl > 0 }

func (c *favoriteCounts) currentLocked(now time.Time) {
	version := c.versions.favorites.Load()
	if c.counts == nil || c.stamp != version || !now.Before(c.expires) || len(c.counts) > c.maxItems {
		c.counts = make(map[int64]int64, 256)
		c.stamp = version
		c.expires = now.Add(c.ttl)
	}
}

func (c *favoriteCounts) lookup(ids []int64) (map[int64]int64, []int64, uint64) {
	if !c.enabled() {
		return nil, ids, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentLocked(time.Now())
	found := make(map[int64]int64, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	var missing []int64
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if n, ok := c.counts[id]; ok {
			found[id] = n
		} else {
			missing = append(missing, id)
		}
	}
	return found, missing, c.stamp
}

func (c *favoriteCounts) store(stamp uint64, ids []int64, counts map[int64]int64) {
	if !c.enabled() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil || c.stamp != stamp || c.versions.favorites.Load() != stamp {
		return
	}
	for _, id := range ids {
		if len(c.counts) >= c.maxItems {
			return
		}
		c.counts[id] = counts[id]
	}
}
