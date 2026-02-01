package otidxd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"otterindex/internal/core/indexer"
	"otterindex/internal/core/query"
	"otterindex/internal/core/walk"
	"otterindex/internal/core/watch"
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
	watchers   map[string]*watcherEntry
}

func NewHandlers() *Handlers {
	return &Handlers{
		workspaces: map[string]workspaceInfo{},
		cache:      query.NewQueryCache(128),
		session:    query.NewSessionStore(query.SessionOptions{TTL: 30 * time.Second}),
		watchers:   map[string]*watcherEntry{},
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

	s, err := sqlite.Open(ws.dbPath)
	if err != nil {
		return nil, err
	}
	ver, err := s.GetVersion(p.WorkspaceID)
	_ = s.Close()
	if err != nil {
		return nil, err
	}
	return ver, nil
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
		items, err := run()
		if err != nil {
			return nil, err
		}
		if p.Show {
			attachText(ws.root, items)
		}
		return items, nil
	}
	items, err := query.QueryWithCache(h.cache, ver, p.WorkspaceID, p.Q, opts, run)
	if err != nil {
		return nil, err
	}
	if p.Show {
		attachText(ws.root, items)
	}
	return items, nil
}

type watcherEntry struct {
	w      *watch.Watcher
	cancel context.CancelFunc
	done   chan struct{}
}

func (h *Handlers) WatchStart(p WatchStartParams) (WatchStatusResult, error) {
	if h == nil {
		return WatchStatusResult{}, fmt.Errorf("handlers is nil")
	}
	ws, ok := h.getWorkspace(p.WorkspaceID)
	if !ok {
		return WatchStatusResult{}, fmt.Errorf("workspace not found")
	}

	wsid := strings.TrimSpace(p.WorkspaceID)

	h.mu.Lock()
	if existing, ok := h.watchers[wsid]; ok && existing != nil {
		if existing.done != nil {
			select {
			case <-existing.done:
				delete(h.watchers, wsid)
			default:
				h.mu.Unlock()
				return WatchStatusResult{Running: true}, nil
			}
		}
	}
	h.mu.Unlock()

	w, err := watch.NewWatcherWithOptions(ws.root, ws.dbPath, indexer.Options{
		WorkspaceID:  wsid,
		ScanAll:      p.ScanAll,
		IncludeGlobs: p.IncludeGlobs,
		ExcludeGlobs: p.ExcludeGlobs,
	}, watch.Options{
		Debounce:         debounceFromParams(p.DebounceMS),
		AdaptiveDebounce: p.AdaptiveDebounce,
		DebounceMin:      debounceFromParams(p.DebounceMinMS),
		DebounceMax:      debounceFromParams(p.DebounceMaxMS),
	})
	if err != nil {
		return WatchStatusResult{}, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
		close(done)
	}()

	h.mu.Lock()
	h.watchers[wsid] = &watcherEntry{w: w, cancel: cancel, done: done}
	h.mu.Unlock()

	if p.SyncOnStart {
		if err := syncChangedFiles(ws.root, ws.dbPath, indexer.Options{
			WorkspaceID:  wsid,
			ScanAll:      p.ScanAll,
			IncludeGlobs: p.IncludeGlobs,
			ExcludeGlobs: p.ExcludeGlobs,
		}, p.SyncWorkers); err != nil {
			return WatchStatusResult{}, err
		}
	}

	return WatchStatusResult{Running: true}, nil
}

func (h *Handlers) WatchStop(p WatchStopParams) (WatchStatusResult, error) {
	if h == nil {
		return WatchStatusResult{}, fmt.Errorf("handlers is nil")
	}
	if _, ok := h.getWorkspace(p.WorkspaceID); !ok {
		return WatchStatusResult{}, fmt.Errorf("workspace not found")
	}

	wsid := strings.TrimSpace(p.WorkspaceID)

	h.mu.Lock()
	entry := h.watchers[wsid]
	delete(h.watchers, wsid)
	h.mu.Unlock()

	if entry != nil {
		if entry.cancel != nil {
			entry.cancel()
		}
		if entry.w != nil {
			_ = entry.w.Close()
		}
	}
	return WatchStatusResult{Running: false}, nil
}

func (h *Handlers) WatchStatus(p WatchStatusParams) (WatchStatusResult, error) {
	if h == nil {
		return WatchStatusResult{}, fmt.Errorf("handlers is nil")
	}
	if _, ok := h.getWorkspace(p.WorkspaceID); !ok {
		return WatchStatusResult{}, fmt.Errorf("workspace not found")
	}

	wsid := strings.TrimSpace(p.WorkspaceID)
	h.mu.RLock()
	entry := h.watchers[wsid]
	h.mu.RUnlock()
	if entry == nil || entry.done == nil {
		return WatchStatusResult{Running: false}, nil
	}
	select {
	case <-entry.done:
		return WatchStatusResult{Running: false}, nil
	default:
		return WatchStatusResult{Running: true}, nil
	}
}

func (h *Handlers) Close() error {
	if h == nil {
		return nil
	}

	h.mu.Lock()
	watchers := h.watchers
	h.watchers = map[string]*watcherEntry{}
	h.mu.Unlock()

	for _, entry := range watchers {
		if entry == nil {
			continue
		}
		if entry.cancel != nil {
			entry.cancel()
		}
		if entry.w != nil {
			_ = entry.w.Close()
		}
	}
	return nil
}

func (h *Handlers) getWorkspace(workspaceID string) (workspaceInfo, bool) {
	h.mu.RLock()
	ws, ok := h.workspaces[strings.TrimSpace(workspaceID)]
	h.mu.RUnlock()
	return ws, ok
}

func attachText(workspaceRoot string, items []model.ResultItem) {
	base := strings.TrimSpace(workspaceRoot)
	if base == "" {
		base = "."
	}

	fileCache := map[string][]string{}
	for i := range items {
		if strings.TrimSpace(items[i].Text) != "" {
			continue
		}

		lines := loadFileLines(base, items[i].Path, fileCache)
		if len(lines) == 0 {
			continue
		}

		sl := clampInt(items[i].Range.SL, 1, len(lines))
		el := clampInt(items[i].Range.EL, sl, len(lines))
		items[i].Text = strings.Join(lines[sl-1:el], "\n")
	}
}

func loadFileLines(base string, rel string, cache map[string][]string) []string {
	if cache != nil {
		if v, ok := cache[rel]; ok {
			return v
		}
	}

	full := filepath.Join(base, filepath.FromSlash(rel))
	b, err := os.ReadFile(full)
	if err != nil {
		if cache != nil {
			cache[rel] = nil
		}
		return nil
	}

	lines := splitLines(string(b))
	if cache != nil {
		cache[rel] = lines
	}
	return lines
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func clampInt(v int, min int, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func syncChangedFiles(root string, dbPath string, opts indexer.Options, workers int) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rootAbs = filepath.Clean(rootAbs)
	if strings.TrimSpace(rootAbs) == "" {
		return fmt.Errorf("root is required")
	}

	dbAbs := dbPath
	if !filepath.IsAbs(dbAbs) {
		if abs, err := filepath.Abs(dbAbs); err == nil {
			dbAbs = abs
		}
	}
	dbRel := ""
	if rel, err := filepath.Rel(rootAbs, dbAbs); err == nil {
		if rel != "." && !strings.HasPrefix(rel, "..") {
			dbRel = filepath.ToSlash(rel)
		}
	}

	s, err := sqlite.Open(dbPath)
	if err != nil {
		return err
	}
	defer s.Close()

	workspaceID := strings.TrimSpace(opts.WorkspaceID)
	if workspaceID == "" {
		workspaceID = rootAbs
	}

	if err := s.EnsureWorkspace(workspaceID, rootAbs); err != nil {
		return err
	}

	dbMeta, err := s.ListFilesMeta(workspaceID)
	if err != nil {
		return err
	}

	files, err := walk.ListFiles(rootAbs, walk.Options{
		IncludeGlobs: opts.IncludeGlobs,
		ExcludeGlobs: opts.ExcludeGlobs,
		ScanAll:      opts.ScanAll,
	})
	if err != nil {
		return err
	}

	workers = normalizeWorkers(workers)

	fileSet := map[string]bool{}
	for _, rel := range files {
		fileSet[rel] = true
	}

	var deleted []string
	for rel := range dbMeta {
		if !fileSet[rel] {
			deleted = append(deleted, rel)
		}
	}

	for _, rel := range deleted {
		if isDBRel(rel, dbRel) {
			continue
		}
		if err := s.DeleteFileAll(workspaceID, rel); err != nil {
			return err
		}
	}

	jobs := make(chan string)
	plans := make(chan indexer.UpdatePlan)
	var wg sync.WaitGroup
	var writeWg sync.WaitGroup
	var firstErr error
	var once sync.Once

	setErr := func(err error) {
		if err == nil {
			return
		}
		once.Do(func() { firstErr = err })
	}

	writeWg.Add(1)
	go func() {
		defer writeWg.Done()
		writerStore, err := sqlite.Open(dbPath)
		if err != nil {
			setErr(err)
			return
		}
		defer writerStore.Close()
		if err := writerStore.EnsureWorkspace(workspaceID, rootAbs); err != nil {
			setErr(err)
			return
		}
		for plan := range plans {
			if firstErr != nil {
				continue
			}
			if err := indexer.ApplyUpdatePlan(writerStore, workspaceID, plan, nil); err != nil {
				setErr(err)
			}
		}
	}()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range jobs {
				if firstErr != nil {
					continue
				}
				if isDBRel(rel, dbRel) {
					continue
				}
				var plan indexer.UpdatePlan
				var err error
				if meta, ok := dbMeta[rel]; ok {
					plan, err = indexer.PrepareUpdatePlan(rootAbs, rel, opts, &meta, true)
				} else {
					plan, err = indexer.PrepareUpdatePlan(rootAbs, rel, opts, nil, false)
				}
				if err != nil {
					setErr(err)
					continue
				}
				if plan.Skip {
					continue
				}
				plans <- plan
			}
		}()
	}

	for _, rel := range files {
		if firstErr != nil {
			break
		}
		jobs <- rel
	}
	close(jobs)
	wg.Wait()
	close(plans)
	writeWg.Wait()

	return firstErr
}

func isDBRel(rel string, dbRel string) bool {
	if dbRel == "" {
		return false
	}
	switch rel {
	case dbRel, dbRel + "-wal", dbRel + "-shm", dbRel + "-journal":
		return true
	default:
		return false
	}
}

func normalizeWorkers(workers int) int {
	if workers <= 0 {
		workers = runtime.NumCPU() / 2
	}
	if workers < 1 {
		workers = 1
	}
	max := runtime.NumCPU()
	if max < 1 {
		max = 1
	}
	if workers > max {
		workers = max
	}
	return workers
}

func debounceFromParams(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
