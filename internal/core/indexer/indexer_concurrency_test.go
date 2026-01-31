package indexer

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"otterindex/internal/index/sqlite"
)

func TestBuild_ParallelRead_SerialWrite(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		_ = os.WriteFile(filepath.Join(root, "f"+strconv.Itoa(i)+".go"), []byte("hello\nworld\n"), 0o644)
	}
	dbPath := filepath.Join(root, ".otidx", "index.db")

	err := Build(root, dbPath, Options{Workers: 4})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	s, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	files, err := s.CountFiles(root)
	if err != nil {
		t.Fatalf("count files: %v", err)
	}
	if files != 50 {
		t.Fatalf("expected files=50, got %d", files)
	}

	chunks, err := s.CountChunks(root)
	if err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if chunks <= 0 {
		t.Fatalf("expected chunks > 0, got %d", chunks)
	}
}

