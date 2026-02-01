package sqlite

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"otterindex/internal/index/store"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db     *sql.DB
	hasFTS bool
	ftsErr error
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
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

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

func (s *Store) Backend() string { return "sqlite" }

func (s *Store) HasFTS() bool { return s != nil && s.hasFTS }

func (s *Store) FTSReason() string {
	if s == nil {
		return ""
	}
	if s.hasFTS {
		return "enabled"
	}
	if s.ftsErr != nil {
		return s.ftsErr.Error()
	}
	return "fts5 not available"
}

func (s *Store) GetVersion(workspaceID string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspaceID is required")
	}
	if err := s.ensureWorkspace(workspaceID, ""); err != nil {
		return 0, err
	}

	var v int64
	if err := s.db.QueryRow(`SELECT version FROM meta WHERE workspace_id = ?`, workspaceID).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func (s *Store) BumpVersion(workspaceID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	if err := s.ensureWorkspace(workspaceID, ""); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`INSERT INTO meta(workspace_id, version, updated_at)
		 VALUES(?, 1, ?)
		 ON CONFLICT(workspace_id) DO UPDATE SET
		   version = version + 1,
		   updated_at = excluded.updated_at`,
		workspaceID,
		time.Now().Unix(),
	)
	return err
}

func (s *Store) EnsureWorkspace(id string, root string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	return s.ensureWorkspace(id, root)
}

func (s *Store) UpsertFile(workspaceID string, path string, size int64, mtime int64, hash string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	hash = strings.TrimSpace(hash)
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
		hash,
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

func (s *Store) GetFileMeta(workspaceID string, path string) (File, bool, error) {
	f, err := s.GetFile(workspaceID, path)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, err
	}
	return f, true, nil
}

func (s *Store) GetFilesStats(workspaceID string) (int, int64, error) {
	if s == nil || s.db == nil {
		return 0, 0, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, 0, fmt.Errorf("workspaceID is required")
	}

	var count int
	var total sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT COUNT(1), COALESCE(SUM(size), 0) FROM files WHERE workspace_id = ?`,
		workspaceID,
	).Scan(&count, &total); err != nil {
		return 0, 0, err
	}
	if !total.Valid {
		return count, 0, nil
	}
	return count, total.Int64, nil
}

func (s *Store) ListFilesMeta(workspaceID string) (map[string]File, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceID is required")
	}

	rows, err := s.db.Query(
		`SELECT path, size, mtime, hash
		 FROM files
		 WHERE workspace_id = ?`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]File{}
	for rows.Next() {
		var f File
		f.WorkspaceID = workspaceID
		if err := rows.Scan(&f.Path, &f.Size, &f.MTime, &f.Hash); err != nil {
			return nil, err
		}
		out[f.Path] = f
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) DeleteFile(workspaceID string, path string) error {
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

	_, err := s.db.Exec(`DELETE FROM files WHERE workspace_id = ? AND path = ?`, workspaceID, path)
	return err
}

func (s *Store) GetWorkspace(workspaceID string) (Workspace, error) {
	if s == nil || s.db == nil {
		return Workspace{}, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return Workspace{}, fmt.Errorf("workspaceID is required")
	}

	var ws Workspace
	err := s.db.QueryRow(
		`SELECT id, root, created_at
		 FROM workspaces
		 WHERE id = ?`,
		workspaceID,
	).Scan(&ws.ID, &ws.Root, &ws.CreatedAt)
	if err != nil {
		return Workspace{}, err
	}
	return ws, nil
}

func (s *Store) SearchChunks(workspaceID string, keyword string, limit int, caseInsensitive bool) (store.SearchResult, error) {
	if s == nil || s.db == nil {
		return store.SearchResult{}, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	keyword = strings.TrimSpace(keyword)
	if workspaceID == "" {
		return store.SearchResult{}, fmt.Errorf("workspaceID is required")
	}
	if keyword == "" {
		return store.SearchResult{}, fmt.Errorf("keyword is required")
	}
	if limit <= 0 {
		limit = 50
	}

	var rows *sql.Rows
	var err error
	if s.hasFTS {
		rows, err = s.db.Query(
			`SELECT path, sl, el, kind, title, text
			 FROM chunks_fts
			 WHERE chunks_fts MATCH ? AND workspace_id = ?
			 ORDER BY path, sl, el
			 LIMIT ?`,
			keyword,
			workspaceID,
			limit,
		)
		if err != nil {
			rows, err = s.db.Query(
				`SELECT c.path, c.sl, c.el, c.kind, c.title, c.text
				 FROM chunks_fts
				 JOIN chunks c ON c.id = chunks_fts.rowid
				 WHERE chunks_fts MATCH ? AND c.workspace_id = ?
				 ORDER BY c.path, c.sl, c.el
				 LIMIT ?`,
				keyword,
				workspaceID,
				limit,
			)
		}
		if err != nil {
			return store.SearchResult{}, err
		}
		defer rows.Close()

		var out []Chunk
		for rows.Next() {
			var c Chunk
			c.WorkspaceID = workspaceID
			if err := rows.Scan(&c.Path, &c.SL, &c.EL, &c.Kind, &c.Title, &c.Text); err != nil {
				return store.SearchResult{}, err
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return store.SearchResult{}, err
		}
		return store.SearchResult{
			Chunks:               out,
			MatchCaseInsensitive: true,
			Backend:              "sqlite",
		}, nil
	} else {
		query := `SELECT path, sl, el, kind, title, text
		          FROM chunks
		          WHERE workspace_id = ? AND text LIKE '%' || ? || '%'
		          ORDER BY path, sl, el
		          LIMIT ?`
		if caseInsensitive {
			query = `SELECT path, sl, el, kind, title, text
			         FROM chunks
			         WHERE workspace_id = ? AND LOWER(text) LIKE '%' || LOWER(?) || '%'
			         ORDER BY path, sl, el
			         LIMIT ?`
		}
		rows, err = s.db.Query(query, workspaceID, keyword, limit)
	}
	if err != nil {
		return store.SearchResult{}, err
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		var c Chunk
		c.WorkspaceID = workspaceID
		if err := rows.Scan(&c.Path, &c.SL, &c.EL, &c.Kind, &c.Title, &c.Text); err != nil {
			return store.SearchResult{}, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return store.SearchResult{}, err
	}
	return store.SearchResult{
		Chunks:               out,
		MatchCaseInsensitive: caseInsensitive,
		Backend:              "sqlite",
	}, nil
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

	var in []ChunkInput
	for _, c := range chunks {
		in = append(in, ChunkInput{SL: c.SL, EL: c.EL, Kind: c.Kind, Title: c.Title, Text: c.Text})
	}
	return s.ReplaceChunksBatch(workspaceID, path, in)
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

func (s *Store) CountFiles(workspaceID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspaceID is required")
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM files WHERE workspace_id = ?`, workspaceID).Scan(&n); err != nil {
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
		s.ftsErr = err
	}

	return nil
}

func (s *Store) tryCreateFTS() error {
	// FTS is optional: if the driver/build does not support fts5 we fall back later.
	ok, err := hasFTSColumns(s.db)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	stmts := []string{
		`DROP TRIGGER IF EXISTS chunks_ai`,
		`DROP TRIGGER IF EXISTS chunks_ad`,
		`DROP TRIGGER IF EXISTS chunks_au`,
		`DROP TABLE IF EXISTS chunks_fts`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts
		 USING fts5(
		   text,
		   title,
		   path UNINDEXED,
		   workspace_id UNINDEXED,
		   kind UNINDEXED,
		   sl UNINDEXED,
		   el UNINDEXED,
		   content='chunks',
		   content_rowid='id'
		 )`,
		`CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
		   INSERT INTO chunks_fts(rowid, text, title, path, workspace_id, kind, sl, el)
		   VALUES (new.id, new.text, new.title, new.path, new.workspace_id, new.kind, new.sl, new.el);
		 END`,
		`CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
		   INSERT INTO chunks_fts(chunks_fts, rowid, text, title, path, workspace_id, kind, sl, el)
		   VALUES('delete', old.id, old.text, old.title, old.path, old.workspace_id, old.kind, old.sl, old.el);
		 END`,
		`CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
		   INSERT INTO chunks_fts(chunks_fts, rowid, text, title, path, workspace_id, kind, sl, el)
		   VALUES('delete', old.id, old.text, old.title, old.path, old.workspace_id, old.kind, old.sl, old.el);
		   INSERT INTO chunks_fts(rowid, text, title, path, workspace_id, kind, sl, el)
		   VALUES (new.id, new.text, new.title, new.path, new.workspace_id, new.kind, new.sl, new.el);
		 END`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	_, _ = s.db.Exec(`INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')`)
	return nil
}

func hasFTSColumns(db *sql.DB) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("db is nil")
	}
	rows, err := db.Query(`PRAGMA table_info(chunks_fts)`)
	if err != nil {
		return false, nil
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	want := []string{"text", "title", "path", "workspace_id", "kind", "sl", "el"}
	for _, name := range want {
		if !cols[name] {
			return false, nil
		}
	}
	return true, nil
}

func (s *Store) ensureWorkspace(id string, root string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("workspace id is required")
	}

	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT INTO workspaces (id, root, created_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = COALESCE(NULLIF(excluded.root, ''), workspaces.root)`,
		id,
		root,
		now,
	)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`INSERT INTO meta(workspace_id, version, updated_at)
		 VALUES (?, 1, ?)
		 ON CONFLICT(workspace_id) DO NOTHING`,
		id,
		now,
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
