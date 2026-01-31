//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"

	"otterindex/internal/index/sqlite"
)

var javaTypeKinds = map[string]struct{}{
	"class_declaration":     {},
	"interface_declaration": {},
	"enum_declaration":      {},
	"record_declaration":    {},
}

func extractJava(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_java.Language())
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
			comms = append(comms, makeComment(n, src, "java"))
		}

		switch k {
		case "class_declaration":
			if sym, ok := makeJavaType(n, src, "class"); ok {
				syms = append(syms, sym)
			}
		case "interface_declaration":
			if sym, ok := makeJavaType(n, src, "interface"); ok {
				syms = append(syms, sym)
			}
		case "enum_declaration":
			if sym, ok := makeJavaType(n, src, "enum"); ok {
				syms = append(syms, sym)
			}
		case "record_declaration":
			if sym, ok := makeJavaType(n, src, "record"); ok {
				syms = append(syms, sym)
			}
		case "method_declaration":
			if sym, ok := makeJavaMethod(n, src); ok {
				syms = append(syms, sym)
			}
		case "constructor_declaration":
			if sym, ok := makeJavaConstructor(n, src); ok {
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

func makeJavaType(n *tree_sitter.Node, src []byte, kind string) (sqlite.SymbolInput, bool) {
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
		Lang:      "java",
		Signature: sig,
	}, true
}

func makeJavaMethod(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, javaTypeKinds)
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
		Lang:      "java",
		Signature: sig,
	}, true
}

func makeJavaConstructor(n *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, javaTypeKinds)
	sl, sc, el, ec := nodeRange1Based(n)

	sig := name
	if container != "" {
		sig = container + "." + name
	}
	return sqlite.SymbolInput{
		Kind:      "constructor",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: container,
		Lang:      "java",
		Signature: sig,
	}, true
}
