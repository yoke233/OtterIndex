package query

import (
	"testing"

	"otterindex/internal/model"
)

func TestDedupeByPathTopN(t *testing.T) {
	in := []model.ResultItem{
		{Path: "a.go"}, {Path: "a.go"}, {Path: "a.go"},
		{Path: "b.go"},
		{Path: "a.go"},
	}
	out := DedupeByPathTopN(in, 3)
	if len(out) != 4 {
		t.Fatalf("len=%d out=%v", len(out), out)
	}
}

