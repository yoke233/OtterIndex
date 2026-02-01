//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_json "github.com/tree-sitter/tree-sitter-json/bindings/go"

	"otterindex/internal/index/store"
)

func extractJSON(path string, src []byte) ([]store.SymbolInput, []store.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_json.Language())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, nil, err
	}

	tree := parser.Parse(src, nil)
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil, nil, nil
	}

	var syms []store.SymbolInput
	var comms []store.CommentInput

	var walk func(n *tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}

		k := n.Kind()
		if isCommentKind(k) {
			comms = append(comms, makeComment(n, src, "json"))
		}

		switch k {
		case "pair":
			if sym, ok := makeJSONPair(n, src); ok {
				syms = append(syms, sym)
			}
		}

		for i := uint(0); i < n.NamedChildCount(); i++ {
			walk(n.NamedChild(i))
		}
	}

	walk(root)
	return syms, comms, nil
}

func makeJSONPair(n *tree_sitter.Node, src []byte) (store.SymbolInput, bool) {
	keyNode := n.ChildByFieldName("key")
	raw := trimNodeText(keyNode, src)
	name := jsonUnquoteKey(raw)
	if name == "" {
		return store.SymbolInput{}, false
	}

	sl, sc, el, ec := nodeRange1Based(n)
	return store.SymbolInput{
		Kind:      "key",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "json",
		Signature: name,
	}, true
}

func jsonUnquoteKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		raw = strings.TrimSuffix(strings.TrimPrefix(raw, "\""), "\"")
	}
	return strings.TrimSpace(raw)
}

