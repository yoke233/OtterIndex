package query

import (
	"sort"
	"strings"
	"unicode"
)

func buildSnippetFromMatchLine(line string, col int, q string, caseInsensitive bool) string {
	line = strings.TrimRight(line, " \t\r")
	if strings.TrimSpace(line) == "" {
		return ""
	}

	idx := col - 1
	if idx < 0 || idx >= len(line) {
		idx = -1
	}

	candidates := snippetCandidates(q)
	if idx >= 0 {
		for _, term := range candidates {
			if term == "" {
				continue
			}
			if hasTermAt(line, idx, term, caseInsensitive) {
				return windowedHighlight(line, idx, idx+len(term))
			}
		}
	}

	if idx < 0 {
		for _, term := range candidates {
			if term == "" {
				continue
			}
			pos := indexOfTerm(line, term, caseInsensitive)
			if pos < 0 {
				continue
			}
			return windowedHighlight(line, pos, pos+len(term))
		}
		return strings.TrimSpace(line)
	}

	start := idx
	end := idx
	for end < len(line) {
		r := rune(line[end])
		if unicode.IsSpace(r) {
			break
		}
		end++
	}
	if end == start {
		end = minInt(len(line), start+1)
	}
	return windowedHighlight(line, start, end)
}

func snippetCandidates(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}

	terms := extractQueryTerms(q)
	if len(terms) == 0 {
		terms = []string{q}
	}

	sort.SliceStable(terms, func(i, j int) bool {
		if len(terms[i]) != len(terms[j]) {
			return len(terms[i]) > len(terms[j])
		}
		return terms[i] < terms[j]
	})
	return terms
}

func hasTermAt(line string, idx int, term string, caseInsensitive bool) bool {
	if idx < 0 || idx+len(term) > len(line) {
		return false
	}
	if !caseInsensitive {
		return line[idx:idx+len(term)] == term
	}
	return strings.EqualFold(line[idx:idx+len(term)], term)
}

func indexOfTerm(line string, term string, caseInsensitive bool) int {
	if !caseInsensitive {
		return strings.Index(line, term)
	}
	return strings.Index(strings.ToLower(line), strings.ToLower(term))
}

func windowedHighlight(line string, start int, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(line) {
		end = len(line)
	}
	if start >= end {
		return strings.TrimSpace(line)
	}

	const context = 80
	winStart := start - context
	if winStart < 0 {
		winStart = 0
	}
	winEnd := end + context
	if winEnd > len(line) {
		winEnd = len(line)
	}

	prefix := ""
	suffix := ""
	if winStart > 0 {
		prefix = "…"
	}
	if winEnd < len(line) {
		suffix = "…"
	}

	window := line[winStart:winEnd]
	localStart := start - winStart
	localEnd := end - winStart

	var b strings.Builder
	b.Grow(len(prefix) + len(window) + len(suffix) + 4)
	b.WriteString(prefix)
	b.WriteString(window[:localStart])
	b.WriteString("<<")
	b.WriteString(window[localStart:localEnd])
	b.WriteString(">>")
	b.WriteString(window[localEnd:])
	b.WriteString(suffix)
	return strings.TrimSpace(b.String())
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

