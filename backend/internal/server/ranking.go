package server

import (
	"cmp"
	"context"
	"slices"
)

type rankedIDs struct {
	ids []int64
}

func rankByUsage(ids []int64, usage *usageSnapshot) {
	slices.SortFunc(ids, func(a, b int64) int {
		if c := cmp.Compare(usage.byID[b], usage.byID[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
}

func (s *Server) rankedPage(ctx context.Context, cacheKey, idQuery string, args []any, usage *usageSnapshot, page, pageSize int) ([]int64, int64, error) {
	build := func(ctx context.Context) (*rankedIDs, int, error) {
		rows, err := s.db.Query(ctx, idQuery, args...)
		if err != nil {
			return nil, 0, err
		}
		ids, err := collectIDs(rows)
		if err != nil {
			return nil, 0, err
		}
		rankByUsage(ids, usage)
		return &rankedIDs{ids: ids}, len(ids) + 16, nil
	}
	var list *rankedIDs
	var err error
	if cacheKey != "" && s.rankedLists.enabled() {
		list, err = s.rankedLists.load(ctx, cacheKey, 0, build)
	} else {
		list, _, err = build(ctx)
	}
	if err != nil {
		return nil, 0, err
	}
	total := len(list.ids)
	offset := offsetOf(page, pageSize)
	if offset < 0 || offset >= total {
		return []int64{}, int64(total), nil
	}
	return list.ids[offset:min(offset+pageSize, total)], int64(total), nil
}

func orderByIDs[T any](ids []int64, items []T, idOf func(T) int64) []T {
	byID := make(map[int64]T, len(items))
	for _, item := range items {
		byID[idOf(item)] = item
	}
	out := make([]T, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			out = append(out, item)
		}
	}
	return out
}
