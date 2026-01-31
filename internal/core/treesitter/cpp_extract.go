//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"

	"otterindex/internal/index/sqlite"
)

var cppTypeKinds = map[string]struct{}{
	"class_specifier":  {},
	"struct_specifier": {},
}

func extractCPP(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_cpp.Language())
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
			comms = append(comms, makeComment(n, src, "cpp"))
		}

		switch k {
		case "namespace_definition":
			if sym, ok := makeCPPNamespace(n, src); ok {
				syms = append(syms, sym)
			}
		case "class_specifier":
			if sym, ok := makeCPPType(n, src, "class"); ok {
				syms = append(syms, sym)
			}
		case "struct_specifier":
			if sym, ok := makeCPPType(n, src, "struct"); ok {
				syms = append(syms, sym)
			}
		case "function_definition":
			if sym, ok := makeCPPFunction(n, src); ok {
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

func makeCPPType(n *tree_sitter.Node, src []byte, kind string) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		if id := firstDescendantKind(n, map[string]struct{}{"type_identifier": {}, "identifier": {}}); id != nil {
			name = strings.TrimSpace(id.Utf8Text(src))
		}
	}
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
		Lang:      "cpp",
		Signature: sig,
	}, true
}

func makeCPPNamespace(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		if id := firstDescendantKind(n, map[string]struct{}{"namespace_identifier": {}, "identifier": {}}); id != nil {
			name = strings.TrimSpace(id.Utf8Text(src))
		}
	}
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
		Lang:      "cpp",
		Signature: "namespace " + name,
	}, true
}

func makeCPPFunction(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	decl := n.ChildByFieldName("declarator")
	if decl == nil {
		return sqlite.SymbolInput{}, false
	}

	id := firstDescendantKind(decl, map[string]struct{}{
		"identifier":           {},
		"field_identifier":     {},
		"destructor_name":      {},
		"operator_name":        {},
		"qualified_identifier": {},
	})
	if id == nil {
		id = firstDescendantKind(decl, map[string]struct{}{"identifier": {}})
	}
	if id == nil {
		return sqlite.SymbolInput{}, false
	}
	name := strings.TrimSpace(id.Utf8Text(src))
	if name == "" {
		return sqlite.SymbolInput{}, false
	}

	container := enclosingTypeName(n, src, cppTypeKinds)
	sl, sc, el, ec := nodeRange1Based(n)

	sig := name
	if container != "" {
		sig = container + "::" + name
	}
	return sqlite.SymbolInput{
		Kind:      "function",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: container,
		Lang:      "cpp",
		Signature: sig,
	}, true
}
