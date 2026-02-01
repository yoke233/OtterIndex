package bleve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	bquery "github.com/blevesearch/bleve/v2/search/query"
	"go.etcd.io/bbolt"

	"otterindex/internal/index/store"
	"otterindex/internal/model"
)

const (
	docTypeChunk   = "chunk"
	docTypeSymbol  = "symbol"
	docTypeComment = "comment"
)

type Store struct {
	mu       sync.Mutex
	path     string
	metaPath string
	idx      bleve.Index
	meta     *bbolt.DB
}

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("dbPath is required")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}

	var idx bleve.Index
	if _, err := os.Stat(filepath.Join(path, "index_meta.json")); err == nil {
		idx, err = bleve.Open(path)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		idx, err = bleve.New(path, buildMapping())
		if err != nil {
			return nil, err
		}
	}

	metaPath := filepath.Join(path, "otidx-meta.db")
	meta, err := bbolt.Open(metaPath, 0o600, nil)
	if err != nil {
		_ = idx.Close()
		return nil, err
	}

	s := &Store{path: path, metaPath: metaPath, idx: idx, meta: meta}
	if err := s.ensureBuckets(); err != nil {
		_ = meta.Close()
		_ = idx.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if s.idx != nil {
		_ = s.idx.Close()
	}
	if s.meta != nil {
		_ = s.meta.Close()
	}
	return nil
}

func (s *Store) Backend() string { return "bleve" }

func (s *Store) HasFTS() bool { return true }

func (s *Store) FTSReason() string { return "bleve" }

func (s *Store) EnsureWorkspace(id string, root string) error {
	if s == nil || s.meta == nil {
		return fmt.Errorf("store is not open")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("workspaceID is required")
	}
	root = strings.TrimSpace(root)

	return s.meta.Update(func(tx *bbolt.Tx) error {
		wb := mustBucket(tx, bucketWorkspaces)
		fb := mustBucket(tx, bucketFiles)
		if _, err := fb.CreateBucketIfNotExists([]byte(id)); err != nil {
			return err
		}

		meta := workspaceMeta{}
		raw := wb.Get([]byte(id))
		if raw != nil {
			if err := decode(raw, &meta); err != nil {
				return err
			}
		}
		if meta.ID == "" {
			meta.ID = id
		}
		if meta.CreatedAt == 0 {
			meta.CreatedAt = nowUnix()
		}
		if meta.Version == 0 {
			meta.Version = 1
		}
		if root != "" {
			meta.Root = root
		}
		buf, err := encode(meta)
		if err != nil {
			return err
		}
		return wb.Put([]byte(id), buf)
	})
}

func (s *Store) GetVersion(workspaceID string) (int64, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspaceID is required")
	}
	if err := s.EnsureWorkspace(workspaceID, ""); err != nil {
		return 0, err
	}

	var ver int64
	err := s.meta.View(func(tx *bbolt.Tx) error {
		wb := mustBucket(tx, bucketWorkspaces)
		raw := wb.Get([]byte(workspaceID))
		if raw == nil {
			return fmt.Errorf("workspace not found")
		}
		meta := workspaceMeta{}
		if err := decode(raw, &meta); err != nil {
			return err
		}
		ver = meta.Version
		return nil
	})
	return ver, err
}

func (s *Store) BumpVersion(workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	return s.meta.Update(func(tx *bbolt.Tx) error {
		wb := mustBucket(tx, bucketWorkspaces)
		raw := wb.Get([]byte(workspaceID))
		meta := workspaceMeta{ID: workspaceID, CreatedAt: nowUnix(), Version: 1}
		if raw != nil {
			if err := decode(raw, &meta); err != nil {
				return err
			}
			if meta.ID == "" {
				meta.ID = workspaceID
			}
			if meta.CreatedAt == 0 {
				meta.CreatedAt = nowUnix()
			}
			if meta.Version <= 0 {
				meta.Version = 1
			} else {
				meta.Version++
			}
		}
		buf, err := encode(meta)
		if err != nil {
			return err
		}
		return wb.Put([]byte(workspaceID), buf)
	})
}

func (s *Store) GetWorkspace(workspaceID string) (store.Workspace, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return store.Workspace{}, fmt.Errorf("workspaceID is required")
	}
	var ws store.Workspace
	err := s.meta.View(func(tx *bbolt.Tx) error {
		wb := mustBucket(tx, bucketWorkspaces)
		raw := wb.Get([]byte(workspaceID))
		if raw == nil {
			return fmt.Errorf("workspace not found")
		}
		meta := workspaceMeta{}
		if err := decode(raw, &meta); err != nil {
			return err
		}
		ws = store.Workspace{
			ID:        meta.ID,
			Root:      meta.Root,
			CreatedAt: meta.CreatedAt,
		}
		return nil
	})
	return ws, err
}

func (s *Store) UpsertFile(workspaceID string, path string, size int64, mtime int64, hash string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	hash = strings.TrimSpace(hash)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}
	if err := s.EnsureWorkspace(workspaceID, ""); err != nil {
		return err
	}

	return s.meta.Update(func(tx *bbolt.Tx) error {
		fb := mustFileBucket(tx, workspaceID)
		meta := fileMeta{
			Size:  size,
			MTime: mtime,
			Hash:  hash,
		}
		if raw := fb.Get([]byte(path)); raw != nil {
			if err := decode(raw, &meta); err != nil {
				return err
			}
			meta.Size = size
			meta.MTime = mtime
			meta.Hash = hash
		}
		buf, err := encode(meta)
		if err != nil {
			return err
		}
		return fb.Put([]byte(path), buf)
	})
}

func (s *Store) GetFile(workspaceID string, path string) (store.File, error) {
	f, ok, err := s.GetFileMeta(workspaceID, path)
	if err != nil {
		return store.File{}, err
	}
	if !ok {
		return store.File{}, fmt.Errorf("file not found")
	}
	return f, nil
}

func (s *Store) GetFileMeta(workspaceID string, path string) (store.File, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	if workspaceID == "" {
		return store.File{}, false, fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return store.File{}, false, fmt.Errorf("path is required")
	}
	var meta fileMeta
	var ok bool
	err := s.meta.View(func(tx *bbolt.Tx) error {
		fb := fileBucket(tx, workspaceID)
		if fb == nil {
			return nil
		}
		raw := fb.Get([]byte(path))
		if raw == nil {
			return nil
		}
		ok = true
		return decode(raw, &meta)
	})
	if err != nil {
		return store.File{}, false, err
	}
	if !ok {
		return store.File{}, false, nil
	}
	return store.File{
		WorkspaceID: workspaceID,
		Path:        path,
		Size:        meta.Size,
		MTime:       meta.MTime,
		Hash:        meta.Hash,
	}, true, nil
}

func (s *Store) ListFilesMeta(workspaceID string) (map[string]store.File, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceID is required")
	}
	out := map[string]store.File{}
	err := s.meta.View(func(tx *bbolt.Tx) error {
		fb := fileBucket(tx, workspaceID)
		if fb == nil {
			return nil
		}
		return fb.ForEach(func(k, v []byte) error {
			meta := fileMeta{}
			if err := decode(v, &meta); err != nil {
				return err
			}
			path := string(k)
			out[path] = store.File{
				WorkspaceID: workspaceID,
				Path:        path,
				Size:        meta.Size,
				MTime:       meta.MTime,
				Hash:        meta.Hash,
			}
			return nil
		})
	})
	return out, err
}

func (s *Store) GetFilesStats(workspaceID string) (int, int64, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, 0, fmt.Errorf("workspaceID is required")
	}
	var count int
	var total int64
	err := s.meta.View(func(tx *bbolt.Tx) error {
		fb := fileBucket(tx, workspaceID)
		if fb == nil {
			return nil
		}
		return fb.ForEach(func(_ []byte, v []byte) error {
			meta := fileMeta{}
			if err := decode(v, &meta); err != nil {
				return err
			}
			count++
			total += meta.Size
			return nil
		})
	})
	return count, total, err
}

func (s *Store) DeleteFile(workspaceID string, path string) error {
	return s.DeleteFileAll(workspaceID, path)
}

func (s *Store) ReplaceChunksBatch(workspaceID string, path string, chunks []store.ChunkInput) error {
	return s.replaceOne(workspaceID, path, replaceParts{chunks: chunks, chunksSet: true})
}

func (s *Store) ReplaceSymbolsBatch(workspaceID string, path string, syms []store.SymbolInput) error {
	return s.replaceOne(workspaceID, path, replaceParts{syms: syms, symsSet: true})
}

func (s *Store) ReplaceCommentsBatch(workspaceID string, path string, comms []store.CommentInput) error {
	return s.replaceOne(workspaceID, path, replaceParts{comms: comms, commsSet: true})
}

func (s *Store) ReplaceFileAll(workspaceID string, path string, size int64, mtime int64, hash string, chunks []store.ChunkInput, syms []store.SymbolInput, comms []store.CommentInput) error {
	plan := store.FilePlan{
		Path:   filepath.ToSlash(path),
		Size:   size,
		MTime:  mtime,
		Hash:   hash,
		Chunks: chunks,
		Syms:   syms,
		Comms:  comms,
		Delete: false,
	}
	return s.ReplaceFilesBatch(workspaceID, []store.FilePlan{plan})
}

func (s *Store) DeleteFileAll(workspaceID string, path string) error {
	plan := store.FilePlan{Path: filepath.ToSlash(path), Delete: true}
	return s.ReplaceFilesBatch(workspaceID, []store.FilePlan{plan})
}

func (s *Store) ReplaceFilesBatch(workspaceID string, plans []store.FilePlan) error {
	if len(plans) == 0 {
		return nil
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	if err := s.EnsureWorkspace(workspaceID, ""); err != nil {
		return err
	}

	paths := make([]string, 0, len(plans))
	for i := range plans {
		p := filepath.ToSlash(strings.TrimSpace(plans[i].Path))
		if p == "" {
			return fmt.Errorf("path is required")
		}
		plans[i].Path = p
		paths = append(paths, p)
	}

	oldMeta := map[string]fileMeta{}
	err := s.meta.View(func(tx *bbolt.Tx) error {
		fb := fileBucket(tx, workspaceID)
		if fb == nil {
			return nil
		}
		for _, path := range paths {
			raw := fb.Get([]byte(path))
			if raw == nil {
				continue
			}
			meta := fileMeta{}
			if err := decode(raw, &meta); err != nil {
				return err
			}
			oldMeta[path] = meta
		}
		return nil
	})
	if err != nil {
		return err
	}

	batch := s.idx.NewBatch()
	for _, plan := range plans {
		meta, ok := oldMeta[plan.Path]
		if ok {
			deleteDocs(batch, workspaceID, plan.Path, meta)
		}
		if plan.Delete {
			continue
		}
		indexDocs(batch, workspaceID, plan)
	}
	if err := s.idx.Batch(batch); err != nil {
		return err
	}

	return s.meta.Update(func(tx *bbolt.Tx) error {
		fb := mustFileBucket(tx, workspaceID)
		for _, plan := range plans {
			if plan.Delete {
				if err := fb.Delete([]byte(plan.Path)); err != nil {
					return err
				}
				continue
			}
			meta := fileMeta{
				Size:         plan.Size,
				MTime:        plan.MTime,
				Hash:         strings.TrimSpace(plan.Hash),
				ChunkCount:   len(plan.Chunks),
				SymbolCount:  len(plan.Syms),
				CommentCount: len(plan.Comms),
			}
			buf, err := encode(meta)
			if err != nil {
				return err
			}
			if err := fb.Put([]byte(plan.Path), buf); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) SearchChunks(workspaceID string, keyword string, limit int, caseInsensitive bool) (store.SearchResult, error) {
	if s == nil || s.idx == nil {
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

	baseQ := bleve.NewMatchQuery(keyword)
	baseQ.SetField("text")
	wsQ := bleve.NewTermQuery(workspaceID)
	wsQ.SetField("workspace_id")
	typeQ := bleve.NewTermQuery(docTypeChunk)
	typeQ.SetField("doc_type")
	q := bleve.NewConjunctionQuery(baseQ, wsQ, typeQ)

	req := bleve.NewSearchRequestOptions(q, limit, 0, false)
	req.Fields = []string{"path", "sl", "el", "kind", "title", "text"}
	req.Highlight = bleve.NewHighlightWithStyle("html")
	req.Highlight.Fields = []string{"text"}
	req.SortBy([]string{"path", "sl", "el"})

	res, err := s.idx.Search(req)
	if err != nil {
		return store.SearchResult{}, err
	}

	out := make([]store.Chunk, 0, len(res.Hits))
	for _, hit := range res.Hits {
		chunk := store.Chunk{
			WorkspaceID: workspaceID,
		}
		if v, ok := hit.Fields["path"].(string); ok {
			chunk.Path = v
		}
		if v, ok := toInt(hit.Fields["sl"]); ok {
			chunk.SL = v
		}
		if v, ok := toInt(hit.Fields["el"]); ok {
			chunk.EL = v
		}
		if v, ok := hit.Fields["kind"].(string); ok {
			chunk.Kind = v
		}
		if v, ok := hit.Fields["title"].(string); ok {
			chunk.Title = v
		}
		if v, ok := hit.Fields["text"].(string); ok {
			chunk.Text = v
		}
		if hit.Fragments != nil {
			if frags := hit.Fragments["text"]; len(frags) > 0 {
				chunk.Snippet = normalizeSnippet(frags[0])
			}
		}
		out = append(out, chunk)
	}
	return store.SearchResult{
		Chunks:               out,
		MatchCaseInsensitive: true,
		Backend:              "bleve",
	}, nil
}

func (s *Store) FindMinEnclosingSymbols(workspaceID string, path string, line int) ([]model.SymbolItem, error) {
	if s == nil || s.idx == nil {
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

	pathQ := bleve.NewTermQuery(path)
	pathQ.SetField("path")
	wsQ := bleve.NewTermQuery(workspaceID)
	wsQ.SetField("workspace_id")
	typeQ := bleve.NewTermQuery(docTypeSymbol)
	typeQ.SetField("doc_type")

	q := bleve.NewConjunctionQuery(pathQ, wsQ, typeQ)
	req := bleve.NewSearchRequestOptions(q, 2000, 0, false)
	req.Fields = []string{"kind", "name", "container", "lang", "signature", "path", "sl", "sc", "el", "ec"}

	res, err := s.idx.Search(req)
	if err != nil {
		return nil, err
	}

	items := make([]model.SymbolItem, 0, len(res.Hits))
	for _, hit := range res.Hits {
		var item model.SymbolItem
		if v, ok := hit.Fields["kind"].(string); ok {
			item.Kind = v
		}
		if v, ok := hit.Fields["name"].(string); ok {
			item.Name = v
		}
		if v, ok := hit.Fields["container"].(string); ok {
			item.Container = v
		}
		if v, ok := hit.Fields["lang"].(string); ok {
			item.Lang = v
		}
		if v, ok := hit.Fields["signature"].(string); ok {
			item.Signature = v
		}
		if v, ok := hit.Fields["path"].(string); ok {
			item.Path = v
		}
		if v, ok := toInt(hit.Fields["sl"]); ok {
			item.Range.SL = v
		}
		if v, ok := toInt(hit.Fields["sc"]); ok {
			item.Range.SC = v
		}
		if v, ok := toInt(hit.Fields["el"]); ok {
			item.Range.EL = v
		}
		if v, ok := toInt(hit.Fields["ec"]); ok {
			item.Range.EC = v
		}
		if item.Range.SL > 0 && item.Range.EL > 0 && line >= item.Range.SL && line <= item.Range.EL {
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		ri := items[i].Range
		rj := items[j].Range
		deltaI := ri.EL - ri.SL
		deltaJ := rj.EL - rj.SL
		if deltaI == deltaJ {
			if ri.SL == rj.SL {
				return ri.EL < rj.EL
			}
			return ri.SL < rj.SL
		}
		return deltaI < deltaJ
	})

	return items, nil
}

func (s *Store) CountChunks(workspaceID string) (int, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspaceID is required")
	}
	q := bleve.NewConjunctionQuery(
		termQuery("workspace_id", workspaceID),
		termQuery("doc_type", docTypeChunk),
	)
	req := bleve.NewSearchRequestOptions(q, 0, 0, false)
	req.SortBy(nil)
	res, err := s.idx.Search(req)
	if err != nil {
		return 0, err
	}
	return int(res.Total), nil
}

func (s *Store) CountFiles(workspaceID string) (int, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspaceID is required")
	}
	count, _, err := s.GetFilesStats(workspaceID)
	return count, err
}

func (s *Store) replaceOne(workspaceID string, path string, parts replaceParts) error {
	workspaceID = strings.TrimSpace(workspaceID)
	path = filepath.ToSlash(path)
	if workspaceID == "" {
		return fmt.Errorf("workspaceID is required")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}

	if err := s.EnsureWorkspace(workspaceID, ""); err != nil {
		return err
	}

	old := fileMeta{}
	err := s.meta.View(func(tx *bbolt.Tx) error {
		fb := fileBucket(tx, workspaceID)
		if fb == nil {
			return nil
		}
		raw := fb.Get([]byte(path))
		if raw == nil {
			return nil
		}
		return decode(raw, &old)
	})
	if err != nil {
		return err
	}

	batch := s.idx.NewBatch()

	meta := old
	if len(parts.chunks) > 0 || parts.chunksSet {
		deleteChunkDocs(batch, workspaceID, path, old.ChunkCount)
		meta.ChunkCount = len(parts.chunks)
		indexChunks(batch, workspaceID, path, parts.chunks)
	}
	if len(parts.syms) > 0 || parts.symsSet {
		deleteSymbolDocs(batch, workspaceID, path, old.SymbolCount)
		meta.SymbolCount = len(parts.syms)
		indexSymbols(batch, workspaceID, path, parts.syms)
	}
	if len(parts.comms) > 0 || parts.commsSet {
		deleteCommentDocs(batch, workspaceID, path, old.CommentCount)
		meta.CommentCount = len(parts.comms)
		indexComments(batch, workspaceID, path, parts.comms)
	}
	if err := s.idx.Batch(batch); err != nil {
		return err
	}

	return s.meta.Update(func(tx *bbolt.Tx) error {
		fb := mustFileBucket(tx, workspaceID)
		buf, err := encode(meta)
		if err != nil {
			return err
		}
		return fb.Put([]byte(path), buf)
	})
}

type replaceParts struct {
	chunks    []store.ChunkInput
	syms      []store.SymbolInput
	comms     []store.CommentInput
	chunksSet bool
	symsSet   bool
	commsSet  bool
}

func buildMapping() mapping.IndexMapping {
	idxMapping := bleve.NewIndexMapping()
	idxMapping.DefaultAnalyzer = "standard"

	doc := bleve.NewDocumentMapping()
	doc.Dynamic = false

	keyword := bleve.NewTextFieldMapping()
	keyword.Analyzer = "keyword"
	keyword.Store = true
	keyword.Index = true
	keyword.DocValues = true

	text := bleve.NewTextFieldMapping()
	text.Analyzer = "standard"
	text.Store = true
	text.Index = true

	num := bleve.NewNumericFieldMapping()
	num.Store = true
	num.Index = true
	num.DocValues = true

	doc.AddFieldMappingsAt("doc_type", keyword)
	doc.AddFieldMappingsAt("workspace_id", keyword)
	doc.AddFieldMappingsAt("path", keyword)
	doc.AddFieldMappingsAt("kind", keyword)
	doc.AddFieldMappingsAt("title", text)
	doc.AddFieldMappingsAt("text", text)
	doc.AddFieldMappingsAt("name", text)
	doc.AddFieldMappingsAt("container", text)
	doc.AddFieldMappingsAt("lang", keyword)
	doc.AddFieldMappingsAt("signature", text)
	doc.AddFieldMappingsAt("sl", num)
	doc.AddFieldMappingsAt("sc", num)
	doc.AddFieldMappingsAt("el", num)
	doc.AddFieldMappingsAt("ec", num)

	idxMapping.DefaultMapping = doc
	return idxMapping
}

func termQuery(field string, value string) bquery.Query {
	q := bleve.NewTermQuery(value)
	q.SetField(field)
	return q
}

func indexDocs(batch *bleve.Batch, workspaceID string, plan store.FilePlan) {
	indexChunks(batch, workspaceID, plan.Path, plan.Chunks)
	indexSymbols(batch, workspaceID, plan.Path, plan.Syms)
	indexComments(batch, workspaceID, plan.Path, plan.Comms)
}

func indexChunks(batch *bleve.Batch, workspaceID string, path string, chunks []store.ChunkInput) {
	for i, c := range chunks {
		doc := map[string]any{
			"doc_type":     docTypeChunk,
			"workspace_id": workspaceID,
			"path":         path,
			"sl":           c.SL,
			"el":           c.EL,
			"kind":         strings.TrimSpace(c.Kind),
			"title":        strings.TrimSpace(c.Title),
			"text":         c.Text,
		}
		batch.Index(chunkDocID(workspaceID, path, i), doc)
	}
}

func indexSymbols(batch *bleve.Batch, workspaceID string, path string, syms []store.SymbolInput) {
	for i, sym := range syms {
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
		doc := map[string]any{
			"doc_type":     docTypeSymbol,
			"workspace_id": workspaceID,
			"path":         path,
			"kind":         kind,
			"name":         sym.Name,
			"container":    sym.Container,
			"lang":         sym.Lang,
			"signature":    sym.Signature,
			"sl":           sym.SL,
			"sc":           sc,
			"el":           sym.EL,
			"ec":           ec,
		}
		batch.Index(symbolDocID(workspaceID, path, i), doc)
	}
}

func indexComments(batch *bleve.Batch, workspaceID string, path string, comms []store.CommentInput) {
	for i, comm := range comms {
		doc := map[string]any{
			"doc_type":     docTypeComment,
			"workspace_id": workspaceID,
			"path":         path,
			"kind":         strings.TrimSpace(comm.Kind),
			"text":         comm.Text,
			"lang":         comm.Lang,
			"sl":           comm.SL,
			"sc":           comm.SC,
			"el":           comm.EL,
			"ec":           comm.EC,
		}
		batch.Index(commentDocID(workspaceID, path, i), doc)
	}
}

func deleteDocs(batch *bleve.Batch, workspaceID string, path string, meta fileMeta) {
	deleteChunkDocs(batch, workspaceID, path, meta.ChunkCount)
	deleteSymbolDocs(batch, workspaceID, path, meta.SymbolCount)
	deleteCommentDocs(batch, workspaceID, path, meta.CommentCount)
}

func deleteChunkDocs(batch *bleve.Batch, workspaceID string, path string, count int) {
	for i := 0; i < count; i++ {
		batch.Delete(chunkDocID(workspaceID, path, i))
	}
}

func deleteSymbolDocs(batch *bleve.Batch, workspaceID string, path string, count int) {
	for i := 0; i < count; i++ {
		batch.Delete(symbolDocID(workspaceID, path, i))
	}
}

func deleteCommentDocs(batch *bleve.Batch, workspaceID string, path string, count int) {
	for i := 0; i < count; i++ {
		batch.Delete(commentDocID(workspaceID, path, i))
	}
}

func chunkDocID(workspaceID string, path string, idx int) string {
	return fmt.Sprintf("chunk|%s|%s|%d", workspaceID, escapePath(path), idx)
}

func symbolDocID(workspaceID string, path string, idx int) string {
	return fmt.Sprintf("symbol|%s|%s|%d", workspaceID, escapePath(path), idx)
}

func commentDocID(workspaceID string, path string, idx int) string {
	return fmt.Sprintf("comment|%s|%s|%d", workspaceID, escapePath(path), idx)
}

func escapePath(path string) string {
	return strings.ReplaceAll(path, "|", "%7C")
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case float32:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case int32:
		return int(t), true
	case uint64:
		return int(t), true
	case uint32:
		return int(t), true
	default:
		return 0, false
	}
}

func normalizeSnippet(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "<mark>", "<<")
	s = strings.ReplaceAll(s, "</mark>", ">>")
	return s
}

func (s *Store) ensureBuckets() error {
	return s.meta.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketWorkspaces)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketFiles)); err != nil {
			return err
		}
		return nil
	})
}

func mustBucket(tx *bbolt.Tx, name string) *bbolt.Bucket {
	b := tx.Bucket([]byte(name))
	if b == nil {
		b, _ = tx.CreateBucketIfNotExists([]byte(name))
	}
	return b
}

func fileBucket(tx *bbolt.Tx, workspaceID string) *bbolt.Bucket {
	fb := tx.Bucket([]byte(bucketFiles))
	if fb == nil {
		return nil
	}
	return fb.Bucket([]byte(workspaceID))
}

func mustFileBucket(tx *bbolt.Tx, workspaceID string) *bbolt.Bucket {
	fb := mustBucket(tx, bucketFiles)
	b, _ := fb.CreateBucketIfNotExists([]byte(workspaceID))
	return b
}

var errDecode = errors.New("decode failed")

func decode(data []byte, target any) error {
	if len(data) == 0 {
		return errDecode
	}
	return decodeJSON(data, target)
}

func encode(v any) ([]byte, error) {
	return encodeJSON(v)
}
