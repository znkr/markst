package format

import (
	"fmt"
	"strings"

	"znkr.io/writst/syntax"
)

func Format(n syntax.Node) string {
	var sb strings.Builder
	format(&sb, n, "")
	return sb.String()
}

func format(sb *strings.Builder, n syntax.Node, indent string) {
	kind := strings.TrimPrefix(n.Kind.String(), "Kind")
	switch v := n.Value.(type) {
	case *syntax.LeafValue:
		fmt.Fprintf(sb, "%s%s: %q\n", indent, kind, v.Literal)
	case *syntax.InnerValue:
		fmt.Fprintf(sb, "%s%s\n", indent, kind)
		for _, child := range v.Children {
			format(sb, child, indent+"  ")
		}
	case *syntax.ErrorValue:
		if v.Literal != "" {
			fmt.Fprintf(sb, "%s%s: %q\n", indent, kind, v.Literal)
		} else {
			fmt.Fprintf(sb, "%s%s\n", indent, kind)
		}
		fmt.Fprintf(sb, "%s  Msg: %q\n", indent, v.Message)
		for _, hint := range v.Hints {
			fmt.Fprintf(sb, "%s  Hint: %q\n", indent, hint)
		}
	}
}
