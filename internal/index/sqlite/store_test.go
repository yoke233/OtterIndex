package sqlite

import (
	"database/sql"
	"errors"
	"testing"
)

func TestCreateAndUpsertFile(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.UpsertFile("ws1", "a.go", 123, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}

func TestUpsertFile_UpdatesExisting(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.UpsertFile("ws1", "a.go", 123, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.UpsertFile("ws1", "a.go", 456, 2); err != nil {
		t.Fatalf("upsert2: %v", err)
	}

	f, err := s.GetFile("ws1", "a.go")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if f.Size != 456 || f.MTime != 2 {
		t.Fatalf("file=%+v", f)
	}
}

func TestOpen_EmptyPath(t *testing.T) {
	_, err := Open("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpsertFile_RequiresWorkspaceAndPath(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.UpsertFile("", "a.go", 1, 1); err == nil {
		t.Fatal("expected error for empty workspaceID")
	}
	if err := s.UpsertFile("ws1", "", 1, 1); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestGetFile_NotFound(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	_, err = s.GetFile("ws1", "missing.go")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
