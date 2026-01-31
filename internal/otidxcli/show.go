package otidxcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func RenderShow(workspaceRoot string, items []ResultItem) string {
	base := strings.TrimSpace(workspaceRoot)
	if base == "" {
		base = "."
	}

	var b strings.Builder
	fileCache := map[string][]string{}
	seen := map[string]bool{}

	for _, item := range items {
		key := fmt.Sprintf("%s:%d:%d", item.Path, item.Range.SL, item.Range.EL)
		if seen[key] {
			continue
		}
		seen[key] = true

		lines := loadFileLines(base, item.Path, fileCache)
		if len(lines) == 0 {
			line, col, _ := bestVimLocationAndSnippet(item)
			_, _ = fmt.Fprintf(&b, "%s:%d:%d (%d-%d)\n\n", item.Path, line, col, item.Range.SL, item.Range.EL)
			continue
		}

		sl := clampInt(item.Range.SL, 1, len(lines))
		el := clampInt(item.Range.EL, sl, len(lines))

		line, col, _ := bestVimLocationAndSnippet(item)
		_, _ = fmt.Fprintf(&b, "%s:%d:%d (%d-%d)\n", item.Path, line, col, sl, el)

		width := len(strconv.Itoa(el))
		matchLines := map[int]bool{}
		for _, m := range item.Matches {
			matchLines[m.Line] = true
		}

		for i := sl; i <= el; i++ {
			prefix := " "
			if matchLines[i] {
				prefix = ">"
			}
			_, _ = fmt.Fprintf(&b, "%s %*d| %s\n", prefix, width, i, lines[i-1])
		}
		_, _ = fmt.Fprintln(&b)
	}

	return b.String()
}

func AttachText(workspaceRoot string, items []ResultItem) {
	base := strings.TrimSpace(workspaceRoot)
	if base == "" {
		base = "."
	}

	fileCache := map[string][]string{}
	for i := range items {
		if strings.TrimSpace(items[i].Text) != "" {
			continue
		}

		lines := loadFileLines(base, items[i].Path, fileCache)
		if len(lines) == 0 {
			continue
		}

		sl := clampInt(items[i].Range.SL, 1, len(lines))
		el := clampInt(items[i].Range.EL, sl, len(lines))
		items[i].Text = strings.Join(lines[sl-1:el], "\n")
	}
}

func loadFileLines(base string, rel string, cache map[string][]string) []string {
	if cache != nil {
		if v, ok := cache[rel]; ok {
			return v
		}
	}

	full := filepath.Join(base, filepath.FromSlash(rel))
	b, err := os.ReadFile(full)
	if err != nil {
		if cache != nil {
			cache[rel] = nil
		}
		return nil
	}

	lines := splitLines(string(b))
	if cache != nil {
		cache[rel] = lines
	}
	return lines
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

func clampInt(v int, min int, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
