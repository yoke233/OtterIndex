package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"otterindex/internal/index/sqlite"
)

func TestBuildWritesChunks(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0o644)
	dbPath := filepath.Join(root, "index.db")

	if err := Build(root, dbPath, Options{}); err != nil {
		t.Fatalf("build: %v", err)
	}

	s, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	cnt, err := s.CountChunks(root)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt <= 0 {
		t.Fatalf("expected chunks > 0, got %d", cnt)
	}
}

