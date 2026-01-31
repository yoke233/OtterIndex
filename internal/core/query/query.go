package query

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"otterindex/internal/core/explain"
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
	Offset          int
	Explain         explain.Explain
}

func Query(dbPath string, workspaceID string, q string, opts Options) ([]model.ResultItem, error) {
	items, _, err := queryWithInfo(dbPath, workspaceID, q, opts, 0)
	return items, err
}

type queryInfo struct {
	workspaceRoot string
	hasFTS        bool
	ftsReason     string
	candidates    []candidateRow
	fetchN        int
	exhausted     bool
}

func queryWithInfo(dbPath string, workspaceID string, q string, opts Options, prefetchMin int) ([]model.ResultItem, queryInfo, error) {
	ex := opts.Explain
	startTotal := time.Now()

	workspaceID = strings.TrimSpace(workspaceID)
	q = strings.TrimSpace(q)
	opts.Unit = strings.TrimSpace(opts.Unit)
	if opts.Unit == "" {
		opts.Unit = "block"
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		return nil, queryInfo{}, fmt.Errorf("offset must be >= 0")
	}

	if strings.TrimSpace(dbPath) == "" {
		return nil, queryInfo{}, fmt.Errorf("dbPath is required")
	}
	if workspaceID == "" {
		return nil, queryInfo{}, fmt.Errorf("workspaceID is required")
	}
	if q == "" {
		return nil, queryInfo{}, fmt.Errorf("query is required")
	}

	if ex != nil {
		ex.KV("phase", "query")
		ex.KV("db_path", dbPath)
		ex.KV("workspace_id", workspaceID)
		ex.KV("q", q)
		ex.KV("case_insensitive", opts.CaseInsensitive)
		ex.KV("limit", opts.Limit)
		ex.KV("offset", opts.Offset)
		ex.KV("include_globs", opts.IncludeGlobs)
		ex.KV("exclude_globs", opts.ExcludeGlobs)
		ex.KV("unit", opts.Unit)
		if opts.Unit == "line" {
			ex.KV("context_lines", opts.ContextLines)
		}
	}

	s, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, queryInfo{}, err
	}
	defer s.Close()

	ws, _ := s.GetWorkspace(workspaceID)
	if ex != nil {
		ex.KV("workspace_root", ws.Root)
		ex.KV("fts5", s.HasFTS())
		ex.KV("fts5_reason", s.FTSReason())
	}
	info := queryInfo{
		workspaceRoot: ws.Root,
		hasFTS:        s.HasFTS(),
		ftsReason:     s.FTSReason(),
	}

	wantN := opts.Offset + opts.Limit
	if wantN < 0 {
		return nil, queryInfo{}, fmt.Errorf("limit+offset overflow")
	}

	pathTopN := 3
	fetchN := wantN * 5
	if fetchN < 100 {
		fetchN = 100
	}
	if prefetchMin > 0 && fetchN < prefetchMin {
		fetchN = prefetchMin
	}
	if len(opts.IncludeGlobs) > 0 || len(opts.ExcludeGlobs) > 0 {
		// Over-fetch a bit to keep results useful when filters are strict.
		if fetchN < 500 {
			fetchN = 500
		}
	}
	if ex != nil {
		ex.KV("prefetch_n", fetchN)
		ex.KV("dedupe_topn", pathTopN)
	}
	info.fetchN = fetchN

	matchCaseInsensitive := opts.CaseInsensitive
	if s.HasFTS() {
		// FTS5 is case-insensitive by default; keep match extraction aligned.
		matchCaseInsensitive = true
	}
	if ex != nil {
		ex.KV("match_case_insensitive", matchCaseInsensitive)
	}

	sqlKeyword := q
	if s.HasFTS() {
		sqlKeyword = ftsPrefixQuery(q)
	}

	var items []model.ResultItem
	rowsReturned := 0
	var candidates []candidateRow
	attempts := 0
	for attempt := 0; attempt < 3; attempt++ {
		attempts++

		stopSQL := func() {}
		if ex != nil {
			stopSQL = ex.Timer("sql")
		}
		chunks, err := s.SearchChunks(workspaceID, sqlKeyword, fetchN, opts.CaseInsensitive)
		stopSQL()
		if err != nil {
			return nil, queryInfo{}, err
		}
		rowsReturned = len(chunks)
		candidates = candidatesFromChunks(chunks)

		stopMatch := func() {}
		if ex != nil {
			stopMatch = ex.Timer("match")
		}
		items, err = buildItemsFromCandidates(candidates, q, opts, matchCaseInsensitive, pathTopN, ex)
		stopMatch()
		if err != nil {
			return nil, queryInfo{}, err
		}

		// If we have enough (after dedupe/filters) or the DB returned fewer than requested, stop.
		if len(items) >= wantN || len(chunks) < fetchN {
			info.exhausted = len(chunks) < fetchN
			break
		}
		fetchN *= 2
		if fetchN < 0 {
			return nil, queryInfo{}, fmt.Errorf("fetch limit overflow")
		}
		if ex != nil {
			ex.KV("prefetch_n", fetchN)
		}
		info.fetchN = fetchN
	}
	if ex != nil {
		ex.KV("rows_returned", rowsReturned)
		ex.KV("prefetch_attempts", attempts)
		ex.KV("items_after_dedupe", len(items))
	}
	info.candidates = candidates

	items = sliceLimitOffset(items, opts.Offset, opts.Limit, ex)

	if opts.Unit == "symbol" {
		stopSymbol := func() {}
		if ex != nil {
			stopSymbol = ex.Timer("symbol")
		}
		fallback := refineSymbolRangesWithStore(s, workspaceID, items, ex)
		stopSymbol()
		if ex != nil && fallback > 0 {
			ex.KV("unit_fallback", "symbol->block")
		}
	}

	// Refine ranges using the real file when available.
	if ws.Root != "" {
		stopFile := func() {}
		if ex != nil {
			stopFile = ex.Timer("file_read")
		}
		refineRangesWithFiles(items, ws.Root, opts.Unit)
		stopFile()
	}

	if ex != nil {
		ex.KV("elapsed_ms_total", time.Since(startTotal).Milliseconds())
	}

	return items, info, nil
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

func findMatchesInChunk(text string, q string, caseInsensitive bool) []model.Match {
	matches := search.FindInText(text, q, caseInsensitive)
	if len(matches) > 0 {
		return matches
	}

	terms := extractQueryTerms(q)
	for _, term := range terms {
		matches = append(matches, search.FindInText(text, term, caseInsensitive)...)
	}
	if len(matches) <= 1 {
		return matches
	}

	seen := map[[2]int]bool{}
	dst := matches[:0]
	for _, m := range matches {
		k := [2]int{m.Line, m.Col}
		if seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, m)
	}
	matches = dst

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Line != matches[j].Line {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Col < matches[j].Col
	})
	return matches
}

func extractQueryTerms(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}

	seen := map[string]bool{}
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		term := b.String()
		b.Reset()
		if seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}

	for _, r := range q {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()

	return out
}

func ftsPrefixQuery(q string) string {
	terms := extractQueryTerms(q)
	if len(terms) == 0 {
		return q
	}
	for i := range terms {
		if len(terms[i]) >= 2 {
			terms[i] += "*"
		}
	}
	return strings.Join(terms, " ")
}

func candidatesFromChunks(chunks []sqlite.Chunk) []candidateRow {
	if len(chunks) == 0 {
		return nil
	}
	out := make([]candidateRow, len(chunks))
	for i := range chunks {
		out[i] = candidateRow{
			Path:    chunks[i].Path,
			SL:      chunks[i].SL,
			EL:      chunks[i].EL,
			Text:    chunks[i].Text,
			Snippet: chunks[i].Snippet,
		}
	}
	return out
}

func buildItemsFromCandidates(candidates []candidateRow, q string, opts Options, matchCaseInsensitive bool, pathTopN int, ex explain.Explain) ([]model.ResultItem, error) {
	items := make([]model.ResultItem, 0, len(candidates))
	for _, c := range candidates {
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

		relMatches := findMatchesInChunk(c.Text, q, matchCaseInsensitive)
		for i := range relMatches {
			relMatches[i].Line = c.SL + relMatches[i].Line - 1
		}
		item.Matches = relMatches
		if strings.TrimSpace(c.Snippet) != "" {
			item.Snippet = strings.TrimSpace(c.Snippet)
		} else if len(relMatches) > 0 {
			item.Snippet = buildSnippetFromMatchLine(relMatches[0].Text, relMatches[0].Col, q, matchCaseInsensitive)
		}

		stopUnitize := func() {}
		if ex != nil {
			stopUnitize = ex.Timer("unitize")
		}
		switch opts.Unit {
		case "symbol":
			fallthrough
		case "block":
			relLine := 1
			relCol := 1
			if len(relMatches) > 0 {
				relLine = relMatches[0].Line - c.SL + 1
				relCol = relMatches[0].Col
			}
			r := unit.BlockRange(c.Text, model.Match{Line: relLine, Col: relCol})
			r.SL += c.SL - 1
			r.EL += c.SL - 1
			item.Range = r
		case "line":
			if len(relMatches) == 0 {
				r := unit.LineRange(c.Text, model.Match{Line: 1, Col: 1}, opts.ContextLines)
				r.SL += c.SL - 1
				r.EL += c.SL - 1
				item.Range = r
				break
			}

			m := relMatches[0]
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
			item.Range = r
		case "file":
			// Filled after selection, if we have the workspace root.
		default:
			stopUnitize()
			return nil, fmt.Errorf("invalid unit %q", opts.Unit)
		}
		stopUnitize()

		items = append(items, item)
	}

	items = DedupeByPathTopN(items, pathTopN)
	return items, nil
}

func sliceLimitOffset(items []model.ResultItem, offset int, limit int, ex explain.Explain) []model.ResultItem {
	if offset >= len(items) {
		if ex != nil {
			ex.KV("items_returned", 0)
		}
		return nil
	}
	items = items[offset:]
	if len(items) > limit {
		items = items[:limit]
	}
	if ex != nil {
		ex.KV("items_returned", len(items))
	}
	return items
}

func refineRangesWithFiles(items []model.ResultItem, workspaceRoot string, unitKind string) {
	if strings.TrimSpace(workspaceRoot) == "" || len(items) == 0 {
		return
	}

	fileLineCountCache := map[string]int{}
	fileTextCache := map[string]string{}
	fileTextLoaded := map[string]bool{}

	for i := range items {
		switch unitKind {
		case "block":
			match := model.Match{Line: items[i].Range.SL, Col: 1}
			if len(items[i].Matches) > 0 {
				match = items[i].Matches[0]
			}

			fullText, ok := fileTextCache[items[i].Path]
			if !ok && !fileTextLoaded[items[i].Path] {
				fullText = readFileText(filepath.Join(workspaceRoot, filepath.FromSlash(items[i].Path)))
				fileTextCache[items[i].Path] = fullText
				fileTextLoaded[items[i].Path] = true
			}
			if strings.TrimSpace(fullText) != "" {
				items[i].Range = unit.BlockRange(fullText, match)
			}
		case "symbol":
			if items[i].Kind == "symbol" {
				total, ok := fileLineCountCache[items[i].Path]
				if !ok {
					total = countFileLines(filepath.Join(workspaceRoot, filepath.FromSlash(items[i].Path)))
					fileLineCountCache[items[i].Path] = total
				}
				if items[i].Range.SC <= 0 {
					items[i].Range.SC = 1
				}
				if items[i].Range.EC <= 0 {
					items[i].Range.EC = 1
				}
				if total > 0 && items[i].Range.EL > total {
					items[i].Range.EL = total
				}
				break
			}

			match := model.Match{Line: items[i].Range.SL, Col: 1}
			if len(items[i].Matches) > 0 {
				match = items[i].Matches[0]
			}

			fullText, ok := fileTextCache[items[i].Path]
			if !ok && !fileTextLoaded[items[i].Path] {
				fullText = readFileText(filepath.Join(workspaceRoot, filepath.FromSlash(items[i].Path)))
				fileTextCache[items[i].Path] = fullText
				fileTextLoaded[items[i].Path] = true
			}
			if strings.TrimSpace(fullText) != "" {
				items[i].Range = unit.BlockRange(fullText, match)
			}
		case "line":
			total, ok := fileLineCountCache[items[i].Path]
			if !ok {
				total = countFileLines(filepath.Join(workspaceRoot, filepath.FromSlash(items[i].Path)))
				fileLineCountCache[items[i].Path] = total
			}
			if total > 0 && items[i].Range.EL > total {
				items[i].Range.EL = total
			}
		case "file":
			total, ok := fileLineCountCache[items[i].Path]
			if !ok {
				total = countFileLines(filepath.Join(workspaceRoot, filepath.FromSlash(items[i].Path)))
				fileLineCountCache[items[i].Path] = total
			}
			if total > 0 {
				items[i].Range = model.Range{SL: 1, SC: 1, EL: total, EC: 1}
			}
		}
	}
}

func refineSymbolRangesWithStore(s *sqlite.Store, workspaceID string, items []model.ResultItem, ex explain.Explain) (fallback int) {
	if s == nil || len(items) == 0 {
		return 0
	}

	for i := range items {
		line := items[i].Range.SL
		if len(items[i].Matches) > 0 {
			line = items[i].Matches[0].Line
		}
		syms, err := s.FindMinEnclosingSymbols(workspaceID, items[i].Path, line)
		if err != nil || len(syms) == 0 {
			fallback++
			continue
		}

		// Prefer the smallest span; SQL already orders by (el-sl) but keep it deterministic here too.
		r, ok := unit.MinEnclosingSymbolRange(syms, line)
		chosen := syms[0]
		if ok {
			for _, sym := range syms {
				if sym.Range == r {
					chosen = sym
					break
				}
			}
			items[i].Range = r
		} else {
			items[i].Range = syms[0].Range
		}

		items[i].Kind = "symbol"
		if title := strings.TrimSpace(chosen.Signature); title != "" {
			items[i].Title = title
		} else if title := strings.TrimSpace(chosen.Name); title != "" {
			items[i].Title = title
		}
	}

	if ex != nil {
		ex.KV("symbol_fallback", fallback)
	}
	return fallback
}
