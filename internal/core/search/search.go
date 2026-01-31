package search

import (
	"strings"

	"otterindex/internal/model"
)

type Match = model.Match

func FindInText(text string, keyword string, caseInsensitive bool) []Match {
	if keyword == "" {
		return nil
	}

	needle := keyword
	if caseInsensitive {
		needle = strings.ToLower(needle)
	}

	var out []Match
	lines := strings.Split(text, "\n")
	for i, rawLine := range lines {
		line := rawLine
		hay := line
		if caseInsensitive {
			hay = strings.ToLower(hay)
		}

		idx := strings.Index(hay, needle)
		if idx < 0 {
			continue
		}

		out = append(out, Match{
			Line: i + 1,
			Col:  idx + 1,
			Text: line,
		})
	}
	return out
}

