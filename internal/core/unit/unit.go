package unit

import (
	"strings"

	"otterindex/internal/model"
)

type Match = model.Match
type Range = model.Range

func LineRange(text string, m Match, contextLines int) Range {
	total := lineCount(text)
	if total <= 0 {
		return Range{SL: 1, SC: 1, EL: 1, EC: 1}
	}

	line := clamp(m.Line, 1, total)
	if contextLines < 0 {
		contextLines = 0
	}

	sl := line - contextLines
	el := line + contextLines
	if sl < 1 {
		sl = 1
	}
	if el > total {
		el = total
	}

	return Range{SL: sl, SC: 1, EL: el, EC: 1}
}

func FileRange(text string) Range {
	total := lineCount(text)
	if total <= 0 {
		return Range{SL: 1, SC: 1, EL: 1, EC: 1}
	}
	return Range{SL: 1, SC: 1, EL: total, EC: 1}
}

func BlockRange(text string, m Match) Range {
	total := lineCount(text)
	if total <= 0 {
		return Range{SL: 1, SC: 1, EL: 1, EC: 1}
	}

	line := clamp(m.Line, 1, total)
	if r, ok := blockRangeByBraces(text, line); ok {
		return r
	}
	if r, ok := blockRangeByBlankLines(text, line); ok {
		return r
	}
	return LineRange(text, Match{Line: line, Col: 1}, 0)
}

func blockRangeByBraces(text string, matchLine int) (Range, bool) {
	type brace struct {
		line int
		col  int
	}
	type pair struct {
		open  brace
		close brace
	}

	var stack []brace
	var pairs []pair

	line := 1
	col := 1
	for _, r := range text {
		switch r {
		case '\n':
			line++
			col = 1
			continue
		case '{':
			stack = append(stack, brace{line: line, col: col})
		case '}':
			if len(stack) > 0 {
				open := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				pairs = append(pairs, pair{open: open, close: brace{line: line, col: col}})
			}
		}
		col++
	}

	bestIdx := -1
	bestLen := 0
	for i, p := range pairs {
		if p.open.line <= matchLine && matchLine <= p.close.line {
			l := p.close.line - p.open.line
			if bestIdx == -1 || l < bestLen {
				bestIdx = i
				bestLen = l
			}
		}
	}
	if bestIdx == -1 {
		return Range{}, false
	}

	p := pairs[bestIdx]
	return Range{SL: p.open.line, SC: 1, EL: p.close.line, EC: 1}, true
}

func blockRangeByBlankLines(text string, matchLine int) (Range, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return Range{}, false
	}

	idx := matchLine - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(lines) {
		idx = len(lines) - 1
	}

	isEmpty := func(s string) bool { return strings.TrimSpace(s) == "" }

	if isEmpty(lines[idx]) {
		return Range{}, false
	}

	start := idx
	for start > 0 && !isEmpty(lines[start-1]) {
		start--
	}
	end := idx
	for end+1 < len(lines) && !isEmpty(lines[end+1]) {
		end++
	}

	return Range{SL: start + 1, SC: 1, EL: end + 1, EC: 1}, true
}

func lineCount(text string) int {
	if text == "" {
		return 0
	}
	// If file ends with newline, Split gives last empty line; ignore it.
	parts := strings.Split(text, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		return len(parts) - 1
	}
	return len(parts)
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func MinEnclosingSymbolRange(symbols []model.SymbolItem, line int) (model.Range, bool) {
	if line <= 0 || len(symbols) == 0 {
		return model.Range{}, false
	}

	bestIdx := -1
	bestSpan := 0
	for i := range symbols {
		r := symbols[i].Range
		if r.SL <= 0 || r.EL <= 0 {
			continue
		}
		if r.SL > line || r.EL < line {
			continue
		}
		span := r.EL - r.SL
		if bestIdx == -1 || span < bestSpan {
			bestIdx = i
			bestSpan = span
		}
	}
	if bestIdx == -1 {
		return model.Range{}, false
	}
	return symbols[bestIdx].Range, true
}
