package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"otterindex/internal/core/indexer"
)

func TestNewWatcherWithOptions_Debounce(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\n"), 0o644)

	w, err := NewWatcherWithOptions(root, filepath.Join(root, ".otidx", "index.db"), indexer.Options{}, Options{
		Debounce: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if w.Debounce() != 50*time.Millisecond {
		t.Fatalf("expected debounce 50ms, got=%v", w.Debounce())
	}
}
