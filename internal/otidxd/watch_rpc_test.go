package otidxd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServer_WatchStartStopStatus(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\n"), 0o644)

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

	var st WatchStatusResult
	if err := c.call("watch.status", WatchStatusParams{WorkspaceID: wsid}, &st); err != nil {
		t.Fatalf("watch.status: %v", err)
	}
	if st.Running {
		t.Fatalf("expected not running at start")
	}

	if err := c.call("watch.start", WatchStartParams{WorkspaceID: wsid}, &st); err != nil {
		t.Fatalf("watch.start: %v", err)
	}
	if !st.Running {
		t.Fatalf("expected running after start")
	}

	if err := c.call("watch.stop", WatchStopParams{WorkspaceID: wsid}, &st); err != nil {
		t.Fatalf("watch.stop: %v", err)
	}
	if st.Running {
		t.Fatalf("expected stopped")
	}
}
