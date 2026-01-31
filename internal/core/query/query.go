package query

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"otterindex/internal/core/search"
	"otterindex/internal/core/unit"
	"otterindex/internal/index/sqlite"
	"otterindex/internal/model"
)

type Options struct {
	Unit            string
	ContextLines    int
	CaseInsensitive bool
	IncludeGlobs    []string
	ExcludeGlobs    []string
	Limit           int
}

func Query(dbPath string, workspaceID string, q string, opts Options) ([]model.ResultItem, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	q = strings.TrimSpace(q)
	opts.Unit = strings.TrimSpace(opts.Unit)
	if opts.Unit == "" {
		opts.Unit = "block"
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("dbPath is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceID is required")
	}
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}

	s, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	ws, _ := s.GetWorkspace(workspaceID)

	searchLimit := opts.Limit
	if len(opts.IncludeGlobs) > 0 || len(opts.ExcludeGlobs) > 0 {
		// Over-fetch a bit to keep results useful when filters are strict.
		if searchLimit < 500 {
			searchLimit = 500
		}
	}
	chunks, err := s.SearchChunks(workspaceID, q, searchLimit, opts.CaseInsensitive)
	if err != nil {
		return nil, err
	}

	fileLineCountCache := map[string]int{}
	fileTextCache := map[string]string{}
	fileTextLoaded := map[string]bool{}

	var out []model.ResultItem
	for _, c := range chunks {
		if len(opts.IncludeGlobs) > 0 && !anyGlobMatch(opts.IncludeGlobs, c.Path) {
			continue
		}
		if anyGlobMatch(opts.ExcludeGlobs, c.Path) {
			continue
		}

		item := model.ResultItem{
			Kind:  "unit",
			Path:  c.Path,
			Range: model.Range{SL: c.SL, SC: 1, EL: c.EL, EC: 1},
		}

		relMatches := search.FindInText(c.Text, q, opts.CaseInsensitive)
		if len(relMatches) > 0 {
			m0 := relMatches[0]
			m0.Line = c.SL + m0.Line - 1
			item.Matches = append(item.Matches, m0)
			item.Snippet = strings.TrimSpace(m0.Text)
		}

		switch opts.Unit {
		case "block":
			match := model.Match{Line: c.SL, Col: 1}
			if len(item.Matches) > 0 {
				match = item.Matches[0]
			}

			if ws.Root != "" {
				fullText, ok := fileTextCache[c.Path]
				if !ok && !fileTextLoaded[c.Path] {
					fullText = readFileText(filepath.Join(ws.Root, filepath.FromSlash(c.Path)))
					fileTextCache[c.Path] = fullText
					fileTextLoaded[c.Path] = true
				}
				if strings.TrimSpace(fullText) != "" {
					item.Range = unit.BlockRange(fullText, match)
					break
				}
			}

			relLine := 1
			relCol := 1
			if match.Line >= c.SL {
				relLine = match.Line - c.SL + 1
				relCol = match.Col
			}
			r := unit.BlockRange(c.Text, model.Match{Line: relLine, Col: relCol})
			r.SL += c.SL - 1
			r.EL += c.SL - 1
			item.Range = r
		case "line":
			if len(item.Matches) == 0 {
				item.Range = unit.LineRange(c.Text, model.Match{Line: 1, Col: 1}, opts.ContextLines)
				item.Range.SL += c.SL - 1
				item.Range.EL += c.SL - 1
				break
			}

			m := item.Matches[0]
			contextLines := opts.ContextLines
			if contextLines < 0 {
				contextLines = 0
			}
			r := model.Range{
				SL: m.Line - contextLines,
				SC: 1,
				EL: m.Line + contextLines,
				EC: 1,
			}
			if r.SL < 1 {
				r.SL = 1
			}
			if ws.Root != "" {
				total, ok := fileLineCountCache[c.Path]
				if !ok {
					total = countFileLines(filepath.Join(ws.Root, filepath.FromSlash(c.Path)))
					fileLineCountCache[c.Path] = total
				}
				if total > 0 && r.EL > total {
					r.EL = total
				}
			}
			item.Range = r
		case "file":
			if ws.Root != "" {
				total, ok := fileLineCountCache[c.Path]
				if !ok {
					total = countFileLines(filepath.Join(ws.Root, filepath.FromSlash(c.Path)))
					fileLineCountCache[c.Path] = total
				}
				if total > 0 {
					item.Range = model.Range{SL: 1, SC: 1, EL: total, EC: 1}
				}
			}
		default:
			return nil, fmt.Errorf("invalid unit %q", opts.Unit)
		}

		out = append(out, item)
		if len(out) >= opts.Limit {
			break
		}
	}

	return out, nil
}

func countFileLines(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	parts := strings.Split(string(b), "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return len(parts)
}

func readFileText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func anyGlobMatch(patterns []string, rel string) bool {
	for _, pat := range patterns {
		if matchesGlob(pat, rel) {
			return true
		}
	}
	return false
}

func matchesGlob(pattern string, rel string) bool {
	pat := strings.TrimSpace(pattern)
	if pat == "" {
		return false
	}
	pat = strings.ReplaceAll(pat, "\\", "/")
	rel = filepath.ToSlash(rel)

	// Support csv passed via -x "*.js,*.sql" when not using StringSliceVar.
	if strings.Contains(pat, ",") {
		for _, piece := range strings.Split(pat, ",") {
			if matchesGlob(strings.TrimSpace(piece), rel) {
				return true
			}
		}
		return false
	}

	// Treat patterns without path separators as basename patterns.
	if !strings.Contains(pat, "/") {
		ok, _ := path.Match(pat, path.Base(rel))
		return ok
	}

	ok, _ := path.Match(pat, rel)
	return ok
}
