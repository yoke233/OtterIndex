package otidxd

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestServerPingAndVersion(t *testing.T) {
	s := NewServer(Options{Listen: "127.0.0.1:0"})

	errCh := make(chan error, 1)
	go func() { errCh <- s.Run() }()

	addr := waitAddr(t, s, time.Second)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	t.Cleanup(func() { _ = w.Flush() })

	if err := WriteOneLine(w, Request{JSONRPC: "2.0", Method: "ping", ID: json.RawMessage("1")}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush ping: %v", err)
	}
	var pingResp Response
	line, err := ReadOneLine(r)
	if err != nil {
		t.Fatalf("read ping: %v", err)
	}
	if err := json.Unmarshal(line, &pingResp); err != nil {
		t.Fatalf("unmarshal ping: %v", err)
	}
	if string(pingResp.ID) != "1" {
		t.Fatalf("ping id=%s", string(pingResp.ID))
	}
	if pingResp.Error != nil {
		t.Fatalf("ping error=%+v", pingResp.Error)
	}
	if pingResp.Result != "pong" {
		t.Fatalf("ping result=%v", pingResp.Result)
	}

	if err := WriteOneLine(w, Request{JSONRPC: "2.0", Method: "version", ID: json.RawMessage("2")}); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush version: %v", err)
	}
	var versionResp Response
	line, err = ReadOneLine(r)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if err := json.Unmarshal(line, &versionResp); err != nil {
		t.Fatalf("unmarshal version: %v", err)
	}
	if string(versionResp.ID) != "2" {
		t.Fatalf("version id=%s", string(versionResp.ID))
	}
	if versionResp.Error != nil {
		t.Fatalf("version error=%+v", versionResp.Error)
	}
	if s, ok := versionResp.Result.(string); !ok || s == "" {
		t.Fatalf("version result=%v", versionResp.Result)
	}

	_ = s.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop within 1s after Close")
	}
}

func waitAddr(t *testing.T, s *Server, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if addr := s.Addr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not start listening in time")
	return ""
}
