package otidxd

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func startTestServer(t *testing.T) (string, func()) {
	t.Helper()

	s := NewServer(Options{Listen: "127.0.0.1:0"})
	go func() { _ = s.Run() }()
	addr := waitAddr(t, s, time.Second)
	cleanup := func() { _ = s.Close() }
	return addr, cleanup
}

func sendRawRequest(t *testing.T, addr string, raw string) Response {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	if _, err := w.WriteString(raw + "\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	line, err := ReadOneLine(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
}

func TestHandlers_WorkspaceAdd_Errors(t *testing.T) {
	h := NewHandlers()

	if _, err := h.WorkspaceAdd(WorkspaceAddParams{Root: ""}); err == nil {
		t.Fatalf("expected error for empty root")
	}

	root := t.TempDir()
	filePath := filepath.Join(root, "a.txt")
	_ = os.WriteFile(filePath, []byte("x"), 0o644)
	if _, err := h.WorkspaceAdd(WorkspaceAddParams{Root: filePath}); err == nil {
		t.Fatalf("expected error for file root")
	}

	if _, err := h.WorkspaceAdd(WorkspaceAddParams{Root: filepath.Join(root, "missing")}); err == nil {
		t.Fatalf("expected error for missing root")
	}
}

func TestHandlers_WorkspaceAdd_RelativeDBPath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, err := os.MkdirTemp(cwd, "otidxd-edge-")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\n"), 0o644)

	absTarget := filepath.Join(root, "sub", "index.db")
	relTarget, err := filepath.Rel(cwd, absTarget)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root, DBPath: relTarget})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("build: %v", err)
	}

	if _, err := os.Stat(absTarget); err != nil {
		t.Fatalf("expected db at %s: %v", absTarget, err)
	}
}

func TestHandlers_Query_DefaultsApplied(t *testing.T) {
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

	items, err := h.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "", Limit: 0, Offset: 0, ContextLines: -1})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected items with defaults applied")
	}
}

func TestHandlers_Query_ShowClampsRange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	_ = os.WriteFile(path, []byte("hello\nworld\nagain\n"), 0o644)

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("build: %v", err)
	}

	_ = os.WriteFile(path, []byte("hello\n"), 0o644)

	items, err := h.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "block", Limit: 10, Offset: 0, Show: true})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected items")
	}
	if items[0].Text == "" {
		t.Fatalf("expected clamped text")
	}
}

func TestRPC_ValidationErrors(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.call("workspace.add", "bad", nil); err == nil {
		t.Fatalf("expected invalid params error")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32602 {
		t.Fatalf("expected -32602, got=%T %+v", err, err)
	}

	if _, err := c.IndexBuild(IndexBuildParams{WorkspaceID: ""}); err == nil {
		t.Fatalf("expected invalid workspace_id error")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32602 {
		t.Fatalf("expected -32602, got=%T %+v", err, err)
	}

	if _, err := c.Query(QueryParams{WorkspaceID: "", Q: "x"}); err == nil {
		t.Fatalf("expected invalid workspace_id error")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32602 {
		t.Fatalf("expected -32602, got=%T %+v", err, err)
	}

	if _, err := c.Query(QueryParams{WorkspaceID: "x", Q: ""}); err == nil {
		t.Fatalf("expected invalid q error")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32602 {
		t.Fatalf("expected -32602, got=%T %+v", err, err)
	}
}

func TestRPC_MethodNotFound(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.call("no.such.method", nil, nil); err == nil {
		t.Fatalf("expected method not found")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32601 {
		t.Fatalf("expected -32601, got=%T %+v", err, err)
	}
}

func TestRPC_InvalidJSONRPCVersion(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp := sendRawRequest(t, addr, `{"jsonrpc":"1.0","method":"ping","id":1}`)
	if resp.Error == nil || resp.Error.Code != -32600 {
		t.Fatalf("expected -32600, got=%+v", resp.Error)
	}
}

func TestRPC_WorkspaceNotFound(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.IndexBuild(IndexBuildParams{WorkspaceID: "missing"}); err == nil {
		t.Fatalf("expected workspace not found")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32000 {
		t.Fatalf("expected -32000, got=%T %+v", err, err)
	}

	if _, err := c.Query(QueryParams{WorkspaceID: "missing", Q: "x"}); err == nil {
		t.Fatalf("expected workspace not found")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32000 {
		t.Fatalf("expected -32000, got=%T %+v", err, err)
	}

	if err := c.call("watch.status", WatchStatusParams{WorkspaceID: "missing"}, nil); err == nil {
		t.Fatalf("expected workspace not found")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32000 {
		t.Fatalf("expected -32000, got=%T %+v", err, err)
	}
	if err := c.call("watch.start", WatchStartParams{WorkspaceID: "missing"}, nil); err == nil {
		t.Fatalf("expected workspace not found")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32000 {
		t.Fatalf("expected -32000, got=%T %+v", err, err)
	}
	if err := c.call("watch.stop", WatchStopParams{WorkspaceID: "missing"}, nil); err == nil {
		t.Fatalf("expected workspace not found")
	} else if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32000 {
		t.Fatalf("expected -32000, got=%T %+v", err, err)
	}
}
