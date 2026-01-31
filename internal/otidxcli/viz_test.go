package otidxcli

import (
	"strings"
	"testing"
)

func TestVizASCII(t *testing.T) {
	s := VizASCII()
	for _, want := range []string{"walk", "index", "query", "unitize", "render"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %s", want, s)
		}
	}
}

