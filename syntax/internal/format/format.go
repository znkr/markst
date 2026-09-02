// Package format provides a tree-dump formatter for syntax nodes, used in
// parser golden test files. The output shows each node on its own line with
// indentation reflecting the tree structure.
package format

import (
	"fmt"
	"strings"

	"znkr.io/markst/syntax"
)

// Format returns a human-readable tree dump of a parsed tree. Leaf nodes show
// their kind and quoted text; Inner nodes show their kind and recursively
// format children with increased indentation; Error nodes show their kind,
// text, message, and hints. The text comes from the source the tree was parsed
// from, which is what a node's span points into.
func Format(root syntax.RootNode) string {
	var sb strings.Builder
	format(&sb, root.Src, root.Inner, "")
	return sb.String()
}

func format(sb *strings.Builder, src []byte, n syntax.Node, indent string) {
	kind := strings.TrimPrefix(n.Kind().String(), "Kind")
	switch n := n.(type) {
	case *syntax.Leaf:
		fmt.Fprintf(sb, "%s%s: %q\n", indent, kind, syntax.Text(src, n))
	case *syntax.Inner:
		fmt.Fprintf(sb, "%s%s\n", indent, kind)
		for _, child := range n.Children() {
			format(sb, src, child, indent+"  ")
		}
	case *syntax.Error:
		if text := syntax.Text(src, n); len(text) > 0 {
			fmt.Fprintf(sb, "%s%s: %q\n", indent, kind, text)
		} else {
			fmt.Fprintf(sb, "%s%s\n", indent, kind)
		}
		fmt.Fprintf(sb, "%s  Msg: %q\n", indent, n.Message())
		for _, hint := range n.Hints() {
			fmt.Fprintf(sb, "%s  Hint: %q\n", indent, hint)
		}
	}
}
