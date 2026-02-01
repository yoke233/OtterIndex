package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"otterindex/internal/model"
)

func (s *Store) ReplaceSymbolsBatch(workspaceID string, path string, syms []SymbolInput) error {
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

	if _, err := conn.ExecContext(ctx, `DELETE FROM symbols WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}

	stmt, err := conn.PrepareContext(ctx, `INSERT INTO symbols(workspace_id,path,kind,name,sl,sc,el,ec,container,lang,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sym := range syms {
		kind := strings.TrimSpace(sym.Kind)
		if kind == "" {
			kind = "symbol"
		}
		sc := sym.SC
		if sc <= 0 {
			sc = 1
		}
		ec := sym.EC
		if ec <= 0 {
			ec = 1
		}
		if _, err := stmt.ExecContext(ctx, workspaceID, path, kind, sym.Name, sym.SL, sc, sym.EL, ec, sym.Container, sym.Lang, sym.Signature); err != nil {
			return err
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) ReplaceCommentsBatch(workspaceID string, path string, comms []CommentInput) error {
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

	if _, err := conn.ExecContext(ctx, `DELETE FROM comments WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}

	stmt, err := conn.PrepareContext(ctx, `INSERT INTO comments(workspace_id,path,kind,sl,sc,el,ec,text,lang) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range comms {
		kind := strings.TrimSpace(c.Kind)
		if kind == "" {
			kind = "comment"
		}
		sc := c.SC
		if sc <= 0 {
			sc = 1
		}
		ec := c.EC
		if ec <= 0 {
			ec = 1
		}
		if _, err := stmt.ExecContext(ctx, workspaceID, path, kind, c.SL, sc, c.EL, ec, c.Text, c.Lang); err != nil {
			return err
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) FindMinEnclosingSymbols(workspaceID string, path string, line int) ([]model.SymbolItem, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	if line <= 0 {
		return nil, fmt.Errorf("line must be >= 1")
	}

	rows, err := s.db.Query(
		`SELECT kind, name, container, lang, signature, sl, sc, el, ec
		 FROM symbols
		 WHERE workspace_id = ? AND path = ? AND sl <= ? AND el >= ?
		 ORDER BY (el - sl) ASC
		 LIMIT 8`,
		workspaceID,
		path,
		line,
		line,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.SymbolItem
	for rows.Next() {
		var sym model.SymbolItem
		sym.Path = path
		if err := rows.Scan(&sym.Kind, &sym.Name, &sym.Container, &sym.Lang, &sym.Signature, &sym.Range.SL, &sym.Range.SC, &sym.Range.EL, &sym.Range.EC); err != nil {
			return nil, err
		}
		out = append(out, sym)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
