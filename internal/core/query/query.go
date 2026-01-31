package query

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"otterindex/internal/core/search"
	"otterindex/internal/core/unit"
	"otterindex/internal/index/sqlite"
	"otterindex/internal/model"
)

func Query(dbPath string, workspaceID string, q string, unitMode string, contextLines int, caseInsensitive bool) ([]model.ResultItem, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	q = strings.TrimSpace(q)
	unitMode = strings.TrimSpace(unitMode)
	if unitMode == "" {
		unitMode = "block"
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

	chunks, err := s.SearchChunks(workspaceID, q, 50, caseInsensitive)
	if err != nil {
		return nil, err
	}

	fileLineCountCache := map[string]int{}

	var out []model.ResultItem
	for _, c := range chunks {
		item := model.ResultItem{
			Kind:  "unit",
			Path:  c.Path,
			Range: model.Range{SL: c.SL, SC: 1, EL: c.EL, EC: 1},
		}

		relMatches := search.FindInText(c.Text, q, caseInsensitive)
		if len(relMatches) > 0 {
			m0 := relMatches[0]
			m0.Line = c.SL + m0.Line - 1
			item.Matches = append(item.Matches, m0)
			item.Snippet = strings.TrimSpace(m0.Text)
		}

		switch unitMode {
		case "block":
			item.Range = model.Range{SL: c.SL, SC: 1, EL: c.EL, EC: 1}
		case "line":
			if len(item.Matches) == 0 {
				item.Range = unit.LineRange(c.Text, model.Match{Line: 1, Col: 1}, contextLines)
				item.Range.SL += c.SL - 1
				item.Range.EL += c.SL - 1
				break
			}

			m := item.Matches[0]
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
			return nil, fmt.Errorf("invalid unit %q", unitMode)
		}

		out = append(out, item)
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
