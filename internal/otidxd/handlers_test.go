package otidxd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandlers_MinLoop_WorkspaceBuildQuery(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0o644)

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("build: %v", err)
	}

	res, err := h.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "block", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res) == 0 || res[0].Path != "a.go" {
		t.Fatalf("bad result: %v", res)
	}
}

