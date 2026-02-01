//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"otterindex/internal/index/store"
)

func makeComment(n *tree_sitter.Node, src []byte, lang string) store.CommentInput {
	text := ""
	if n != nil {
		text = n.Utf8Text(src)
	}
	trimmed := strings.TrimSpace(text)
	kind := "comment"
	switch {
	case strings.HasPrefix(trimmed, "//"), strings.HasPrefix(trimmed, "#"):
		kind = "line"
	case strings.HasPrefix(trimmed, "/*"):
		kind = "block"
	}

	sl, sc, el, ec := nodeRange1Based(n)

	return store.CommentInput{
		Kind: kind,
		Text: strings.TrimRight(text, "\r\n"),
		SL:   sl,
		SC:   sc,
		EL:   el,
		EC:   ec,
		Lang: strings.TrimSpace(lang),
	}
}
