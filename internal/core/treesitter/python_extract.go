//go:build treesitter && cgo

package treesitter

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"

	"otterindex/internal/index/sqlite"
)

var pythonTypeKinds = map[string]struct{}{
	"class_definition": {},
}

func extractPython(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_python.Language())
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
			comms = append(comms, makeComment(n, src, "python"))
		}

		switch k {
		case "class_definition":
			if sym, ok := makePythonClass(n, src); ok {
				syms = append(syms, sym)
			}
		case "function_definition":
			if sym, ok := makePythonFunction(n, src); ok {
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

func makePythonClass(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
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
		Lang:      "python",
		Signature: "class " + name,
	}, true
}

func makePythonFunction(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, pythonTypeKinds)
	sl, sc, el, ec := nodeRange1Based(n)

	sig := "def " + name
	if container != "" {
		sig = container + "." + name
	}
	return sqlite.SymbolInput{
		Kind:      "function",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: container,
		Lang:      "python",
		Signature: sig,
	}, true
}
