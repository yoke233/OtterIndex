package query

import "otterindex/internal/model"

func DedupeByPathTopN(items []model.ResultItem, n int) []model.ResultItem {
	if n <= 0 || len(items) == 0 {
		return items
	}

	seen := map[string]int{}
	out := make([]model.ResultItem, 0, len(items))
	for _, item := range items {
		if seen[item.Path] >= n {
			continue
		}
		seen[item.Path]++
		out = append(out, item)
	}
	return out
}

