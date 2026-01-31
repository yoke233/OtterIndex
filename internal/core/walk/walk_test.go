package walk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkIncludeExclude(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "a.sql"), []byte("x"), 0o644)

	files, err := ListFiles(root, Options{
		IncludeGlobs: []string{"*.go"},
		ExcludeGlobs: []string{"*.sql"},
		ScanAll:      false,
	})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "a.go" {
		t.Fatalf("files=%v", files)
	}
}

