package sqlite

import "testing"

func TestMetaVersion_Bump(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	ws := "ws1"
	_ = s.EnsureWorkspace(ws, "/tmp")

	v1, err := s.GetVersion(ws)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if err := s.BumpVersion(ws); err != nil {
		t.Fatalf("bump: %v", err)
	}
	v2, err := s.GetVersion(ws)
	if err != nil {
		t.Fatalf("get v2: %v", err)
	}

	if v2 <= v1 {
		t.Fatalf("expected version increased: v1=%d v2=%d", v1, v2)
	}
}

