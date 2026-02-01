package otidxd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch_SyncOnStartUpdatesIndex(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	_ = os.WriteFile(path, []byte("hello\n"), 0o644)

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

	needle := "SYNC_TOKEN_123"
	_ = os.WriteFile(path, []byte("hello\n"+needle+"\n"), 0o644)

	var st WatchStatusResult
	if err := c.call("watch.start", WatchStartParams{WorkspaceID: wsid, SyncOnStart: true, SyncWorkers: 1, DebounceMS: 50}, &st); err != nil {
		t.Fatalf("watch.start: %v", err)
	}
	if !st.Running {
		t.Fatalf("expected running")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items, err := c.Query(QueryParams{WorkspaceID: wsid, Q: needle, Unit: "block", Limit: 10, Offset: 0})
		if err == nil && len(items) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout: did not observe synced index for %q", needle)
}
