package otidxcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderShow_PrintsLinesWithMatchMarker(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "a", "b.go")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte("1\n2\n3\n4\n5\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := RenderShow(root, []ResultItem{
		{
			Path:    "a/b.go",
			Range:   Range{SL: 2, SC: 1, EL: 4, EC: 1},
			Snippet: "SNIP",
			Matches: []Match{{Line: 3, Col: 1, Text: "3"}},
		},
	})

	if !strings.Contains(out, "a/b.go:3:1 (2-4)") {
		t.Fatalf("missing header: %s", out)
	}
	if !strings.Contains(out, "  2| 2") {
		t.Fatalf("missing line 2: %s", out)
	}
	if !strings.Contains(out, "> 3| 3") {
		t.Fatalf("missing match marker line 3: %s", out)
	}
	if !strings.Contains(out, "  4| 4") {
		t.Fatalf("missing line 4: %s", out)
	}
}

func TestAttachText_FillsTextFromRange(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "a.txt")
	if err := os.WriteFile(full, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	items := []ResultItem{
		{Path: "a.txt", Range: Range{SL: 2, SC: 1, EL: 3, EC: 1}},
	}
	AttachText(root, items)
	if items[0].Text != "b\nc" {
		t.Fatalf("Text=%q", items[0].Text)
	}
}
