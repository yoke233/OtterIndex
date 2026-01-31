package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"otterindex/internal/core/walk"
	"otterindex/internal/index/sqlite"
)

type Options struct {
	WorkspaceID string
	ScanAll     bool
	IncludeGlobs []string
	ExcludeGlobs []string

	ChunkLines   int
	ChunkOverlap int
}

func Build(root string, dbPath string, opts Options) error {
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

	s, err := sqlite.Open(dbPath)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.EnsureWorkspace(workspaceID, root); err != nil {
		return err
	}

	files, err := walk.ListFiles(root, walk.Options{
		IncludeGlobs: opts.IncludeGlobs,
		ExcludeGlobs: opts.ExcludeGlobs,
		ScanAll:      opts.ScanAll,
	})
	if err != nil {
		return err
	}

	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		st, err := os.Stat(full)
		if err != nil {
			return err
		}

		b, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if isBinary(b) {
			continue
		}

		if err := s.UpsertFile(workspaceID, rel, st.Size(), st.ModTime().Unix()); err != nil {
			return err
		}

		text := string(b)
		chunks := chunkByLines(rel, text, chunkLines, step)
		if err := s.ReplaceChunks(workspaceID, rel, chunks); err != nil {
			return err
		}
	}

	return nil
}

func chunkByLines(relPath string, text string, chunkLines int, step int) []sqlite.Chunk {
	lines := splitLines(text)
	if len(lines) == 0 {
		return nil
	}

	var out []sqlite.Chunk
	for start := 0; start < len(lines); start += step {
		end := start + chunkLines
		if end > len(lines) {
			end = len(lines)
		}
		chunkText := strings.Join(lines[start:end], "\n")
		out = append(out, sqlite.Chunk{
			Path:  relPath,
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

