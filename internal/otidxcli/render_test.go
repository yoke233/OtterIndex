package otidxcli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJSONL(t *testing.T) {
	lines := RenderJSONL([]ResultItem{
		{Path: "a.go", Range: Range{SL: 1, SC: 1, EL: 2, EC: 1}},
	})
	for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
		var v any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("invalid json: %v (%s)", err, line)
		}
	}
}

func TestRenderDefault_UsesMatchLineAndSnippet(t *testing.T) {
	s := RenderDefault([]ResultItem{
		{
			Path:    "a.go",
			Range:   Range{SL: 1, SC: 1, EL: 1, EC: 1},
			Snippet: "SNIP",
			Matches: []Match{{Line: 10, Col: 7, Text: "hello"}},
		},
	})
	if s != "a.go:10: SNIP\n" {
		t.Fatalf("RenderDefault=%q", s)
	}
}

func TestRenderVim_UsesMatchLineColAndSnippet(t *testing.T) {
	s := RenderVim([]ResultItem{
		{
			Path:    "a.go",
			Range:   Range{SL: 1, SC: 1, EL: 1, EC: 1},
			Snippet: "SNIP",
			Matches: []Match{{Line: 10, Col: 7, Text: "hello"}},
		},
	})
	if s != "a.go:10:7: SNIP\n" {
		t.Fatalf("RenderVim=%q", s)
	}
}

func TestRenderDefault_FallbackToRangeAndTitle(t *testing.T) {
	s := RenderDefault([]ResultItem{
		{
			Path:  "a.go",
			Range: Range{SL: 3, SC: 2, EL: 4, EC: 1},
			Title: "TITLE",
		},
	})
	if s != "a.go:3: TITLE\n" {
		t.Fatalf("RenderDefault=%q", s)
	}
}
