package otidxd

import (
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

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	if err := enc.Encode(Request{JSONRPC: "2.0", Method: "ping", ID: json.RawMessage("1")}); err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	var pingResp Response
	if err := dec.Decode(&pingResp); err != nil {
		t.Fatalf("decode ping: %v", err)
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

	if err := enc.Encode(Request{JSONRPC: "2.0", Method: "version", ID: json.RawMessage("2")}); err != nil {
		t.Fatalf("encode version: %v", err)
	}
	var versionResp Response
	if err := dec.Decode(&versionResp); err != nil {
		t.Fatalf("decode version: %v", err)
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
