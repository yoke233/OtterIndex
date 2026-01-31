package otidxd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"otterindex/internal/core/indexer"
	"otterindex/internal/core/query"
	"otterindex/internal/index/sqlite"
	"otterindex/internal/model"
)

type workspaceInfo struct {
	root   string
	dbPath string
}

type Handlers struct {
	mu         sync.RWMutex
	workspaces map[string]workspaceInfo
	cache      *query.QueryCache
	session    *query.SessionStore
}

func NewHandlers() *Handlers {
	return &Handlers{
		workspaces: map[string]workspaceInfo{},
		cache:      query.NewQueryCache(128),
		session:    query.NewSessionStore(query.SessionOptions{TTL: 30 * time.Second}),
	}
}

func (h *Handlers) WorkspaceAdd(p WorkspaceAddParams) (string, error) {
	if h == nil {
		return "", fmt.Errorf("handlers is nil")
	}
	root := strings.TrimSpace(p.Root)
	if root == "" {
		return "", fmt.Errorf("root is required")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootAbs = filepath.Clean(rootAbs)

	st, err := os.Stat(rootAbs)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("root is not a directory")
	}

	dbPath := strings.TrimSpace(p.DBPath)
	if dbPath == "" {
		dbPath = filepath.Join(rootAbs, ".otidx", "index.db")
	} else if !filepath.IsAbs(dbPath) {
		if abs, err := filepath.Abs(dbPath); err == nil {
			dbPath = abs
		}
	}

	wsid := uuid.NewString()

	h.mu.Lock()
	h.workspaces[wsid] = workspaceInfo{root: rootAbs, dbPath: dbPath}
	h.mu.Unlock()

	return wsid, nil
}

func (h *Handlers) IndexBuild(p IndexBuildParams) (any, error) {
	if h == nil {
		return nil, fmt.Errorf("handlers is nil")
	}

	ws, ok := h.getWorkspace(p.WorkspaceID)
	if !ok {
		return nil, fmt.Errorf("workspace not found")
	}

	err := indexer.Build(ws.root, ws.dbPath, indexer.Options{
		WorkspaceID:  p.WorkspaceID,
		ScanAll:      p.ScanAll,
		IncludeGlobs: p.IncludeGlobs,
		ExcludeGlobs: p.ExcludeGlobs,
	})
	if err != nil {
		return nil, err
	}
	return true, nil
}

func (h *Handlers) Query(p QueryParams) ([]model.ResultItem, error) {
	if h == nil {
		return nil, fmt.Errorf("handlers is nil")
	}

	ws, ok := h.getWorkspace(p.WorkspaceID)
	if !ok {
		return nil, fmt.Errorf("workspace not found")
	}

	opts := query.Options{
		Unit:            p.Unit,
		ContextLines:    p.ContextLines,
		CaseInsensitive: p.CaseInsensitive,
		IncludeGlobs:    p.IncludeGlobs,
		ExcludeGlobs:    p.ExcludeGlobs,
		Limit:           p.Limit,
		Offset:          p.Offset,
	}

	// Normalize to match query.Query defaults so the cache key matches actual behavior.
	opts.Unit = strings.TrimSpace(opts.Unit)
	if opts.Unit == "" {
		opts.Unit = "block"
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.ContextLines < 0 {
		opts.ContextLines = 0
	}

	if h.cache == nil && h.session == nil {
		return query.Query(ws.dbPath, p.WorkspaceID, p.Q, opts)
	}

	s, err := sqlite.Open(ws.dbPath)
	if err != nil {
		return nil, err
	}
	ver, err := s.GetVersion(p.WorkspaceID)
	_ = s.Close()
	if err != nil {
		return nil, err
	}

	run := func() ([]model.ResultItem, error) {
		if h.session != nil {
			return query.QueryWithSession(h.session, ver, ws.dbPath, p.WorkspaceID, p.Q, opts)
		}
		return query.Query(ws.dbPath, p.WorkspaceID, p.Q, opts)
	}

	if h.cache == nil {
		return run()
	}
	return query.QueryWithCache(h.cache, ver, p.WorkspaceID, p.Q, opts, run)
}

func (h *Handlers) getWorkspace(workspaceID string) (workspaceInfo, bool) {
	h.mu.RLock()
	ws, ok := h.workspaces[strings.TrimSpace(workspaceID)]
	h.mu.RUnlock()
	return ws, ok
}
