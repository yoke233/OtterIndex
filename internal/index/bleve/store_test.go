package bleve

import (
	"path/filepath"
	"testing"

	"otterindex/internal/index/store"
)

func TestBleveStore_ReplaceFilesBatch(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "index.bleve")

	st, err := Open(indexPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	workspaceID := root
	if err := st.EnsureWorkspace(workspaceID, root); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	plan := store.FilePlan{
		Path:  "a.go",
		Size:  10,
		MTime: 1,
		Hash:  "abc",
		Chunks: []store.ChunkInput{
			{SL: 1, EL: 1, Kind: "chunk", Title: "", Text: "hello world"},
		},
		Syms: []store.SymbolInput{
			{Kind: "function", Name: "Hello", SL: 1, SC: 1, EL: 1, EC: 5, Lang: "go", Signature: "func Hello"},
		},
		Comms: []store.CommentInput{
			{Kind: "line", Text: "// hi", SL: 1, SC: 1, EL: 1, EC: 5, Lang: "go"},
		},
	}

	if err := st.ReplaceFilesBatch(workspaceID, []store.FilePlan{plan}); err != nil {
		t.Fatalf("replace batch: %v", err)
	}

	cnt, err := st.CountChunks(workspaceID)
	if err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected chunks=1, got %d", cnt)
	}

	meta, ok, err := st.GetFileMeta(workspaceID, "a.go")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if !ok || meta.Hash != "abc" {
		t.Fatalf("unexpected meta: ok=%v meta=%+v", ok, meta)
	}

	res, err := st.SearchChunks(workspaceID, "hello", 10, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Chunks) == 0 {
		t.Fatalf("expected search results")
	}

	syms, err := st.FindMinEnclosingSymbols(workspaceID, "a.go", 1)
	if err != nil {
		t.Fatalf("symbols: %v", err)
	}
	if len(syms) == 0 {
		t.Fatalf("expected symbols")
	}

	if err := st.ReplaceFilesBatch(workspaceID, []store.FilePlan{{Path: "a.go", Delete: true}}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cnt, err = st.CountChunks(workspaceID)
	if err != nil {
		t.Fatalf("count chunks after delete: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("expected chunks=0, got %d", cnt)
	}
}
