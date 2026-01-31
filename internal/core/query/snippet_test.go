package query

import (
	"strings"
	"testing"
)

func TestBuildSnippetFromMatchLine_HasMarkers(t *testing.T) {
	snip := buildSnippetFromMatchLine("a hello world", 3, "hello", false)
	if !strings.Contains(snip, "<<") || !strings.Contains(snip, ">>") {
		t.Fatalf("snippet=%q", snip)
	}
}

