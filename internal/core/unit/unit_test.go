package unit

import (
	"testing"

	"otterindex/internal/model"
)

func TestBlockUnit_Braces(t *testing.T) {
	text := "fn a() {\n  let x = 1;\n  // KEY\n  let y = 2;\n}\n"
	r := BlockRange(text, model.Match{Line: 3, Col: 3})
	if r.SL != 1 || r.EL != 5 {
		t.Fatalf("range=%+v", r)
	}
}

func TestBlockUnit_BracesPrefersInnermost(t *testing.T) {
	text := "fn a() {\n  if ok {\n    // KEY\n  }\n}\n"
	r := BlockRange(text, model.Match{Line: 3, Col: 5})
	if r.SL != 2 || r.EL != 4 {
		t.Fatalf("range=%+v", r)
	}
}

func TestBlockUnit_FallbackBlankLineBlock(t *testing.T) {
	text := "a\nb\n\nx\n// KEY\nz\n\np\n"
	r := BlockRange(text, model.Match{Line: 5, Col: 1})
	if r.SL != 4 || r.EL != 6 {
		t.Fatalf("range=%+v", r)
	}
}

func TestLineUnit_ContextClamps(t *testing.T) {
	text := "1\n2\n3\n4\n5\n"
	r := LineRange(text, model.Match{Line: 1, Col: 1}, 2)
	if r.SL != 1 || r.EL != 3 {
		t.Fatalf("range=%+v", r)
	}
	r = LineRange(text, model.Match{Line: 5, Col: 1}, 2)
	if r.SL != 3 || r.EL != 5 {
		t.Fatalf("range=%+v", r)
	}
}

func TestBlockUnit_UnmatchedBracesFallback(t *testing.T) {
	text := "fn a() {\n// KEY\n"
	r := BlockRange(text, model.Match{Line: 2, Col: 1})
	if r.SL != 1 || r.EL != 2 {
		t.Fatalf("range=%+v", r)
	}
}

func TestBlockUnit_MatchOnBlankLineFallsBackToLine(t *testing.T) {
	text := "a\n\nb\n"
	r := BlockRange(text, model.Match{Line: 2, Col: 1})
	if r.SL != 2 || r.EL != 2 {
		t.Fatalf("range=%+v", r)
	}
}
