package parser

import (
	"fmt"

	"znkr.io/writst/syntax"
)

func asErrorNode(node syntax.Node, format string, args ...any) *syntax.Error {
	if node.Kind() == syntax.KindError {
		return node.(*syntax.Error)
	}
	return syntax.NewError(
		node.Span(),
		fmt.Sprintf(format, args...),
		node.Text(),
	)
}
