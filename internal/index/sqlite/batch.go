package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type ChunkInput struct {
	SL, EL int
	Kind   string
	Title  string
	Text   string
}

func (s *Store) ReplaceChunksBatch(workspaceID string, path string, chunks []ChunkInput) error {
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

	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
	}()

	if _, err := conn.ExecContext(ctx, `DELETE FROM chunks WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}

	stmt, err := conn.PrepareContext(ctx, `INSERT INTO chunks(workspace_id,path,sl,el,kind,title,text) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range chunks {
		kind := strings.TrimSpace(c.Kind)
		if kind == "" {
			kind = "chunk"
		}
		if _, err := stmt.ExecContext(ctx, workspaceID, path, c.SL, c.EL, kind, c.Title, c.Text); err != nil {
			return err
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) CountChunksByPath(workspaceID string, path string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return 0, fmt.Errorf("path is required")
	}

	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM chunks WHERE workspace_id = ? AND path = ?`,
		workspaceID,
		path,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

