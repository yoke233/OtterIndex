package sqlite

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db     *sql.DB
	hasFTS bool
}

type File struct {
	WorkspaceID string
	Path        string
	Size        int64
	MTime       int64
	Hash        string
}

type Chunk struct {
	Path        string
	SL          int
	EL          int
	Kind        string
	Title       string
	Text        string
	WorkspaceID string
}

func Open(dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("dbPath is required")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) HasFTS() bool { return s != nil && s.hasFTS }

func (s *Store) EnsureWorkspace(id string, root string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	return s.ensureWorkspace(id, root)
}

func (s *Store) UpsertFile(workspaceID string, path string, size int64, mtime int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}

	if err := s.ensureWorkspace(workspaceID, ""); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`INSERT INTO files (workspace_id, path, size, mtime, hash)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id, path) DO UPDATE SET
		   size=excluded.size,
		   mtime=excluded.mtime,
		   hash=excluded.hash`,
		workspaceID,
		path,
		size,
		mtime,
		"",
	)
	return err
}

func (s *Store) GetFile(workspaceID string, path string) (File, error) {
	if s == nil || s.db == nil {
		return File{}, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	if workspaceID == "" {
		return File{}, fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return File{}, fmt.Errorf("path is required")
	}

	var f File
	f.WorkspaceID = workspaceID
	f.Path = path
	err := s.db.QueryRow(
		`SELECT size, mtime, hash
		 FROM files
		 WHERE workspace_id = ? AND path = ?`,
		workspaceID,
		path,
	).Scan(&f.Size, &f.MTime, &f.Hash)
	if err != nil {
		return File{}, err
	}
	return f, nil
}

func (s *Store) ReplaceChunks(workspaceID string, path string, chunks []Chunk) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}

	if err := s.ensureWorkspace(workspaceID, ""); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM chunks WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}

	for _, c := range chunks {
		kind := strings.TrimSpace(c.Kind)
		if kind == "" {
			kind = "chunk"
		}
		title := c.Title
		text := c.Text
		if _, err := tx.Exec(
			`INSERT INTO chunks (workspace_id, path, sl, el, kind, title, text)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			workspaceID,
			path,
			c.SL,
			c.EL,
			kind,
			title,
			text,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) CountChunks(workspaceID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspaceID is required")
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM chunks WHERE workspace_id = ?`, workspaceID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) init() error {
	if _, err := s.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	if _, err := s.db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return err
	}
	_, _ = s.db.Exec("PRAGMA journal_mode = WAL")

	if err := execStatements(s.db, schemaSQL); err != nil {
		return err
	}

	s.hasFTS = true
	if err := s.tryCreateFTS(); err != nil {
		s.hasFTS = false
	}

	return nil
}

func (s *Store) tryCreateFTS() error {
	// FTS is optional: if the driver/build does not support fts5 we fall back later.
	stmts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts
		 USING fts5(
		   text,
		   title,
		   path UNINDEXED,
		   workspace_id UNINDEXED,
		   content='chunks',
		   content_rowid='id'
		 )`,
		`CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
		   INSERT INTO chunks_fts(rowid, text, title, path, workspace_id)
		   VALUES (new.id, new.text, new.title, new.path, new.workspace_id);
		 END`,
		`CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
		   INSERT INTO chunks_fts(chunks_fts, rowid, text, title, path, workspace_id)
		   VALUES('delete', old.id, old.text, old.title, old.path, old.workspace_id);
		 END`,
		`CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
		   INSERT INTO chunks_fts(chunks_fts, rowid, text, title, path, workspace_id)
		   VALUES('delete', old.id, old.text, old.title, old.path, old.workspace_id);
		   INSERT INTO chunks_fts(rowid, text, title, path, workspace_id)
		   VALUES (new.id, new.text, new.title, new.path, new.workspace_id);
		 END`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureWorkspace(id string, root string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("workspace id is required")
	}

	_, err := s.db.Exec(
		`INSERT INTO workspaces (id, root, created_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		id,
		root,
		time.Now().Unix(),
	)
	return err
}

func execStatements(db *sql.DB, sqlText string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	sqlText = strings.ReplaceAll(sqlText, "\r\n", "\n")

	var cleaned strings.Builder
	for _, line := range strings.Split(sqlText, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "--") {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteString("\n")
	}

	parts := strings.Split(cleaned.String(), ";")
	for _, raw := range parts {
		stmt := strings.TrimSpace(raw)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}
	return nil
}
