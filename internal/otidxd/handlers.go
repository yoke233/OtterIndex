package otidxd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	queue  *updateQueue
	direct *directUpdater
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
	rootAbs := ws.root
	if !filepath.IsAbs(rootAbs) {
		if abs, err := filepath.Abs(rootAbs); err == nil {
			rootAbs = abs
		}
	}

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

	autoEnabled := true
	if p.AutoTune != nil && !*p.AutoTune {
		autoEnabled = false
	}

	tuning := defaultQueueTuning()
	autoParams := watchAutoParams{
		DebounceMS:       0,
		AdaptiveDebounce: true,
		DebounceMinMS:    50,
		DebounceMaxMS:    500,
		SyncWorkers:      0,
		QueueMode:        "simple",
		AutoTune:         true,
	}

	if autoEnabled {
		var err error
		tuning, autoParams, err = autoTuneWatch(ws.dbPath, wsid)
		if err != nil {
			return WatchStatusResult{}, err
		}
	}

	autoParams = applyAutoParams(p, autoParams)

	var uq *updateQueue
	var du *directUpdater
	updateFunc := func(paths []string) {
		for _, rel := range paths {
			_ = indexer.UpdateFile(rootAbs, ws.dbPath, rel, indexer.Options{
				WorkspaceID:  wsid,
				ScanAll:      p.ScanAll,
				IncludeGlobs: p.IncludeGlobs,
				ExcludeGlobs: p.ExcludeGlobs,
			})
		}
	}

	if autoParams.QueueMode == "direct" {
		du = newDirectUpdater(rootAbs, ws.dbPath, indexer.Options{
			WorkspaceID:  wsid,
			ScanAll:      p.ScanAll,
			IncludeGlobs: p.IncludeGlobs,
			ExcludeGlobs: p.ExcludeGlobs,
		})
		updateFunc = func(paths []string) {
			if du == nil {
				return
			}
			for _, rel := range paths {
				_ = du.Update(rel)
			}
		}
	} else {
		uq = newUpdateQueue(rootAbs, ws.dbPath, wsid, indexer.Options{
			WorkspaceID:  wsid,
			ScanAll:      p.ScanAll,
			IncludeGlobs: p.IncludeGlobs,
			ExcludeGlobs: p.ExcludeGlobs,
		}, tuning, autoParams.QueueMode)
		updateFunc = func(paths []string) {
			if uq != nil {
				uq.Enqueue(paths)
			}
		}
	}

	w, err := watch.NewWatcherWithOptions(ws.root, ws.dbPath, indexer.Options{
		WorkspaceID:  wsid,
		ScanAll:      p.ScanAll,
		IncludeGlobs: p.IncludeGlobs,
		ExcludeGlobs: p.ExcludeGlobs,
	}, watch.Options{
		Debounce:         debounceFromParams(autoParams.DebounceMS),
		AdaptiveDebounce: autoParams.AdaptiveDebounce,
		DebounceMin:      debounceFromParams(autoParams.DebounceMinMS),
		DebounceMax:      debounceFromParams(autoParams.DebounceMaxMS),
		UpdateFunc:       updateFunc,
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
	h.watchers[wsid] = &watcherEntry{w: w, cancel: cancel, done: done, queue: uq, direct: du}
	h.mu.Unlock()

	if autoParams.SyncOnStart {
		if err := syncChangedFiles(ws.root, ws.dbPath, indexer.Options{
			WorkspaceID:  wsid,
			ScanAll:      p.ScanAll,
			IncludeGlobs: p.IncludeGlobs,
			ExcludeGlobs: p.ExcludeGlobs,
		}, autoParams.SyncWorkers); err != nil {
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
		if entry.queue != nil {
			entry.queue.Close()
		}
		if entry.direct != nil {
			entry.direct.Close()
		}
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
		if entry.queue != nil {
			entry.queue.Close()
		}
		if entry.direct != nil {
			entry.direct.Close()
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

type watchAutoParams struct {
	DebounceMS       int
	AdaptiveDebounce bool
	DebounceMinMS    int
	DebounceMaxMS    int
	SyncWorkers      int
	SyncOnStart      bool
	QueueMode        string
	AutoTune         bool
}

func applyAutoParams(p WatchStartParams, auto watchAutoParams) watchAutoParams {
	base := watchAutoParams{
		DebounceMS:       200,
		AdaptiveDebounce: false,
		DebounceMinMS:    50,
		DebounceMaxMS:    500,
		SyncWorkers:      0,
		QueueMode:        "simple",
		AutoTune:         true,
	}

	useAuto := true
	if p.AutoTune != nil && !*p.AutoTune {
		useAuto = false
	}

	out := base
	if useAuto {
		out = auto
	}

	out.SyncOnStart = p.SyncOnStart
	if p.DebounceMS > 0 {
		out.DebounceMS = p.DebounceMS
		out.AdaptiveDebounce = false
	}
	if p.AdaptiveDebounce {
		out.AdaptiveDebounce = true
	}
	if p.DebounceMinMS > 0 {
		out.DebounceMinMS = p.DebounceMinMS
	}
	if p.DebounceMaxMS > 0 {
		out.DebounceMaxMS = p.DebounceMaxMS
	}
	if p.SyncWorkers > 0 {
		out.SyncWorkers = p.SyncWorkers
	}
	if mode := normalizeQueueMode(p.QueueMode); mode != "" {
		out.QueueMode = mode
	}
	return out
}

func autoTuneWatch(dbPath string, workspaceID string) (updateQueueTuning, watchAutoParams, error) {
	tuning := defaultQueueTuning()
	auto := watchAutoParams{
		DebounceMS:       0,
		AdaptiveDebounce: true,
		DebounceMinMS:    50,
		DebounceMaxMS:    500,
		SyncWorkers:      0,
		QueueMode:        "simple",
		AutoTune:         true,
	}

	s, err := sqlite.Open(dbPath)
	if err != nil {
		return tuning, auto, err
	}
	defer s.Close()

	count, total, err := s.GetFilesStats(workspaceID)
	if err != nil {
		return tuning, auto, err
	}
	if count <= 0 {
		return tuning, auto, nil
	}
	avg := total / int64(count)

	switch {
	case count < 5000 && avg > 512*1024:
		auto.AdaptiveDebounce = false
		auto.DebounceMS = 100
		auto.DebounceMinMS = 50
		auto.DebounceMaxMS = 200
		if auto.SyncWorkers == 0 {
			auto.SyncWorkers = 2
		}
		auto.QueueMode = "simple"
		tuning = updateQueueTuning{
			IntervalSmall: 40 * time.Millisecond,
			IntervalMed:   60 * time.Millisecond,
			IntervalLarge: 80 * time.Millisecond,
			IntervalXL:    120 * time.Millisecond,
			BatchSmall:    64,
			BatchMed:      128,
			BatchLarge:    256,
			BatchXL:       512,
		}
	case count > 20000 && avg < 128*1024:
		auto.AdaptiveDebounce = true
		auto.DebounceMinMS = 80
		auto.DebounceMaxMS = 800
		if auto.SyncWorkers == 0 {
			auto.SyncWorkers = 0
		}
		auto.QueueMode = "priority"
		tuning = updateQueueTuning{
			IntervalSmall: 80 * time.Millisecond,
			IntervalMed:   150 * time.Millisecond,
			IntervalLarge: 300 * time.Millisecond,
			IntervalXL:    600 * time.Millisecond,
			BatchSmall:    512,
			BatchMed:      1024,
			BatchLarge:    2048,
			BatchXL:       4096,
		}
	default:
		auto.QueueMode = "simple"
		tuning = defaultQueueTuning()
	}

	return tuning, auto, nil
}

type updateQueue struct {
	rootAbs     string
	dbPath      string
	opts        indexer.Options
	workspaceID string
	mode        string

	tuning      updateQueueTuning
	rateEnabled bool

	mu         sync.Mutex
	pending    map[string]struct{}
	hot        map[string]int
	ch         chan struct{}
	done       chan struct{}
	wg         sync.WaitGroup
	lastFlush  time.Time
	lastRate   time.Time
	events     int
	rateFactor float64
}

type updateQueueTuning struct {
	IntervalSmall time.Duration
	IntervalMed   time.Duration
	IntervalLarge time.Duration
	IntervalXL    time.Duration
	BatchSmall    int
	BatchMed      int
	BatchLarge    int
	BatchXL       int
}

func newUpdateQueue(rootAbs string, dbPath string, workspaceID string, opts indexer.Options, tuning updateQueueTuning, mode string) *updateQueue {
	if tuning.IntervalSmall <= 0 {
		tuning = defaultQueueTuning()
	}
	mode = normalizeQueueMode(mode)
	if mode == "" {
		mode = "simple"
	}
	q := &updateQueue{
		rootAbs:     rootAbs,
		dbPath:      dbPath,
		opts:        opts,
		workspaceID: workspaceID,
		mode:        mode,
		tuning:      tuning,
		rateEnabled: mode == "priority",
		pending:     map[string]struct{}{},
		hot:         map[string]int{},
		ch:          make(chan struct{}, 64),
		done:        make(chan struct{}),
		rateFactor:  1,
	}
	if mode != "priority" {
		q.hot = nil
	}
	q.wg.Add(1)
	go q.run()
	return q
}

func (q *updateQueue) Enqueue(paths []string) {
	if q == nil || len(paths) == 0 {
		return
	}
	q.mu.Lock()
	for _, p := range paths {
		q.pending[p] = struct{}{}
		if q.hot != nil {
			q.hot[p]++
		}
	}
	q.events += len(paths)
	q.mu.Unlock()

	select {
	case q.ch <- struct{}{}:
	default:
	}
}

func (q *updateQueue) Close() {
	if q == nil {
		return
	}
	close(q.done)
	q.wg.Wait()
}

func (q *updateQueue) run() {
	defer q.wg.Done()

	store, err := sqlite.Open(q.dbPath)
	if err != nil {
		return
	}
	defer store.Close()
	if err := store.EnsureWorkspace(q.workspaceID, q.rootAbs); err != nil {
		return
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	flush := func(force bool) {
		q.mu.Lock()
		n := len(q.pending)
		if n == 0 {
			q.mu.Unlock()
			return
		}
		interval, maxBatch := q.desired(n)
		if !force && time.Since(q.lastFlush) < interval {
			q.mu.Unlock()
			return
		}
		paths := make([]string, 0, len(q.pending))
		hot := map[string]int{}
		for p := range q.pending {
			paths = append(paths, p)
			if q.hot != nil {
				if c, ok := q.hot[p]; ok {
					hot[p] = c
					delete(q.hot, p)
				}
			}
		}
		q.pending = map[string]struct{}{}
		q.lastFlush = time.Now()
		q.mu.Unlock()

		ordered := q.prioritize(paths, hot)
		batch := make([]indexer.UpdatePlan, 0, maxBatch)
		for _, rel := range ordered {
			meta, ok, err := store.GetFileMeta(q.workspaceID, rel)
			if err != nil {
				continue
			}
			var plan indexer.UpdatePlan
			if ok {
				plan, err = indexer.PrepareUpdatePlan(q.rootAbs, rel, q.opts, &meta, true)
			} else {
				plan, err = indexer.PrepareUpdatePlan(q.rootAbs, rel, q.opts, nil, false)
			}
			if err != nil || plan.Skip {
				continue
			}
			batch = append(batch, plan)
			if len(batch) >= maxBatch {
				_ = indexer.ApplyUpdatePlansBatch(store, q.workspaceID, batch, nil)
				batch = batch[:0]
			}
		}
		if len(batch) > 0 {
			_ = indexer.ApplyUpdatePlansBatch(store, q.workspaceID, batch, nil)
		}
	}

	for {
		select {
		case <-q.done:
			flush(true)
			return
		case <-q.ch:
			q.adjustRate()
			q.mu.Lock()
			n := len(q.pending)
			q.mu.Unlock()
			_, maxBatch := q.desired(n)
			if n >= maxBatch {
				flush(true)
			}
		case <-ticker.C:
			q.adjustRate()
			flush(false)
		}
	}
}

func (q *updateQueue) desired(n int) (time.Duration, int) {
	factor := 1.0
	if q.rateEnabled {
		factor = q.rateFactor
	}
	if factor <= 0 {
		factor = 1
	}
	switch {
	case n <= 50:
		return scaleInterval(q.tuning.IntervalSmall, factor), scaleBatch(q.tuning.BatchSmall, factor)
	case n <= 200:
		return scaleInterval(q.tuning.IntervalMed, factor), scaleBatch(q.tuning.BatchMed, factor)
	case n <= 1000:
		return scaleInterval(q.tuning.IntervalLarge, factor), scaleBatch(q.tuning.BatchLarge, factor)
	default:
		return scaleInterval(q.tuning.IntervalXL, factor), scaleBatch(q.tuning.BatchXL, factor)
	}
}

func (q *updateQueue) prioritize(paths []string, hot map[string]int) []string {
	if len(paths) == 0 {
		return nil
	}

	type item struct {
		path  string
		hot   int
		depth int
		size  int64
	}

	items := make([]item, 0, len(paths))
	for _, rel := range paths {
		full := filepath.Join(q.rootAbs, filepath.FromSlash(rel))
		st, err := os.Stat(full)
		size := int64(0)
		if err == nil {
			size = st.Size()
		}
		hotCount := 0
		if hot != nil {
			hotCount = hot[rel]
		}
		items = append(items, item{
			path:  rel,
			hot:   hotCount,
			depth: strings.Count(rel, "/"),
			size:  size,
		})
	}

	if q.mode == "priority" {
		sort.Slice(items, func(i, j int) bool {
			if items[i].hot != items[j].hot {
				return items[i].hot > items[j].hot
			}
			if items[i].depth != items[j].depth {
				return items[i].depth < items[j].depth
			}
			if items[i].size != items[j].size {
				return items[i].size < items[j].size
			}
			return items[i].path < items[j].path
		})
	} else {
		sort.Slice(items, func(i, j int) bool {
			if items[i].size != items[j].size {
				return items[i].size < items[j].size
			}
			if items[i].depth != items[j].depth {
				return items[i].depth < items[j].depth
			}
			return items[i].path < items[j].path
		})
	}

	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.path)
	}
	return out
}

func defaultQueueTuning() updateQueueTuning {
	return updateQueueTuning{
		IntervalSmall: 50 * time.Millisecond,
		IntervalMed:   100 * time.Millisecond,
		IntervalLarge: 200 * time.Millisecond,
		IntervalXL:    400 * time.Millisecond,
		BatchSmall:    256,
		BatchMed:      512,
		BatchLarge:    1024,
		BatchXL:       2048,
	}
}

func normalizeQueueMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "direct":
		return "direct"
	case "simple":
		return "simple"
	case "priority":
		return "priority"
	case "":
		return ""
	default:
		return ""
	}
}

type directUpdater struct {
	rootAbs string
	opts    indexer.Options
	store   *sqlite.Store
	mu      sync.Mutex
}

func newDirectUpdater(rootAbs string, dbPath string, opts indexer.Options) *directUpdater {
	s, err := sqlite.Open(dbPath)
	if err != nil {
		return nil
	}
	if err := s.EnsureWorkspace(opts.WorkspaceID, rootAbs); err != nil {
		_ = s.Close()
		return nil
	}
	return &directUpdater{
		rootAbs: rootAbs,
		opts:    opts,
		store:   s,
	}
}

func (d *directUpdater) Update(rel string) error {
	if d == nil || d.store == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return indexer.UpdateFileWithStore(d.store, d.rootAbs, rel, d.opts, nil, false)
}

func (d *directUpdater) Close() {
	if d == nil || d.store == nil {
		return
	}
	_ = d.store.Close()
}

func (q *updateQueue) adjustRate() {
	if !q.rateEnabled {
		q.rateFactor = 1
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	if q.lastRate.IsZero() {
		q.lastRate = now
		q.events = 0
		q.rateFactor = 1
		return
	}
	elapsed := now.Sub(q.lastRate)
	if elapsed < 500*time.Millisecond {
		return
	}
	rate := float64(q.events) / elapsed.Seconds()
	q.events = 0
	q.lastRate = now

	switch {
	case rate < 20:
		q.rateFactor = 1
	case rate < 100:
		q.rateFactor = 1.5
	case rate < 500:
		q.rateFactor = 2
	default:
		q.rateFactor = 3
	}
}

func scaleInterval(d time.Duration, factor float64) time.Duration {
	if factor <= 1 {
		return d
	}
	out := time.Duration(float64(d) * factor)
	if out > 2*time.Second {
		return 2 * time.Second
	}
	return out
}

func scaleBatch(v int, factor float64) int {
	if v <= 0 {
		return v
	}
	out := int(float64(v) * factor)
	if out < 32 {
		return 32
	}
	if out > 8192 {
		return 8192
	}
	return out
}
