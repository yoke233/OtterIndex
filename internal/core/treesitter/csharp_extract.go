//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"

	"otterindex/internal/index/store"
)

var csharpTypeKinds = map[string]struct{}{
	"class_declaration":     {},
	"interface_declaration": {},
	"struct_declaration":    {},
	"record_declaration":    {},
	"enum_declaration":      {},
}

func extractCSharp(path string, src []byte) ([]store.SymbolInput, []store.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_c_sharp.Language())
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
			comms = append(comms, makeComment(n, src, "csharp"))
		}

		switch k {
		case "file_scoped_namespace_declaration", "namespace_declaration":
			if sym, ok := makeCSharpNamespace(n, src); ok {
				syms = append(syms, sym)
			}
		case "class_declaration":
			if sym, ok := makeCSharpType(n, src, "class"); ok {
				syms = append(syms, sym)
			}
		case "interface_declaration":
			if sym, ok := makeCSharpType(n, src, "interface"); ok {
				syms = append(syms, sym)
			}
		case "struct_declaration":
			if sym, ok := makeCSharpType(n, src, "struct"); ok {
				syms = append(syms, sym)
			}
		case "record_declaration":
			if sym, ok := makeCSharpType(n, src, "record"); ok {
				syms = append(syms, sym)
			}
		case "enum_declaration":
			if sym, ok := makeCSharpType(n, src, "enum"); ok {
				syms = append(syms, sym)
			}
		case "method_declaration":
			if sym, ok := makeCSharpMethod(n, src); ok {
				syms = append(syms, sym)
			}
		case "constructor_declaration":
			if sym, ok := makeCSharpConstructor(n, src); ok {
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

func makeCSharpNamespace(n *tree_sitter.Node, src []byte) (store.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return store.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	return store.SymbolInput{
		Kind:      "namespace",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "csharp",
		Signature: "namespace " + name,
	}, true
}

func makeCSharpType(n *tree_sitter.Node, src []byte, kind string) (store.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
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
		Lang:      "csharp",
		Signature: sig,
	}, true
}

func makeCSharpMethod(n *tree_sitter.Node, src []byte) (store.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return store.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, csharpTypeKinds)
	sl, sc, el, ec := nodeRange1Based(n)

	sig := name
	if container != "" {
		sig = container + "." + name
	}
	return store.SymbolInput{
		Kind:      "method",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: container,
		Lang:      "csharp",
		Signature: sig,
	}, true
}

func makeCSharpConstructor(n *tree_sitter.Node, src []byte) (store.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return store.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, csharpTypeKinds)
	sl, sc, el, ec := nodeRange1Based(n)

	sig := name
	if container != "" {
		sig = container + "." + name
	}
	return store.SymbolInput{
		Kind:      "constructor",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: container,
		Lang:      "csharp",
		Signature: sig,
	}, true
}

