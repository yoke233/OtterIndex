package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"otterindex/internal/core/explain"
	"otterindex/internal/core/treesitter"
	"otterindex/internal/core/walk"
	"otterindex/internal/index/backend"
	"otterindex/internal/index/store"
)

type Options struct {
	Store        string
	WorkspaceID  string
	Workers      int
	ScanAll      bool
	IncludeGlobs []string
	ExcludeGlobs []string

	ChunkLines   int
	ChunkOverlap int

	Explain explain.Explain
}

func Build(root string, dbPath string, opts Options) error {
	ex := opts.Explain
	startTotal := time.Now()

	root = filepath.Clean(root)
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("root is required")
	}
	if strings.TrimSpace(dbPath) == "" {
		return fmt.Errorf("dbPath is required")
	}

	workspaceID := strings.TrimSpace(opts.WorkspaceID)
	if workspaceID == "" {
		workspaceID = root
	}

	if ex != nil {
		ex.KV("phase", "build")
		ex.KV("store", opts.Store)
		ex.KV("workspace_id", workspaceID)
		ex.KV("root", root)
		ex.KV("db_path", dbPath)
		ex.KV("scan_all", opts.ScanAll)
		ex.KV("include_globs", opts.IncludeGlobs)
		ex.KV("exclude_globs", opts.ExcludeGlobs)
	}

	rootAbs := root
	if !filepath.IsAbs(rootAbs) {
		if abs, err := filepath.Abs(rootAbs); err == nil {
			rootAbs = abs
		}
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

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU() / 2
		if workers < 1 {
			workers = 1
		}
	}
	if ex != nil {
		ex.KV("workers", workers)
	}

	chunkLines, overlap, step := resolveChunkParams(opts)
	if ex != nil {
		ex.KV("chunk_lines", chunkLines)
		ex.KV("chunk_overlap", overlap)
		ex.KV("chunk_step", step)
	}

	s, err := backend.Open(opts.Store, dbPath)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.EnsureWorkspace(workspaceID, root); err != nil {
		return err
	}
	if applier, ok := s.(store.BuildPragmaApplier); ok {
		if err := applier.ApplyBuildPragmas(); err != nil {
			return err
		}
	}
	if ex != nil {
		ex.KV("fts5", s.HasFTS())
		ex.KV("fts5_reason", s.FTSReason())

		if pr, ok := s.(store.PragmaReader); ok {
			jm, _ := pr.QueryPragma("journal_mode")
			syncMode, _ := pr.QueryPragma("synchronous")
			tempStore, _ := pr.QueryPragma("temp_store")
			cacheSize, _ := pr.QueryPragma("cache_size")
			ex.KV("sqlite_journal_mode", jm)
			ex.KV("sqlite_synchronous", syncMode)
			ex.KV("sqlite_temp_store", tempStore)
			ex.KV("sqlite_cache_size", cacheSize)
		}
	}

	stopWalk := func() {}
	if ex != nil {
		stopWalk = ex.Timer("walk")
	}
	files, err := walk.ListFiles(root, walk.Options{
		IncludeGlobs: opts.IncludeGlobs,
		ExcludeGlobs: opts.ExcludeGlobs,
		ScanAll:      opts.ScanAll,
	})
	stopWalk()
	if err != nil {
		return err
	}
	if ex != nil {
		ex.KV("files_total", len(files))
	}

	type parsedFile struct {
		rel      string
		size     int64
		mtime    int64
		hash     string
		chunks   []store.ChunkInput
		symbols  []store.SymbolInput
		comments []store.CommentInput
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sendErr := func(err error, errCh chan error) {
		select {
		case errCh <- err:
		default:
		}
	}

	jobs := make(chan string, workers*2)
	parsed := make(chan parsedFile, workers*2)
	errCh := make(chan error, 1)

	var skippedDB int64
	var skippedBinary int64
	var filesIndexed int64
	var chunksWritten int64
	var symbolsWritten int64
	var commentsWritten int64
	var treesitterDisabled int64
	var treesitterUnsupported int64
	var treesitterErrors int64

	var writerWG sync.WaitGroup
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		batchSize := workers * 2
		if batchSize < 4 {
			batchSize = 4
		}
		if batchSize > 64 {
			batchSize = 64
		}
		docLimit := 0
		if s.Backend() == "bleve" {
			if batchSize > 8 {
				batchSize = 8
			}
			docLimit = 2000
		}
		batch := make([]store.FilePlan, 0, batchSize)
		batchDocs := 0
		flush := func() bool {
			if len(batch) == 0 {
				return true
			}
			stopWrite := func() {}
			if ex != nil {
				stopWrite = ex.Timer("write")
			}
			if err := s.ReplaceFilesBatch(workspaceID, batch); err != nil {
				sendErr(err, errCh)
				cancel()
				stopWrite()
				return false
			}
			stopWrite()
			for _, plan := range batch {
				if plan.Delete {
					continue
				}
				atomic.AddInt64(&filesIndexed, 1)
				atomic.AddInt64(&chunksWritten, int64(len(plan.Chunks)))
				atomic.AddInt64(&symbolsWritten, int64(len(plan.Syms)))
				atomic.AddInt64(&commentsWritten, int64(len(plan.Comms)))
			}
			batch = batch[:0]
			batchDocs = 0
			return true
		}
		for {
			select {
			case <-ctx.Done():
				return
			case pf, ok := <-parsed:
				if !ok {
					_ = flush()
					return
				}
				plan := store.FilePlan{
					Path:   filepath.ToSlash(pf.rel),
					Size:   pf.size,
					MTime:  pf.mtime,
					Hash:   pf.hash,
					Chunks: pf.chunks,
					Syms:   pf.symbols,
					Comms:  pf.comments,
				}
				batch = append(batch, plan)
				batchDocs += len(plan.Chunks) + len(plan.Syms) + len(plan.Comms)
				if len(batch) >= batchSize || (docLimit > 0 && batchDocs >= docLimit) {
					if !flush() {
						return
					}
				}
			}
		}
	}()

	var workersWG sync.WaitGroup
	workersWG.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer workersWG.Done()

			ts := treesitter.NewProvider()
			for {
				select {
				case <-ctx.Done():
					return
				case rel, ok := <-jobs:
					if !ok {
						return
					}

					stopParse := func() {}
					if ex != nil {
						stopParse = ex.Timer("read_parse")
					}
					full := filepath.Join(root, filepath.FromSlash(rel))
					st, err := os.Stat(full)
					if err != nil {
						sendErr(err, errCh)
						cancel()
						stopParse()
						return
					}

					b, err := os.ReadFile(full)
					if err != nil {
						sendErr(err, errCh)
						cancel()
						stopParse()
						return
					}
					if isBinary(b) {
						atomic.AddInt64(&skippedBinary, 1)
						stopParse()
						continue
					}

					hash := hashText(b)
					chunks := chunkByLines(string(b), chunkLines, step)

					syms, comms, tsErr := ts.Extract(rel, b)
					if tsErr != nil {
						if errors.Is(tsErr, treesitter.ErrDisabled) {
							atomic.AddInt64(&treesitterDisabled, 1)
						} else if errors.Is(tsErr, treesitter.ErrUnsupported) {
							atomic.AddInt64(&treesitterUnsupported, 1)
						} else {
							atomic.AddInt64(&treesitterErrors, 1)
						}
						syms = nil
						comms = nil
					}
					stopParse()

					select {
					case <-ctx.Done():
						return
					case parsed <- parsedFile{
						rel:      rel,
						size:     st.Size(),
						mtime:    st.ModTime().Unix(),
						hash:     hash,
						chunks:   chunks,
						symbols:  syms,
						comments: comms,
					}:
					}
				}
			}
		}()
	}

feed:
	for _, rel := range files {
		if dbRel != "" {
			switch rel {
			case dbRel, dbRel + "-wal", dbRel + "-shm", dbRel + "-journal":
				atomic.AddInt64(&skippedDB, 1)
				continue
			}
		}

		select {
		case <-ctx.Done():
			break feed
		case jobs <- rel:
		}
	}
	close(jobs)

	workersWG.Wait()
	close(parsed)
	writerWG.Wait()

	select {
	case err := <-errCh:
		return err
	default:
	}

	if err := s.BumpVersion(workspaceID); err != nil {
		return err
	}

	if ex != nil {
		ex.KV("files_skipped_db", skippedDB)
		ex.KV("files_skipped_binary", skippedBinary)
		ex.KV("files_indexed", filesIndexed)
		ex.KV("chunks_written", chunksWritten)
		ex.KV("symbols_written", symbolsWritten)
		ex.KV("comments_written", commentsWritten)
		ex.KV("treesitter_disabled", treesitterDisabled)
		ex.KV("treesitter_unsupported", treesitterUnsupported)
		ex.KV("treesitter_errors", treesitterErrors)
		ex.KV("elapsed_ms_total", time.Since(startTotal).Milliseconds())
	}

	return nil
}

func UpdateFile(root string, dbPath string, rel string, opts Options) error {
	if strings.TrimSpace(dbPath) == "" {
		return fmt.Errorf("dbPath is required")
	}

	s, err := backend.Open(opts.Store, dbPath)
	if err != nil {
		return err
	}
	defer s.Close()

	return UpdateFileWithStore(s, root, rel, opts, nil, false)
}

func UpdateFileWithStore(s store.Store, root string, rel string, opts Options, old *store.File, oldOK bool) error {
	ex := opts.Explain

	root = filepath.Clean(root)
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("root is required")
	}
	if s == nil {
		return fmt.Errorf("store is required")
	}

	rel = filepath.ToSlash(strings.TrimSpace(rel))
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" || rel == "." {
		return fmt.Errorf("rel path is required")
	}

	workspaceID := strings.TrimSpace(opts.WorkspaceID)
	if workspaceID == "" {
		workspaceID = root
	}

	if err := s.EnsureWorkspace(workspaceID, root); err != nil {
		return err
	}

	var meta store.File
	var ok bool
	var err error
	if oldOK && old != nil {
		meta = *old
		ok = true
	} else {
		meta, ok, err = s.GetFileMeta(workspaceID, rel)
		if err != nil {
			return err
		}
	}
	plan, err := PrepareUpdatePlan(root, rel, opts, &meta, ok)
	if err != nil {
		return err
	}
	return ApplyUpdatePlan(s, workspaceID, plan, ex)
}

type UpdatePlan struct {
	Rel    string
	Size   int64
	MTime  int64
	Hash   string
	Chunks []store.ChunkInput
	Syms   []store.SymbolInput
	Comms  []store.CommentInput
	Delete bool
	Skip   bool
}

func PrepareUpdatePlan(root string, rel string, opts Options, old *store.File, oldOK bool) (UpdatePlan, error) {
	root = filepath.Clean(root)
	if strings.TrimSpace(root) == "" {
		return UpdatePlan{}, fmt.Errorf("root is required")
	}

	rel = filepath.ToSlash(strings.TrimSpace(rel))
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" || rel == "." {
		return UpdatePlan{}, fmt.Errorf("rel path is required")
	}

	chunkLines, _, step := resolveChunkParams(opts)

	full := filepath.Join(root, filepath.FromSlash(rel))
	st, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return UpdatePlan{Rel: rel, Delete: true}, nil
		}
		return UpdatePlan{}, err
	}

	size := st.Size()
	mtime := st.ModTime().Unix()

	if oldOK && old != nil && old.Size == size && old.MTime == mtime {
		return UpdatePlan{Rel: rel, Skip: true}, nil
	}

	b, err := os.ReadFile(full)
	if err != nil {
		return UpdatePlan{}, err
	}
	if isBinary(b) {
		return UpdatePlan{Rel: rel, Delete: true}, nil
	}

	hash := hashText(b)
	if oldOK && old != nil && old.Hash != "" && old.Hash == hash {
		return UpdatePlan{Rel: rel, Skip: true}, nil
	}

	chunks := chunkByLines(string(b), chunkLines, step)
	ts := treesitter.NewProvider()
	syms, comms, _ := ts.Extract(rel, b)

	return UpdatePlan{
		Rel:    rel,
		Size:   size,
		MTime:  mtime,
		Hash:   hash,
		Chunks: chunks,
		Syms:   syms,
		Comms:  comms,
	}, nil
}

func ApplyUpdatePlan(s store.Store, workspaceID string, plan UpdatePlan, ex explain.Explain) error {
	return ApplyUpdatePlansBatch(s, workspaceID, []UpdatePlan{plan}, ex)
}

func ApplyUpdatePlansBatch(s store.Store, workspaceID string, plans []UpdatePlan, ex explain.Explain) error {
	if len(plans) == 0 {
		return nil
	}

	batch := make([]store.FilePlan, 0, len(plans))
	for _, plan := range plans {
		if plan.Skip {
			continue
		}
		batch = append(batch, store.FilePlan{
			Path:   plan.Rel,
			Size:   plan.Size,
			MTime:  plan.MTime,
			Hash:   plan.Hash,
			Chunks: plan.Chunks,
			Syms:   plan.Syms,
			Comms:  plan.Comms,
			Delete: plan.Delete,
		})
	}
	if len(batch) == 0 {
		return nil
	}

	stopWrite := func() {}
	if ex != nil {
		stopWrite = ex.Timer("write_batch")
	}
	err := s.ReplaceFilesBatch(workspaceID, batch)
	stopWrite()
	return err
}

func chunkByLines(text string, chunkLines int, step int) []store.ChunkInput {
	lines := splitLines(text)
	if len(lines) == 0 {
		return nil
	}

	var out []store.ChunkInput
	for start := 0; start < len(lines); start += step {
		end := start + chunkLines
		if end > len(lines) {
			end = len(lines)
		}
		chunkText := strings.Join(lines[start:end], "\n")
		out = append(out, store.ChunkInput{
			SL:    start + 1,
			EL:    end,
			Kind:  "chunk",
			Title: "",
			Text:  chunkText,
		})
		if end == len(lines) {
			break
		}
	}
	return out
}

func resolveChunkParams(opts Options) (int, int, int) {
	chunkLines := opts.ChunkLines
	if chunkLines <= 0 {
		chunkLines = 40
	}
	overlap := opts.ChunkOverlap
	if overlap < 0 {
		overlap = 0
	}
	step := chunkLines - overlap
	if step <= 0 {
		step = chunkLines
	}
	return chunkLines, overlap, step
}

func hashText(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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

func isBinary(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}
