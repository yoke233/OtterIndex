//go:build treesitter && cgo

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func nodeRange1Based(n *tree_sitter.Node) (sl, sc, el, ec int) {
	if n == nil {
		return 0, 0, 0, 0
	}
	sp := n.StartPosition()
	ep := n.EndPosition()

	sl = int(sp.Row) + 1
	sc = int(sp.Column) + 1
	el = int(ep.Row) + 1
	ec = int(ep.Column) + 1

	if sc <= 0 {
		sc = 1
	}
	if ec <= 0 {
		ec = 1
	}

	if ep.Column == 0 && el > sl {
		el--
	}
	if el < sl {
		el = sl
	}

	return sl, sc, el, ec
}

func isCommentKind(kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	return strings.Contains(kind, "comment")
}

func trimNodeText(n *tree_sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return strings.TrimSpace(n.Utf8Text(src))
}

func firstDescendantKind(n *tree_sitter.Node, want map[string]struct{}) *tree_sitter.Node {
	if n == nil {
		return nil
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		ch := n.NamedChild(i)
		if ch == nil {
			continue
		}
		if _, ok := want[ch.Kind()]; ok {
			return ch
		}
		if found := firstDescendantKind(ch, want); found != nil {
			return found
		}
	}
	return nil
}

func enclosingTypeName(n *tree_sitter.Node, src []byte, typeKinds map[string]struct{}) string {
	for cur := n; cur != nil; cur = cur.Parent() {
		if _, ok := typeKinds[cur.Kind()]; !ok {
			continue
		}
		if name := trimNodeText(cur.ChildByFieldName("name"), src); name != "" {
			return name
		}
		if id := firstDescendantKind(cur, map[string]struct{}{"identifier": {}, "type_identifier": {}, "property_identifier": {}}); id != nil {
			if name := strings.TrimSpace(id.Utf8Text(src)); name != "" {
				return name
			}
		}
	}
	return ""
}
