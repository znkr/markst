package parser

import (
	"fmt"

	"znkr.io/markst/syntax"
)

// asErrorNode converts a node into an error node with the given diagnostic
// message, preserving the original node's span and text. If the node is
// already an error, it is returned as-is.
func (p *parser) asErrorNode(node syntax.Node, format string, args ...any) *syntax.Error {
	if node.Kind() == syntax.KindError {
		return node.(*syntax.Error)
	}
	return p.a.Error(
		node.Span(),
		fmt.Sprintf(format, args...),
	)
}
