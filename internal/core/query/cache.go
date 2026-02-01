package query

import (
	"fmt"
	"strings"

	"otterindex/internal/core/cache"
	"otterindex/internal/model"
)

type QueryCache struct {
	lru *cache.LRU
}

func NewQueryCache(size int) *QueryCache {
	if size <= 0 {
		size = 1
	}
	return &QueryCache{lru: cache.NewLRU(size)}
}

func QueryWithCache(c *QueryCache, version int64, workspaceID string, q string, opts Options, run func() ([]model.ResultItem, error)) ([]model.ResultItem, error) {
	if run == nil {
		return nil, fmt.Errorf("run is nil")
	}
	if c == nil || c.lru == nil {
		return run()
	}

	workspaceID = strings.TrimSpace(workspaceID)
	q = strings.TrimSpace(q)

	key := makeCacheKey(workspaceID, version, q, opts)
	if v, ok := c.lru.Get(key); ok {
		if items, ok := v.([]model.ResultItem); ok {
			if ex := opts.Explain; ex != nil {
				ex.KV("cache_hit", "lru")
			}
			return cloneResultItems(items), nil
		}
	}

	if ex := opts.Explain; ex != nil {
		ex.KV("cache_hit", "miss")
	}
	items, err := run()
	if err != nil {
		return nil, err
	}
	c.lru.Put(key, cloneResultItems(items))
	return items, nil
}

func makeCacheKey(workspaceID string, version int64, q string, opts Options) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "ws=%s|ver=%d|q=%s", workspaceID, version, q)
	if storeName := strings.TrimSpace(opts.Store); storeName != "" {
		_, _ = fmt.Fprintf(&b, "|store=%s", storeName)
	}
	_, _ = fmt.Fprintf(&b, "|unit=%s|i=%t", opts.Unit, opts.CaseInsensitive)
	_, _ = fmt.Fprintf(&b, "|limit=%d|offset=%d", opts.Limit, opts.Offset)
	_, _ = fmt.Fprintf(&b, "|ctx=%d", opts.ContextLines)
	if len(opts.IncludeGlobs) > 0 {
		_, _ = fmt.Fprintf(&b, "|inc=%s", strings.Join(opts.IncludeGlobs, ","))
	}
	if len(opts.ExcludeGlobs) > 0 {
		_, _ = fmt.Fprintf(&b, "|exc=%s", strings.Join(opts.ExcludeGlobs, ","))
	}
	return b.String()
}

func cloneResultItems(items []model.ResultItem) []model.ResultItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.ResultItem, len(items))
	for i := range items {
		out[i] = items[i]
		if len(items[i].Matches) > 0 {
			out[i].Matches = make([]model.Match, len(items[i].Matches))
			copy(out[i].Matches, items[i].Matches)
		}
	}
	return out
}
