package otidxd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch_SyncOnStartDeletesRemovedFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	needle := "DELETE_TOKEN_123"
	_ = os.WriteFile(path, []byte("hello\n"+needle+"\n"), 0o644)

	s := NewServer(Options{Listen: "127.0.0.1:0"})
	go func() { _ = s.Run() }()
	addr := waitAddr(t, s, time.Second)
	t.Cleanup(func() { _ = s.Close() })

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	wsid, err := c.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("workspace.add: %v", err)
	}
	if _, err := c.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("index.build: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var st WatchStatusResult
	if err := c.call("watch.start", WatchStartParams{WorkspaceID: wsid, SyncOnStart: true, SyncWorkers: 1, DebounceMS: 50}, &st); err != nil {
		t.Fatalf("watch.start: %v", err)
	}

	items, err := c.Query(QueryParams{WorkspaceID: wsid, Q: needle, Unit: "block", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items after delete, got=%+v", items)
	}
}
