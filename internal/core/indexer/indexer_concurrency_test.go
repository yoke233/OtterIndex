package indexer

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"otterindex/internal/index/backend"
)

func TestBuild_ParallelRead_SerialWrite(t *testing.T) {
	stores := []string{"sqlite", "bleve"}
	for _, storeName := range stores {
		t.Run(storeName, func(t *testing.T) {
			root := t.TempDir()
			for i := 0; i < 50; i++ {
				_ = os.WriteFile(filepath.Join(root, "f"+strconv.Itoa(i)+".go"), []byte("hello\nworld\n"), 0o644)
			}
			dbPath := filepath.Join(root, ".otidx", "index.db")
			dbPath = backend.NormalizePath(storeName, dbPath)

			err := Build(root, dbPath, Options{Store: storeName, Workers: 4})
			if err != nil {
				t.Fatalf("build: %v", err)
			}

			st, err := backend.Open(storeName, dbPath)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer st.Close()

			files, err := st.CountFiles(root)
			if err != nil {
				t.Fatalf("count files: %v", err)
			}
			if files != 50 {
				t.Fatalf("expected files=50, got %d", files)
			}

			chunks, err := st.CountChunks(root)
			if err != nil {
				t.Fatalf("count chunks: %v", err)
			}
			if chunks <= 0 {
				t.Fatalf("expected chunks > 0, got %d", chunks)
			}
		})
	}
}

