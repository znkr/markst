package format

import (
	"fmt"
	"strings"

	"znkr.io/writst/syntax"
)

func Format(n syntax.Node) string {
	if n, ok := n.(syntax.RootNode); ok {
		return Format(n.Inner)
	}
	var sb strings.Builder
	format(&sb, n, "")
	return sb.String()
}

func format(sb *strings.Builder, n syntax.Node, indent string) {
	kind := strings.TrimPrefix(n.Kind().String(), "Kind")
	switch n := n.(type) {
	case *syntax.Leaf:
		fmt.Fprintf(sb, "%s%s: %q\n", indent, kind, n.Text())
	case *syntax.Inner:
		fmt.Fprintf(sb, "%s%s\n", indent, kind)
		for _, child := range n.Children() {
			format(sb, child, indent+"  ")
		}
	case *syntax.Error:
		if n.Text() != "" {
			fmt.Fprintf(sb, "%s%s: %q\n", indent, kind, n.Text())
		} else {
			fmt.Fprintf(sb, "%s%s\n", indent, kind)
		}
		fmt.Fprintf(sb, "%s  Msg: %q\n", indent, n.Message())
		for _, hint := range n.Hints() {
			fmt.Fprintf(sb, "%s  Hint: %q\n", indent, hint)
		}
	}
}
