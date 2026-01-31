package otidxd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuery_ShowMissingFileDoesNotFail(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	_ = os.WriteFile(path, []byte("hello\nworld\n"), 0o644)

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("build: %v", err)
	}

	_ = os.Remove(path)

	items, err := h.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "block", Limit: 10, Offset: 0, Show: true})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected items even if file missing")
	}
	if items[0].Text != "" {
		t.Fatalf("expected empty text when file missing, got=%q", items[0].Text)
	}
}

func TestWatch_StartStopIdempotent(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\n"), 0o644)

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	if _, err := h.WatchStart(WatchStartParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := h.WatchStart(WatchStartParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("start again: %v", err)
	}
	if st, err := h.WatchStatus(WatchStatusParams{WorkspaceID: wsid}); err != nil || !st.Running {
		t.Fatalf("status running: st=%+v err=%v", st, err)
	}

	if _, err := h.WatchStop(WatchStopParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := h.WatchStop(WatchStopParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("stop again: %v", err)
	}
	if st, err := h.WatchStatus(WatchStatusParams{WorkspaceID: wsid}); err != nil || st.Running {
		t.Fatalf("status stopped: st=%+v err=%v", st, err)
	}
}
