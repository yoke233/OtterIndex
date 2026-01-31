package otidxd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClient_MinLoop(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0o644)

	s := NewServer(Options{Listen: "127.0.0.1:0"})
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run() }()
	addr := waitAddr(t, s, time.Second)
	t.Cleanup(func() { _ = s.Close() })

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if v, err := c.Version(); err != nil || v == "" {
		t.Fatalf("version=%q err=%v", v, err)
	}

	wsid, err := c.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil || wsid == "" {
		t.Fatalf("workspace.add wsid=%q err=%v", wsid, err)
	}

	if _, err := c.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("index.build: %v", err)
	}

	items, err := c.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "block", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) == 0 || items[0].Path != "a.go" {
		t.Fatalf("bad items: %+v", items)
	}
}
