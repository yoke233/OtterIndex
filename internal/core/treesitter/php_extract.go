//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"

	"otterindex/internal/index/sqlite"
)

var phpTypeKinds = map[string]struct{}{
	"class_declaration":     {},
	"interface_declaration": {},
	"trait_declaration":     {},
	"enum_declaration":      {},
}

func extractPHP(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, nil, err
	}

	tree := parser.Parse(src, nil)
	defer tree.Close()

	root := tree.RootNode()
	if root == nil {
		return nil, nil, nil
	}

	var syms []sqlite.SymbolInput
	var comms []sqlite.CommentInput

	var walk func(n *tree_sitter.Node)
	walk = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}

		k := n.Kind()
		if isCommentKind(k) {
			comms = append(comms, makeComment(n, src, "php"))
		}

		switch k {
		case "namespace_definition":
			if sym, ok := makePHPNamespace(n, src); ok {
				syms = append(syms, sym)
			}
		case "class_declaration":
			if sym, ok := makePHPType(n, src, "class"); ok {
				syms = append(syms, sym)
			}
		case "interface_declaration":
			if sym, ok := makePHPType(n, src, "interface"); ok {
				syms = append(syms, sym)
			}
		case "trait_declaration":
			if sym, ok := makePHPType(n, src, "trait"); ok {
				syms = append(syms, sym)
			}
		case "enum_declaration":
			if sym, ok := makePHPType(n, src, "enum"); ok {
				syms = append(syms, sym)
			}
		case "function_definition":
			if sym, ok := makePHPFunction(n, src); ok {
				syms = append(syms, sym)
			}
		case "method_declaration":
			if sym, ok := makePHPMethod(n, src); ok {
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

func makePHPNamespace(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	return sqlite.SymbolInput{
		Kind:      "namespace",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "php",
		Signature: "namespace " + name,
	}, true
}

func makePHPType(n *tree_sitter.Node, src []byte, kind string) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	sig := strings.TrimSpace(kind) + " " + name
	return sqlite.SymbolInput{
		Kind:      strings.TrimSpace(kind),
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "php",
		Signature: sig,
	}, true
}

func makePHPFunction(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	return sqlite.SymbolInput{
		Kind:      "function",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "php",
		Signature: "function " + name,
	}, true
}

func makePHPMethod(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, phpTypeKinds)
	sl, sc, el, ec := nodeRange1Based(n)

	sig := name
	if container != "" {
		sig = container + "." + name
	}
	return sqlite.SymbolInput{
		Kind:      "method",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: container,
		Lang:      "php",
		Signature: sig,
	}, true
}

