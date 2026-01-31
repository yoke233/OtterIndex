//go:build treesitter && cgo

package treesitter

import (
	"strings"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_ts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"otterindex/internal/index/sqlite"
)

var tsTypeKinds = map[string]struct{}{
	"class_declaration": {},
}

func extractTypeScript(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	return extractTypeScriptWithLang(path, src, tree_sitter_ts.LanguageTypescript(), "typescript")
}

func extractTSX(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	return extractTypeScriptWithLang(path, src, tree_sitter_ts.LanguageTSX(), "tsx")
}

func extractTypeScriptWithLang(path string, src []byte, langPtr unsafe.Pointer, langName string) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(langPtr)
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
			comms = append(comms, makeComment(n, src, langName))
		}

		switch k {
		case "class_declaration":
			if sym, ok := makeTSClass(n, src, langName); ok {
				syms = append(syms, sym)
			}
		case "interface_declaration":
			if sym, ok := makeTSNamedDecl(n, src, "interface", "interface", langName); ok {
				syms = append(syms, sym)
			}
		case "type_alias_declaration":
			if sym, ok := makeTSNamedDecl(n, src, "type", "type", langName); ok {
				syms = append(syms, sym)
			}
		case "enum_declaration":
			if sym, ok := makeTSNamedDecl(n, src, "enum", "enum", langName); ok {
				syms = append(syms, sym)
			}
		case "function_declaration", "generator_function_declaration":
			if sym, ok := makeTSFunction(n, src, langName); ok {
				syms = append(syms, sym)
			}
		case "method_definition":
			if sym, ok := makeTSMethod(n, src, langName); ok {
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

func makeTSClass(n *tree_sitter.Node, src []byte, lang string) (sqlite.SymbolInput, bool) {
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
		Lang:      lang,
		Signature: "class " + name,
	}, true
}

func makeTSNamedDecl(n *tree_sitter.Node, src []byte, kind string, sigPrefix string, lang string) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(n)
	return sqlite.SymbolInput{
		Kind:      kind,
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      lang,
		Signature: strings.TrimSpace(sigPrefix) + " " + name,
	}, true
}

func makeTSFunction(n *tree_sitter.Node, src []byte, lang string) (sqlite.SymbolInput, bool) {
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
		Lang:      lang,
		Signature: "function " + name,
	}, true
}

func makeTSMethod(n *tree_sitter.Node, src []byte, lang string) (sqlite.SymbolInput, bool) {
	name := trimNodeText(n.ChildByFieldName("name"), src)
	if name == "" {
		if id := firstDescendantKind(n, map[string]struct{}{"property_identifier": {}, "identifier": {}}); id != nil {
			name = strings.TrimSpace(id.Utf8Text(src))
		}
	}
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	container := enclosingTypeName(n, src, tsTypeKinds)
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
		Lang:      lang,
		Signature: sig,
	}, true
}
