package otidxd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexBuild_ReturnsVersion(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\n"), 0o644)

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	v, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if vv, ok := v.(int64); !ok || vv <= 0 {
		t.Fatalf("expected int64 version > 0, got=%T %#v", v, v)
	}
}
