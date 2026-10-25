package server

import (
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

func paginationParams(r *http.Request, defPageSize, maxPageSize int) (page, pageSize int) {
	page = parseInt(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize = parseInt(r.URL.Query().Get("pageSize"), defPageSize)
	if pageSize <= 0 {
		pageSize = defPageSize
	}
	if maxPageSize > 0 && pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func newPaginated(items any, page, pageSize int, totalCount int64, maxPageSize, defPageSize int) paginated {
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
		MaxPageSize:     maxPageSize,
		DefaultPageSize: defPageSize,
	}
}

func repeatedQuery(r *http.Request, key string) []string {
	return r.URL.Query()[key]
}

func parseLongList(values []string) ([]int64, error) {
	out := make([]int64, 0, len(values))
	for _, v := range values {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func queryBool(r *http.Request, key string, def bool) bool {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
