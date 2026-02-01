package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"otterindex/internal/index/backend"
)

func TestBuildWritesChunks(t *testing.T) {
	stores := []string{"sqlite", "bleve"}
	for _, storeName := range stores {
		t.Run(storeName, func(t *testing.T) {
			root := t.TempDir()
			_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0o644)
			dbPath := filepath.Join(root, "index.db")
			dbPath = backend.NormalizePath(storeName, dbPath)

			if err := Build(root, dbPath, Options{Store: storeName}); err != nil {
				t.Fatalf("build: %v", err)
			}

			st, err := backend.Open(storeName, dbPath)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer st.Close()

			cnt, err := st.CountChunks(root)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if cnt <= 0 {
				t.Fatalf("expected chunks > 0, got %d", cnt)
			}
		})
	}
}

