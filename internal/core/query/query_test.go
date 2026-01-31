package query

import (
	"os"
	"path/filepath"
	"testing"

	"otterindex/internal/core/indexer"
)

func TestQueryReturnsRanges(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0o644)
	dbPath := filepath.Join(root, "index.db")

	if err := indexer.Build(root, dbPath, indexer.Options{}); err != nil {
		t.Fatalf("build: %v", err)
	}

	results, err := Query(dbPath, root, "hello", Options{Unit: "block", ContextLines: 1})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected results, got %d", len(results))
	}
	if results[0].Path == "" {
		t.Fatalf("missing path: %+v", results[0])
	}
	if results[0].Range.SL <= 0 || results[0].Range.EL <= 0 {
		t.Fatalf("invalid range: %+v", results[0])
	}
}
