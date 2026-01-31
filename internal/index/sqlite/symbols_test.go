package sqlite

import "testing"

func TestSymbols_ReplaceAndFindMinEnclosing(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.EnsureWorkspace("ws1", "/tmp"); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	path := "a.go"
	if err := s.ReplaceSymbolsBatch("ws1", path, []SymbolInput{
		{Kind: "function", Name: "big", SL: 1, SC: 1, EL: 100, EC: 1},
		{Kind: "function", Name: "small", SL: 10, SC: 1, EL: 20, EC: 1},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	syms, err := s.FindMinEnclosingSymbols("ws1", path, 12)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(syms) == 0 || syms[0].Name != "small" {
		t.Fatalf("syms=%v", syms)
	}

	if err := s.ReplaceSymbolsBatch("ws1", path, nil); err != nil {
		t.Fatalf("replace nil: %v", err)
	}
	syms, err = s.FindMinEnclosingSymbols("ws1", path, 12)
	if err != nil {
		t.Fatalf("find2: %v", err)
	}
	if len(syms) != 0 {
		t.Fatalf("expected empty, got %v", syms)
	}
}
