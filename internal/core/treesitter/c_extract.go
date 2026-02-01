//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"

	"otterindex/internal/index/store"
)

func extractC(path string, src []byte) ([]store.SymbolInput, []store.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_c.Language())
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
			comms = append(comms, makeComment(n, src, "c"))
		}

		switch k {
		case "function_definition":
			if sym, ok := makeCFunction(n, src); ok {
				syms = append(syms, sym)
			}
		case "struct_specifier":
			if sym, ok := makeCType(n, src, "struct"); ok {
				syms = append(syms, sym)
			}
		case "enum_specifier":
			if sym, ok := makeCType(n, src, "enum"); ok {
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

func makeCFunction(n *tree_sitter.Node, src []byte) (store.SymbolInput, bool) {
	if n == nil {
		return store.SymbolInput{}, false
	}
	decl := n.ChildByFieldName("declarator")
	if decl == nil {
		return store.SymbolInput{}, false
	}
	id := firstDescendantKind(decl, map[string]struct{}{"identifier": {}})
	if id == nil {
		return store.SymbolInput{}, false
	}
	name := strings.TrimSpace(id.Utf8Text(src))
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
		Lang:      "c",
		Signature: name,
	}, true
}

func makeCType(n *tree_sitter.Node, src []byte, kind string) (store.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		if id := firstDescendantKind(n, map[string]struct{}{"type_identifier": {}, "identifier": {}}); id != nil {
			name = strings.TrimSpace(id.Utf8Text(src))
		}
	}
	if name == "" {
		return store.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	sig := strings.TrimSpace(kind) + " " + name
	return store.SymbolInput{
		Kind:      strings.TrimSpace(kind),
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "c",
		Signature: sig,
	}, true
}
