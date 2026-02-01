package watch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"otterindex/internal/core/indexer"
	"otterindex/internal/core/walk"
)

type Watcher struct {
	rootAbs string
	dbPath  string
	dbRel   string

	indexerOpts indexer.Options
	filter      *walk.Filter
	debouncer   *Debouncer
	debounce    time.Duration

	watcher   *fsnotify.Watcher
	closeOnce sync.Once
	closed    chan struct{}
}

type Options struct {
	Debounce         time.Duration
	AdaptiveDebounce bool
	DebounceMin      time.Duration
	DebounceMax      time.Duration
	UpdateFunc       func(paths []string)
}

func NewWatcher(root string, dbPath string, opts indexer.Options) (*Watcher, error) {
	return NewWatcherWithOptions(root, dbPath, opts, Options{})
}

func NewWatcherWithOptions(root string, dbPath string, opts indexer.Options, wopts Options) (*Watcher, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	rootAbs = filepath.Clean(rootAbs)
	if strings.TrimSpace(rootAbs) == "" {
		return nil, fmt.Errorf("root is required")
	}
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("dbPath is required")
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

	filter, err := walk.NewFilter(rootAbs, walk.Options{
		IncludeGlobs: opts.IncludeGlobs,
		ExcludeGlobs: opts.ExcludeGlobs,
		ScanAll:      opts.ScanAll,
	})
	if err != nil {
		return nil, err
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	debounce := wopts.Debounce
	if debounce <= 0 {
		debounce = 200 * time.Millisecond
	}
	minDelay := wopts.DebounceMin
	if minDelay <= 0 {
		minDelay = 50 * time.Millisecond
	}
	maxDelay := wopts.DebounceMax
	if maxDelay <= 0 {
		maxDelay = 500 * time.Millisecond
	}
	if maxDelay < minDelay {
		maxDelay = minDelay
	}

	w := &Watcher{
		rootAbs:     rootAbs,
		dbPath:      dbPath,
		dbRel:       dbRel,
		indexerOpts: opts,
		filter:      filter,
		debouncer:   NewDebouncer(debounce),
		debounce:    debounce,
		watcher:     fsw,
		closed:      make(chan struct{}),
	}
	if wopts.AdaptiveDebounce {
		w.debouncer.SetDelayFunc(func(count int) time.Duration {
			switch {
			case count <= 10:
				return minDelay
			case count <= 100:
				return minDelay * 2
			case count <= 500:
				return minDelay * 4
			default:
				return maxDelay
			}
		})
	}
	if wopts.UpdateFunc != nil {
		w.debouncer.OnFire(wopts.UpdateFunc)
	} else {
		w.debouncer.OnFire(func(paths []string) {
			for _, rel := range paths {
				_ = indexer.UpdateFile(w.rootAbs, w.dbPath, rel, w.indexerOpts)
			}
		})
	}

	if err := w.addExistingDirs(); err != nil {
		_ = fsw.Close()
		return nil, err
	}

	return w, nil
}

func (w *Watcher) Debounce() time.Duration {
	if w == nil {
		return 0
	}
	return w.debounce
}

func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}

	w.closeOnce.Do(func() { close(w.closed) })

	if w.watcher == nil {
		return nil
	}
	return w.watcher.Close()
}

func (w *Watcher) Run(ctx context.Context) error {
	if w == nil || w.watcher == nil {
		return fmt.Errorf("watcher is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.closed:
			return nil
		case ev, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}
			w.handleEvent(ev)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}
			return err
		}
	}
}

func (w *Watcher) addExistingDirs() error {
	return filepath.WalkDir(w.rootAbs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if p == w.rootAbs {
			return w.watcher.Add(p)
		}

		rel, err := filepath.Rel(w.rootAbs, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !w.filter.ShouldInclude(rel, true) {
			return filepath.SkipDir
		}

		return w.watcher.Add(p)
	})
}

func (w *Watcher) handleEvent(ev fsnotify.Event) {
	rel, ok := w.toRel(ev.Name)
	if !ok {
		return
	}
	if w.isDBRel(rel) {
		return
	}

	if ev.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
		if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
			_ = w.addDirRecursive(ev.Name)
			return
		}
	}

	if !w.filter.ShouldInclude(rel, false) {
		return
	}

	switch {
	case ev.Op&fsnotify.Write != 0:
		w.debouncer.Push(rel)
	case ev.Op&fsnotify.Create != 0:
		w.debouncer.Push(rel)
	case ev.Op&fsnotify.Remove != 0:
		w.debouncer.Push(rel)
	case ev.Op&fsnotify.Rename != 0:
		w.debouncer.Push(rel)
	}
}

func (w *Watcher) toRel(abs string) (string, bool) {
	if strings.TrimSpace(abs) == "" {
		return "", false
	}

	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(w.rootAbs, abs)
	if err != nil {
		return "", false
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	return rel, true
}

func (w *Watcher) isDBRel(rel string) bool {
	if w.dbRel == "" {
		return false
	}
	switch rel {
	case w.dbRel, w.dbRel + "-wal", w.dbRel + "-shm", w.dbRel + "-journal":
		return true
	default:
		return false
	}
}

func (w *Watcher) addDirRecursive(absDir string) error {
	absDir = filepath.Clean(absDir)

	return filepath.WalkDir(absDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		rel, ok := w.toRel(p)
		if !ok {
			return nil
		}
		if !w.filter.ShouldInclude(rel, true) {
			return filepath.SkipDir
		}
		return w.watcher.Add(p)
	})
}
