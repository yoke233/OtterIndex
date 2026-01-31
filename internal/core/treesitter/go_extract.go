//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"otterindex/internal/index/sqlite"
)

func extractGo(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	_ = path

	parser := tree_sitter.NewParser()
	defer parser.Close()

	lang := tree_sitter.NewLanguage(tree_sitter_go.Language())
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

		switch n.Kind() {
		case "comment":
			comms = append(comms, makeGoComment(n, src))
		case "function_declaration":
			if sym, ok := makeGoFunction(n, src); ok {
				syms = append(syms, sym)
			}
		case "method_declaration":
			if sym, ok := makeGoMethod(n, src); ok {
				syms = append(syms, sym)
			}
		case "type_spec":
			if sym, ok := makeGoTypeSpec(n, src); ok {
				syms = append(syms, sym)
			}
		}

		for i := uint(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}

	walk(root)
	return syms, comms, nil
}

func makeGoFunction(fn *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	if fn == nil {
		return sqlite.SymbolInput{}, false
	}
	nameNode := fn.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = strings.TrimSpace(nameNode.Utf8Text(src))
	}
	if name == "" {
		return sqlite.SymbolInput{}, false
	}
	sl, sc, el, ec := nodeRange1Based(fn)

	return sqlite.SymbolInput{
		Kind:      "function",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "go",
		Signature: "func " + name,
	}, true
}

func makeGoMethod(m *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	if m == nil {
		return sqlite.SymbolInput{}, false
	}
	nameNode := m.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = strings.TrimSpace(nameNode.Utf8Text(src))
	}
	if name == "" {
		return sqlite.SymbolInput{}, false
	}

	container := strings.TrimSpace(goMethodReceiverType(m, src))

	sig := "func " + name
	if container != "" {
		sig = "func (" + container + ") " + name
	}

	sl, sc, el, ec := nodeRange1Based(m)

	return sqlite.SymbolInput{
		Kind:      "method",
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: container,
		Lang:      "go",
		Signature: sig,
	}, true
}

func makeGoTypeSpec(ts *tree_sitter.Node, src []byte) (sqlite.SymbolInput, bool) {
	if ts == nil {
		return sqlite.SymbolInput{}, false
	}

	nameNode := ts.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = strings.TrimSpace(nameNode.Utf8Text(src))
	}
	if name == "" {
		return sqlite.SymbolInput{}, false
	}

	kind := "type"
	typeNode := ts.ChildByFieldName("type")
	if typeNode != nil {
		inner := goUnwrapType(typeNode)
		switch inner.Kind() {
		case "struct_type":
			kind = "struct"
		case "interface_type":
			kind = "interface"
		}
	}

	sl, sc, el, ec := nodeRange1Based(ts)

	return sqlite.SymbolInput{
		Kind:      kind,
		Name:      name,
		SL:        sl,
		SC:        sc,
		EL:        el,
		EC:        ec,
		Container: "",
		Lang:      "go",
		Signature: kind + " " + name,
	}, true
}

func makeGoComment(c *tree_sitter.Node, src []byte) sqlite.CommentInput {
	text := ""
	if c != nil {
		text = c.Utf8Text(src)
	}
	trimmed := strings.TrimSpace(text)
	kind := "comment"
	if strings.HasPrefix(trimmed, "//") {
		kind = "line"
	} else if strings.HasPrefix(trimmed, "/*") {
		kind = "block"
	}

	sl, sc, el, ec := nodeRange1Based(c)

	return sqlite.CommentInput{
		Kind: kind,
		Text: strings.TrimRight(text, "\r\n"),
		SL:   sl,
		SC:   sc,
		EL:   el,
		EC:   ec,
		Lang: "go",
	}
}

func goMethodReceiverType(m *tree_sitter.Node, src []byte) string {
	if m == nil {
		return ""
	}
	recv := m.ChildByFieldName("receiver")
	if recv == nil {
		return ""
	}

	var decl *tree_sitter.Node
	for i := uint(0); i < recv.NamedChildCount(); i++ {
		ch := recv.NamedChild(i)
		if ch == nil {
			continue
		}
		switch ch.Kind() {
		case "parameter_declaration", "variadic_parameter_declaration":
			decl = ch
			break
		}
	}
	if decl == nil {
		return ""
	}
	typ := decl.ChildByFieldName("type")
	return goBaseTypeName(typ, src)
}

func goUnwrapType(typ *tree_sitter.Node) *tree_sitter.Node {
	for typ != nil {
		switch typ.Kind() {
		case "parenthesized_type", "pointer_type", "negated_type":
			if typ.NamedChildCount() == 0 {
				return typ
			}
			typ = typ.NamedChild(0)
			continue
		case "generic_type":
			if inner := typ.ChildByFieldName("type"); inner != nil {
				typ = inner
				continue
			}
			return typ
		default:
			return typ
		}
	}
	return nil
}

func goBaseTypeName(typ *tree_sitter.Node, src []byte) string {
	typ = goUnwrapType(typ)
	if typ == nil {
		return ""
	}

	switch typ.Kind() {
	case "qualified_type":
		if n := typ.ChildByFieldName("name"); n != nil {
			return strings.TrimSpace(n.Utf8Text(src))
		}
	case "type_identifier", "identifier":
		return strings.TrimSpace(typ.Utf8Text(src))
	}

	if n := goFirstDescendantKind(typ, map[string]struct{}{"type_identifier": {}, "identifier": {}}); n != nil {
		return strings.TrimSpace(n.Utf8Text(src))
	}
	return ""
}

func goFirstDescendantKind(n *tree_sitter.Node, want map[string]struct{}) *tree_sitter.Node {
	return firstDescendantKind(n, want)
}
