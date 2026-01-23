package analyzer

import (
	"iter"
	"strconv"
	"strings"
	"unicode/utf8"

	"znkr.io/writst/syntax"
)

// nodes is a cursor over a slice of syntax nodes, filtering out trivia.
type nodes struct {
	items []syntax.Node
	pos   int
}

// inner returns a cursor for the children of n, after validating its kind.
// Trivia (spaces, comments) are filtered out.
func inner(n syntax.Node, kind syntax.Kind) *nodes {
	if n.Kind != kind {
		panic("invalid node kind: expected " + kind.String() + ", got " + n.Kind.String())
	}
	ns := &nodes{items: n.AsInner().Children}
	for ns.at(syntax.KindSpace) || ns.at(syntax.KindLineComment) || ns.at(syntax.KindBlockComment) {
		ns.pos++
	}
	return ns
}

func (ns *nodes) advance() {
	ns.pos++
	for ns.at(syntax.KindSpace) || ns.at(syntax.KindLineComment) || ns.at(syntax.KindBlockComment) {
		ns.pos++
	}
}

// take validates the current node has the expected kind, returns its literal, and advances.
func (ns *nodes) take(kind syntax.Kind) string {
	if ns.pos >= len(ns.items) {
		panic("unexpected end of nodes, expected " + kind.String())
	}
	n := ns.items[ns.pos]
	if n.Kind != kind {
		panic("invalid node kind: expected " + kind.String() + ", got " + n.Kind.String())
	}
	ns.advance()
	return n.AsLeaf().Literal
}

// node returns the current node and advances.
func (ns *nodes) node() syntax.Node {
	if ns.pos >= len(ns.items) {
		panic("unexpected end of nodes")
	}
	n := ns.items[ns.pos]
	ns.advance()
	return n
}

// done returns true if there are no more nodes.
func (ns *nodes) done() bool {
	return ns.pos >= len(ns.items)
}

func (ns *nodes) at(kind syntax.Kind) bool {
	if ns.pos >= len(ns.items) {
		return false
	}
	return ns.items[ns.pos].Kind == kind
}

// all returns all nodes (ignoring current position).
func (ns *nodes) all() iter.Seq[syntax.Node] {
	return func(yield func(syntax.Node) bool) {
		for !ns.done() {
			if !yield(ns.node()) {
				return
			}
		}
	}
}

// inside returns a sequence of nodes inside the given open and close delimiters.
func (ns *nodes) inside(open, close syntax.Kind) iter.Seq[syntax.Node] {
	ns.take(open)
	return func(yield func(syntax.Node) bool) {
		for !ns.done() {
			n := ns.node()
			if n.Kind == close {
				return
			}
			if !yield(n) {
				return
			}
		}
		panic("expected closing delimiter: " + close.String())
	}
}

func (ns *nodes) finish() {
	if !ns.done() {
		panic("expected no more nodes, but found more")
	}
}

// leaf validates a single node has the expected kind and returns its literal.
func leaf(n syntax.Node, kind syntax.Kind) string {
	if n.Kind != kind {
		panic("invalid node kind: expected " + kind.String() + ", got " + n.Kind.String())
	}
	return n.AsLeaf().Literal
}

func unquote(s string) string {
	s = s[1 : len(s)-1] // trim quotes
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		i += w
		switch r {
		case '\\':
			r, w := utf8.DecodeRuneInString(s[i:])
			i += w
			switch r {
			case 'n':
				sb.WriteRune('\n')
			case 'r':
				sb.WriteRune('\r')
			case 't':
				sb.WriteRune('\t')
			case '\\', '"':
				sb.WriteRune(r)
			case 'u':
				if s[i] != '{' {
					panic("invalid unicode escape sequence")
				}
				i++
				start := i
				for ; i < len(s); i++ {
					if s[i] == '}' {
						break
					}
				}
				end := i
				i++
				hex := s[start:end]
				num, err := strconv.ParseInt(hex, 16, 32)
				if err != nil {
					panic("invalid unicode escape sequence")
				}
				sb.WriteRune(rune(num))
			default:
				panic("invalid escape sequence: \\" + string(r))
			}
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
