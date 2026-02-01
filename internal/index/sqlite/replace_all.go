package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type FilePlan struct {
	Path   string
	Size   int64
	MTime  int64
	Hash   string
	Chunks []ChunkInput
	Syms   []SymbolInput
	Comms  []CommentInput
	Delete bool
}

func (s *Store) ReplaceFileAll(workspaceID string, path string, size int64, mtime int64, hash string, chunks []ChunkInput, syms []SymbolInput, comms []CommentInput) error {
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

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO files (workspace_id, path, size, mtime, hash)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id, path) DO UPDATE SET
		   size=excluded.size,
		   mtime=excluded.mtime,
		   hash=excluded.hash`,
		workspaceID, path, size, mtime, hash,
	); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, `DELETE FROM chunks WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM symbols WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM comments WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}

	if len(chunks) > 0 {
		stmt, err := conn.PrepareContext(ctx, `INSERT INTO chunks(workspace_id,path,sl,el,kind,title,text) VALUES(?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		for _, c := range chunks {
			kind := strings.TrimSpace(c.Kind)
			if kind == "" {
				kind = "chunk"
			}
			if _, err := stmt.ExecContext(ctx, workspaceID, path, c.SL, c.EL, kind, c.Title, c.Text); err != nil {
				_ = stmt.Close()
				return err
			}
		}
		_ = stmt.Close()
	}

	if len(syms) > 0 {
		stmt, err := conn.PrepareContext(ctx, `INSERT INTO symbols(workspace_id,path,kind,name,sl,sc,el,ec,container,lang,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
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
				_ = stmt.Close()
				return err
			}
		}
		_ = stmt.Close()
	}

	if len(comms) > 0 {
		stmt, err := conn.PrepareContext(ctx, `INSERT INTO comments(workspace_id,path,kind,sl,sc,el,ec,text,lang) VALUES(?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
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
				_ = stmt.Close()
				return err
			}
		}
		_ = stmt.Close()
	}

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO meta(workspace_id, version, updated_at)
		 VALUES(?, 1, ?)
		 ON CONFLICT(workspace_id) DO UPDATE SET
		   version = version + 1,
		   updated_at = excluded.updated_at`,
		workspaceID,
		time.Now().Unix(),
	); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) DeleteFileAll(workspaceID string, path string) error {
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

	if _, err := conn.ExecContext(ctx, `DELETE FROM files WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM chunks WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM symbols WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM comments WHERE workspace_id = ? AND path = ?`, workspaceID, path); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO meta(workspace_id, version, updated_at)
		 VALUES(?, 1, ?)
		 ON CONFLICT(workspace_id) DO UPDATE SET
		   version = version + 1,
		   updated_at = excluded.updated_at`,
		workspaceID,
		time.Now().Unix(),
	); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) ReplaceFilesBatch(workspaceID string, plans []FilePlan) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	if len(plans) == 0 {
		return nil
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

	upsertFileStmt, err := conn.PrepareContext(ctx,
		`INSERT INTO files (workspace_id, path, size, mtime, hash)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id, path) DO UPDATE SET
		   size=excluded.size,
		   mtime=excluded.mtime,
		   hash=excluded.hash`,
	)
	if err != nil {
		return err
	}
	defer upsertFileStmt.Close()

	delFileStmt, err := conn.PrepareContext(ctx, `DELETE FROM files WHERE workspace_id = ? AND path = ?`)
	if err != nil {
		return err
	}
	defer delFileStmt.Close()

	delChunksStmt, err := conn.PrepareContext(ctx, `DELETE FROM chunks WHERE workspace_id = ? AND path = ?`)
	if err != nil {
		return err
	}
	defer delChunksStmt.Close()

	delSymbolsStmt, err := conn.PrepareContext(ctx, `DELETE FROM symbols WHERE workspace_id = ? AND path = ?`)
	if err != nil {
		return err
	}
	defer delSymbolsStmt.Close()

	delCommentsStmt, err := conn.PrepareContext(ctx, `DELETE FROM comments WHERE workspace_id = ? AND path = ?`)
	if err != nil {
		return err
	}
	defer delCommentsStmt.Close()

	insertChunkStmt, err := conn.PrepareContext(ctx, `INSERT INTO chunks(workspace_id,path,sl,el,kind,title,text) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insertChunkStmt.Close()

	insertSymStmt, err := conn.PrepareContext(ctx, `INSERT INTO symbols(workspace_id,path,kind,name,sl,sc,el,ec,container,lang,signature) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insertSymStmt.Close()

	insertCommStmt, err := conn.PrepareContext(ctx, `INSERT INTO comments(workspace_id,path,kind,sl,sc,el,ec,text,lang) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insertCommStmt.Close()

	for _, plan := range plans {
		path := filepath.ToSlash(strings.TrimSpace(plan.Path))
		if path == "" {
			return fmt.Errorf("path is required")
		}

		if plan.Delete {
			if _, err := delFileStmt.ExecContext(ctx, workspaceID, path); err != nil {
				return err
			}
			if _, err := delChunksStmt.ExecContext(ctx, workspaceID, path); err != nil {
				return err
			}
			if _, err := delSymbolsStmt.ExecContext(ctx, workspaceID, path); err != nil {
				return err
			}
			if _, err := delCommentsStmt.ExecContext(ctx, workspaceID, path); err != nil {
				return err
			}
			continue
		}

		if _, err := upsertFileStmt.ExecContext(ctx, workspaceID, path, plan.Size, plan.MTime, strings.TrimSpace(plan.Hash)); err != nil {
			return err
		}
		if _, err := delChunksStmt.ExecContext(ctx, workspaceID, path); err != nil {
			return err
		}
		if _, err := delSymbolsStmt.ExecContext(ctx, workspaceID, path); err != nil {
			return err
		}
		if _, err := delCommentsStmt.ExecContext(ctx, workspaceID, path); err != nil {
			return err
		}

		for _, c := range plan.Chunks {
			kind := strings.TrimSpace(c.Kind)
			if kind == "" {
				kind = "chunk"
			}
			if _, err := insertChunkStmt.ExecContext(ctx, workspaceID, path, c.SL, c.EL, kind, c.Title, c.Text); err != nil {
				return err
			}
		}
		for _, sym := range plan.Syms {
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
			if _, err := insertSymStmt.ExecContext(ctx, workspaceID, path, kind, sym.Name, sym.SL, sc, sym.EL, ec, sym.Container, sym.Lang, sym.Signature); err != nil {
				return err
			}
		}
		for _, c := range plan.Comms {
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
			if _, err := insertCommStmt.ExecContext(ctx, workspaceID, path, kind, c.SL, sc, c.EL, ec, c.Text, c.Lang); err != nil {
				return err
			}
		}
	}

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO meta(workspace_id, version, updated_at)
		 VALUES(?, 1, ?)
		 ON CONFLICT(workspace_id) DO UPDATE SET
		   version = version + 1,
		   updated_at = excluded.updated_at`,
		workspaceID,
		time.Now().Unix(),
	); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}
