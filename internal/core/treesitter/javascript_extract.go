//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_js "github.com/tree-sitter/tree-sitter-javascript/bindings/go"

	"otterindex/internal/index/sqlite"
)

var jsTypeKinds = map[string]struct{}{
	"class_declaration": {},
}

func extractJavaScript(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_js.Language())
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
			comms = append(comms, makeComment(n, src, "javascript"))
		}

		switch k {
		case "class_declaration":
			if sym, ok := makeJSClass(n, src); ok {
				syms = append(syms, sym)
			}
		case "function_declaration", "generator_function_declaration":
			if sym, ok := makeJSFunction(n, src); ok {
				syms = append(syms, sym)
			}
		case "method_definition":
			if sym, ok := makeJSMethod(n, src); ok {
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

func makeJSClass(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	return sqlite.SymbolInput{
		Kind:      "class",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "javascript",
		Signature: "class " + name,
	}, true
}

func makeJSFunction(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
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
		Lang:      "javascript",
		Signature: "function " + name,
	}, true
}

func makeJSMethod(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		if id := firstDescendantKind(n, map[string]struct{}{"property_identifier": {}, "identifier": {}}); id != nil {
			name = strings.TrimSpace(id.Utf8Text(src))
		}
	}
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, jsTypeKinds)
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
		Lang:      "javascript",
		Signature: sig,
	}, true
}
