package otidxcli

import (
	"encoding/json"
	"testing"
)

func TestExplainJSON_Parseable(t *testing.T) {
	ex := NewExplainCollector(ExplainOptions{Format: "json"})
	ex.KV("phase", "query")
	ex.KV("fts5", true)
	line := ex.EmitToStringForTest()
	var v map[string]any
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		t.Fatalf("bad json: %v", err)
	}
}

