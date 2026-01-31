package sqlite

import "testing"

func TestReplaceChunksBatch_Replaces(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	ws := "ws1"
	if err := s.EnsureWorkspace(ws, "/tmp/ws1"); err != nil {
		t.Fatalf("ensure ws: %v", err)
	}

	// first write 100
	var c1 []ChunkInput
	for i := 0; i < 100; i++ {
		c1 = append(c1, ChunkInput{SL: i*2 + 1, EL: i*2 + 2, Kind: "window", Title: "", Text: "hello"})
	}
	if err := s.ReplaceChunksBatch(ws, "a.go", c1); err != nil {
		t.Fatalf("replace1: %v", err)
	}

	// then replace with 10
	var c2 []ChunkInput
	for i := 0; i < 10; i++ {
		c2 = append(c2, ChunkInput{SL: i + 1, EL: i + 1, Kind: "window", Title: "", Text: "world"})
	}
	if err := s.ReplaceChunksBatch(ws, "a.go", c2); err != nil {
		t.Fatalf("replace2: %v", err)
	}

	n, err := s.CountChunksByPath(ws, "a.go")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 10 {
		t.Fatalf("expected 10, got %d", n)
	}
}

