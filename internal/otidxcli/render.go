package otidxcli

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RenderJSONL(items []ResultItem) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	for _, item := range items {
		_ = enc.Encode(item)
	}
	return b.String()
}

func RenderDefault(items []ResultItem) string {
	var b strings.Builder
	for _, item := range items {
		line, snippet := bestLocationAndSnippet(item)
		_, _ = fmt.Fprintf(&b, "%s:%d: %s\n", item.Path, line, snippet)
	}
	return b.String()
}

func RenderVim(items []ResultItem) string {
	var b strings.Builder
	for _, item := range items {
		line, col, snippet := bestVimLocationAndSnippet(item)
		_, _ = fmt.Fprintf(&b, "%s:%d:%d: %s\n", item.Path, line, col, snippet)
	}
	return b.String()
}

func bestLocationAndSnippet(item ResultItem) (line int, snippet string) {
	if len(item.Matches) > 0 {
		line = item.Matches[0].Line
		if item.Snippet != "" {
			return line, item.Snippet
		}
		if item.Matches[0].Text != "" {
			return line, strings.TrimSpace(item.Matches[0].Text)
		}
	}
	line = item.Range.SL
	if item.Snippet != "" {
		return line, item.Snippet
	}
	return line, strings.TrimSpace(item.Title)
}

func bestVimLocationAndSnippet(item ResultItem) (line int, col int, snippet string) {
	if len(item.Matches) > 0 {
		line = item.Matches[0].Line
		col = item.Matches[0].Col
		if item.Snippet != "" {
			return line, col, item.Snippet
		}
		if item.Matches[0].Text != "" {
			return line, col, strings.TrimSpace(item.Matches[0].Text)
		}
	}

	line = item.Range.SL
	col = item.Range.SC
	if item.Snippet != "" {
		return line, col, item.Snippet
	}
	return line, col, strings.TrimSpace(item.Title)
}

