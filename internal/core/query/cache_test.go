package query

import (
	"testing"

	"otterindex/internal/model"
)

func TestQueryWithCache_Hit(t *testing.T) {
	call := 0
	f := func() ([]model.ResultItem, error) {
		call++
		return []model.ResultItem{{Path: "a.go"}}, nil
	}

	c := NewQueryCache(128)
	opts := Options{Unit: "block", Limit: 20, Offset: 0}
	ver := int64(42)

	r1, _ := QueryWithCache(c, ver, "ws1", "hello", opts, f)
	r2, _ := QueryWithCache(c, ver, "ws1", "hello", opts, f)

	if call != 1 {
		t.Fatalf("expected 1 call, got %d", call)
	}
	if len(r1) != 1 || len(r2) != 1 {
		t.Fatalf("bad results")
	}
}

