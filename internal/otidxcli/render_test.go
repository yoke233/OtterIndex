package otidxcli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJSONL(t *testing.T) {
	lines := RenderJSONL([]ResultItem{
		{Path: "a.go", Range: Range{SL: 1, SC: 1, EL: 2, EC: 1}},
	})
	for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
		var v any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("invalid json: %v (%s)", err, line)
		}
	}
}

