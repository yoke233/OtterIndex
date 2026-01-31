package sqlite

import "testing"

func TestApplyBuildPragmas_ReadBack(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.ApplyBuildPragmas(); err != nil {
		t.Fatalf("apply: %v", err)
	}

	jm, _ := s.QueryPragma("journal_mode")
	if jm == "" {
		t.Fatalf("journal_mode empty")
	}

	sync, _ := s.QueryPragma("synchronous")
	if sync == "" {
		t.Fatalf("synchronous empty")
	}
}

