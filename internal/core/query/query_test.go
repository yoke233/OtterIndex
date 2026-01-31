package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"otterindex/internal/core/indexer"
	"otterindex/internal/index/sqlite"
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
	if len(results[0].Matches) == 0 {
		t.Fatalf("expected matches, got %+v", results[0])
	}
	if results[0].Matches[0].Line != 1 || results[0].Matches[0].Col != 1 {
		t.Fatalf("unexpected match: %+v", results[0].Matches[0])
	}
	if results[0].Range.SL <= 0 || results[0].Range.EL <= 0 {
		t.Fatalf("invalid range: %+v", results[0])
	}
}

func TestQuery_LimitOffset(t *testing.T) {
	root := t.TempDir()

	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("hello\n")
	}
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte(b.String()), 0o644)
	dbPath := filepath.Join(root, "index.db")

	if err := indexer.Build(root, dbPath, indexer.Options{}); err != nil {
		t.Fatalf("build: %v", err)
	}

	results, err := Query(dbPath, root, "hello", Options{
		Unit:         "line",
		ContextLines: 0,
		Limit:        1,
		Offset:       1,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Matches) == 0 {
		t.Fatalf("expected matches, got %+v", results[0])
	}
	if results[0].Matches[0].Line != 41 {
		t.Fatalf("expected match line 41, got %+v", results[0].Matches[0])
	}
}

func TestQuery_UnitSymbol_UsesEnclosingRangeWhenPresent(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\nfunc Hello() {\n\tprintln(\"hello\")\n}\n"), 0o644)
	dbPath := filepath.Join(root, "index.db")

	if err := indexer.Build(root, dbPath, indexer.Options{}); err != nil {
		t.Fatalf("build: %v", err)
	}

	s, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.ReplaceSymbolsBatch(root, "a.go", []sqlite.SymbolInput{
		{Kind: "function", Name: "Hello", SL: 2, SC: 1, EL: 4, EC: 1, Lang: "go", Signature: "func Hello"},
	}); err != nil {
		_ = s.Close()
		t.Fatalf("replace symbols: %v", err)
	}
	_ = s.Close()

	results, err := Query(dbPath, root, "println", Options{Unit: "symbol"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected results, got %d", len(results))
	}
	if results[0].Kind != "symbol" {
		t.Fatalf("expected kind symbol, got %+v", results[0])
	}
	if results[0].Range.SL != 2 || results[0].Range.EL != 4 {
		t.Fatalf("unexpected range: %+v", results[0].Range)
	}
}
