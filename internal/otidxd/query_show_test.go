package otidxd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuery_ShowAttachesText(t *testing.T) {
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

	items, err := h.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "block", Limit: 10, Offset: 0, Show: true})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) == 0 || !strings.Contains(items[0].Text, "hello") {
		t.Fatalf("expected text attached: %+v", items)
	}
}
