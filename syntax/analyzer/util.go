package analyzer

import (
	"fmt"
	"iter"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
	"unique"

	"znkr.io/writst/ir"
	"znkr.io/writst/syntax"
)

var (
	underscore = unique.Make("_")

	bools = map[string]ir.Bool{
		"true":  ir.Bool(true),
		"false": ir.Bool(false),
	}

	shorthandMap = map[string]string{
		"--":  "\u2013", // en dash
		"---": "\u2014", // em dash
		"...": "\u2026", // ellipsis
		"-?":  "\u00AD", // soft hyphen
		"-":   "\u2212", // minus
		"~":   "\u00A0", // non-breaking space
	}
)

type unexpected struct {
	n        syntax.Node
	expected syntax.Kind
}

func (a *analyzer) expect(kind syntax.Kind, n syntax.Node) {
	if n.Kind() != kind {
		panic(unexpected{n, kind})
	}
}

func (a *analyzer) expected(ns *nodes, expected string) {
	span := syntax.Span{
		Start: ns.items[ns.pos-1].Span().End,
		End:   ns.items[ns.pos-1].Span().End,
	}
	panic(unexpected{
		n: syntax.NewError(span, fmt.Sprintf("expected %s", expected), ""),
	})
}

func (a *analyzer) unexpected(n syntax.Node) {
	panic(unexpected{n: n})
}

func (a *analyzer) handleRecover(p0 any) bool {
	if p0 == nil {
		return false
	}
	p, ok := p0.(unexpected)
	if !ok {
		panic(p0)
	}
	if err, ok := p.n.(*syntax.Error); ok {
		a.error(err)
		return true
	}
	panic(fmt.Sprintf("expected %s, but got %s", p.expected.String(), p.n.Kind().String()))
}

// nodes is a cursor over a slice of syntax nodes, filtering out trivia.
type nodes struct {
	a     *analyzer
	items []syntax.Node
	pos   int
}

// inner returns a cursor for the children of n, after validating its kind.
// Trivia (spaces, comments) are filtered out.
func (a *analyzer) inner(n syntax.Node, kind syntax.Kind) *nodes {
	a.expect(kind, n)
	if m, ok := n.(syntax.RootNode); ok {
		n = m.Inner
	}
	ns := &nodes{a: a, items: n.(*syntax.Inner).Children()}
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
	ns.a.expect(kind, n)
	ns.advance()
	return n.(*syntax.Leaf).Text()
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
	return ns.items[ns.pos].Kind() == kind
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
			if n.Kind() == close {
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
	if ns.a.handleRecover(recover()) {
		return
	}
	if !ns.done() {
		panic("expected no more nodes, but found more")
	}
}

// leaf validates a single node has the expected kind and returns its literal.
func (a *analyzer) leaf(n syntax.Node, kind syntax.Kind) string {
	a.expect(kind, n)
	return n.(*syntax.Leaf).Text()
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

func unescape(lit string) string {
	if strings.HasPrefix(lit, "\\u{") {
		v := lit[3 : len(lit)-1] // inside of \u{...}
		x, err := strconv.ParseInt(v, 16, 32)
		if err != nil || x > unicode.MaxRune || (0xD800 <= x && x < 0xE000) {
			panic("invalid unicode escape: " + lit)
		}
		return string(rune(x))
	}
	return lit[1:]
}

func unshorthand(lit string) string {
	return shorthandMap[lit]
}
