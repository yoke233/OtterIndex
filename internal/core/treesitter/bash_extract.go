//go:build treesitter && cgo

package treesitter

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"

	"otterindex/internal/index/store"
)

func extractBash(path string, src []byte) ([]store.SymbolInput, []store.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_bash.Language())
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
			comms = append(comms, makeComment(n, src, "bash"))
		}

		switch k {
		case "function_definition":
			if sym, ok := makeBashFunction(n, src); ok {
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

func makeBashFunction(n *tree_sitter.Node, src []byte) (store.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return store.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	return store.SymbolInput{
		Kind:      "function",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "bash",
		Signature: name,
	}, true
}



