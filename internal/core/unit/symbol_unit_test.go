package unit

import (
	"testing"

	"otterindex/internal/model"
)

func TestMinEnclosingSymbolRange(t *testing.T) {
	syms := []model.SymbolItem{
		{Kind: "function", Name: "big", Range: model.Range{SL: 1, EL: 100}},
		{Kind: "function", Name: "small", Range: model.Range{SL: 10, EL: 20}},
		{Kind: "block", Name: "tiny", Range: model.Range{SL: 12, EL: 12}},
	}
	r, ok := MinEnclosingSymbolRange(syms, 12)
	if !ok || r.SL != 12 || r.EL != 12 {
		t.Fatalf("range=%+v ok=%v", r, ok)
	}
}
