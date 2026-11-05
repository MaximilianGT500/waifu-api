package server

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

type paginated struct {
	Items           any   `json:"items"`
	PageNumber      int   `json:"pageNumber"`
	TotalPages      int   `json:"totalPages"`
	TotalCount      int64 `json:"totalCount"`
	HasPreviousPage bool  `json:"hasPreviousPage"`
	HasNextPage     bool  `json:"hasNextPage"`
	MaxPageSize     int   `json:"maxPageSize"`
	DefaultPageSize int   `json:"defaultPageSize"`
}

const maxOffset = 10_000_000

func (s *Server) effectiveMaxPageSize(configured int) int {
	hard := s.cfg.Limits.MaxPageSize
	if configured <= 0 || configured > hard {
		return hard
	}
	return configured
}

func (s *Server) paginationParams(r *http.Request, defPageSize, maxPageSize int) (page, pageSize int) {
	max := s.effectiveMaxPageSize(maxPageSize)
	if defPageSize <= 0 {
		defPageSize = 30
	}
	if defPageSize > max {
		defPageSize = max
	}
	page = parseInt(queryGet(r, "page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize = parseInt(queryGet(r, "pageSize"), defPageSize)
	if pageSize <= 0 {
		pageSize = defPageSize
	}
	if pageSize > max {
		pageSize = max
	}
	if (page - 1) > maxOffset/pageSize {
		page = maxOffset/pageSize + 1
	}
	return page, pageSize
}

func offsetOf(page, pageSize int) int { return (page - 1) * pageSize }

func (s *Server) newPaginated(items any, page, pageSize int, totalCount int64, maxPageSize, defPageSize int) paginated {
	totalPages := 0
	if pageSize > 0 {
		totalPages = int(math.Ceil(float64(totalCount) / float64(pageSize)))
	}
	return paginated{
		Items:           items,
		PageNumber:      page,
		TotalPages:      totalPages,
		TotalCount:      totalCount,
		HasPreviousPage: page > 1,
		HasNextPage:     page < totalPages,
		MaxPageSize:     s.effectiveMaxPageSize(maxPageSize),
		DefaultPageSize: defPageSize,
	}
}

func (s *Server) listParam(r *http.Request, key string, lower bool) ([]string, error) {
	raw := queryAll(r, key)
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if lower {
			v = strings.ToLower(v)
		}
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) > s.cfg.Limits.MaxFilterValues {
		return nil, fmt.Errorf("Too many values for '%s' (maximum %d).", key, s.cfg.Limits.MaxFilterValues)
	}
	return out, nil
}

func (s *Server) idListParam(r *http.Request, key string) ([]int64, error) {
	vals, err := s.listParam(r, key, false)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(vals))
	for _, v := range vals {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out, nil
}

func queryBool(r *http.Request, key string, def bool) bool {
	v := strings.TrimSpace(queryGet(r, key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}
