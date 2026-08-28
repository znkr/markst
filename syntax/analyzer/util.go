package analyzer

import (
	"fmt"
	"iter"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

var (
	underscore = name.Make("_")

	bools = map[string]value.Bool{
		"true":  value.Bool(true),
		"false": value.Bool(false),
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

// internal panics with a string describing the offending node. Reserve for
// parser/analyzer drift (running off a node list, encountering a kind no
// case expects); user-visible syntax errors thread through the IR via
// [analyzer.emitSyntaxError] / [analyzer.emitError].
func (a *analyzer) internal(n syntax.Node, format string, args ...any) {
	var sb strings.Builder
	sb.WriteString("analyzer internal error: ")
	fmt.Fprintf(&sb, format, args...)
	span := syntax.Span{}
	if n != nil {
		span = n.Span()
		fmt.Fprintf(&sb, "\n  at node: kind=%s span=[%d,%d) text=%q",
			n.Kind().String(), a.source.Position(span.Start), a.source.Position(span.End), truncate(n.Text(), 60))
	}
	panic(sb.String())
}

// expect reports whether n has the given kind. Mismatch with a
// [*syntax.Error] emits the parser error into the IR and returns false so
// callers bail; any other mismatch is drift and escalates to [internal].
func (a *analyzer) expect(kind syntax.Kind, n syntax.Node) bool {
	if n.Kind() == kind {
		return true
	}
	if err, ok := n.(*syntax.Error); ok {
		a.emitSyntaxError(err)
		return false
	}
	a.internal(n, "expected %s, but got %s", kind.String(), n.Kind().String())
	return false // unreachable
}

// expected emits a synthetic "expected X" error at the cursor's position
// (the end of the most recently consumed node). Used when the cursor runs
// out of nodes early; callers should return NoRef after.
func (a *analyzer) expected(ns *nodes, expected string) {
	span := syntax.Span{
		Start: ns.items[ns.pos-1].Span().End,
		End:   ns.items[ns.pos-1].Span().End,
	}
	a.emitError(span, fmt.Sprintf("expected %s", expected))
}

// unexpected emits n as a parser error if it is a [*syntax.Error];
// otherwise escalates to [internal] (parser/analyzer drift). Used at
// "I got something I can't lower" decision points.
func (a *analyzer) unexpected(n syntax.Node) {
	if err, ok := n.(*syntax.Error); ok {
		a.emitSyntaxError(err)
		return
	}
	a.internal(n, "unexpected node: kind=%s", n.Kind().String())
}

// collectChildErrors emits every [*syntax.Error] in n's subtree (when
// recursive is true) or its immediate children (when false) into the
// current IR block and reports whether any were found. Control-flow
// lowering (if/while/for/closure) uses it to surface parser errors at
// the outer block BEFORE opening inner blocks — otherwise structural
// errors get buried in unreachable IR or duplicated by per-position
// fallback paths.
func (a *analyzer) collectChildErrors(n syntax.Node, recursive bool) bool {
	if err, ok := n.(*syntax.Error); ok {
		a.emitSyntaxError(err)
		return true
	}
	inner, ok := n.(*syntax.Inner)
	if !ok {
		return false
	}
	found := false
	for _, c := range inner.Children() {
		if err, ok := c.(*syntax.Error); ok {
			a.emitSyntaxError(err)
			found = true
			continue
		}
		if recursive && a.collectChildErrors(c, true) {
			found = true
		}
	}
	return found
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// skipped holds the kinds the cursor steps over. Beyond trivia proper this
// includes the two markers `parseEmbeddedCodeExpr` splices in as *flat
// siblings* of the code expression it parses: the leading [syntax.KindHash]
// and the optional statement-terminating [syntax.KindSemicolon]. Neither is
// wrapped into a node standing for "an embedded code expression", so without
// this every context admitting `#code` — math content, attachment operands,
// fraction operands, spread values, math arguments, markup, code blocks —
// would have to skip them by hand, and any context that forgot would take an
// [analyzer.internal] panic on valid input. [syntax.KindParbreak] is here for
// the same reason and comes back out for markup cursors; see [markupSkipped].
var skipped = syntax.SetOf(
	syntax.KindSpace,
	syntax.KindParbreak,
	syntax.KindLineComment,
	syntax.KindBlockComment,
	syntax.KindHash,
	syntax.KindSemicolon,
)

// mathArgsSkipped is [skipped] for a MathArgs cursor, where `;` is meaningful:
// it separates the rows of a 2D element like `mat`.
var mathArgsSkipped = skipped.Remove(syntax.KindSemicolon)

// markupSkipped is [skipped] for a Markup cursor, where both a paragraph break
// and the whitespace between items are content rather than trivia. A parbreak
// lowers to a [value.Parbreak]; everywhere else it is whitespace the parser
// happened to buffer in a structural position (`/ \n\n  :` puts one between a
// term item's body and its colon), and stepping over it keeps that from reading
// as a missing node. A space is what separates `*bold*` from `word` in
// `*bold* word`, and what tells a smart quote whether it opens or closes.
var markupSkipped = skipped.Remove(syntax.KindParbreak).Remove(syntax.KindSpace)

// nodes is a cursor over a slice of syntax nodes, filtering out trivia.
type nodes struct {
	a     *analyzer
	items []syntax.Node
	pos   int
	// skips holds the kinds this cursor steps over; see [skipped].
	skips syntax.Set
}

// inner returns a cursor for the children of n. When n's kind doesn't
// match — either because the parser substituted a [*syntax.Error] or
// because the caller is being defensive about a malformed sub-tree — the
// returned cursor is empty so iteration is a no-op and callers can fall
// through to their cleanup with NoRef.
func (a *analyzer) inner(n syntax.Node, kind syntax.Kind) *nodes {
	if !a.expect(kind, n) {
		return &nodes{a: a, items: nil, skips: skipped}
	}
	if m, ok := n.(syntax.RootNode); ok {
		n = m.Inner
	}
	skips := skipped
	switch kind {
	case syntax.KindMathArgs:
		skips = mathArgsSkipped
	case syntax.KindMarkup:
		skips = markupSkipped
	}
	ns := &nodes{a: a, items: n.(*syntax.Inner).Children(), skips: skips}
	ns.skip()
	return ns
}

func (ns *nodes) advance() {
	ns.pos++
	ns.skip()
}

// skip moves the cursor past any skipped kinds, so it never rests on one.
func (ns *nodes) skip() {
	for ns.pos < len(ns.items) && ns.skips.Contains(ns.items[ns.pos].Kind()) {
		ns.pos++
	}
}

// take validates the current node has the expected kind, returns its
// literal, and advances. Returns ok=false when the cursor is exhausted
// or on a parser-error mismatch; drift (a non-error node of the wrong
// kind) escalates to [internal].
func (ns *nodes) take(kind syntax.Kind) (string, bool) {
	if ns.pos >= len(ns.items) {
		return "", false
	}
	n := ns.items[ns.pos]
	if !ns.a.expect(kind, n) {
		return "", false
	}
	ns.advance()
	return n.(*syntax.Leaf).Text(), true
}

// node returns the current node and advances.
func (ns *nodes) node() syntax.Node {
	if ns.pos >= len(ns.items) {
		ns.a.internal(nil, "unexpected end of nodes")
	}
	n := ns.items[ns.pos]
	ns.advance()
	return n
}

// nextContent returns the next node the cursor will yield that contributes
// content, without moving the cursor. Labels are skipped: one attaches to the
// item before it and emits nothing of its own, so it never stands between two
// neighbours. Returns nil when nothing is left.
func (ns *nodes) nextContent() syntax.Node {
	for i := ns.pos; i < len(ns.items); i++ {
		if k := ns.items[i].Kind(); !ns.skips.Contains(k) && k != syntax.KindLabel {
			return ns.items[i]
		}
	}
	return nil
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

// takeDelim consumes a delimiter, tolerating an [*syntax.Error] in its place.
// The parser rewrites an *opening* delimiter into an "unclosed delimiter" error
// (see [parser.expectClosing]), so a plain [nodes.take] would emit the error but
// leave the cursor on it. Emitting and advancing instead lets the rest of the
// construct still be lowered.
func (ns *nodes) takeDelim(kind syntax.Kind) {
	if ns.at(syntax.KindError) {
		ns.a.emitSyntaxError(ns.node().(*syntax.Error))
		return
	}
	ns.take(kind)
}

// inside returns a sequence of nodes inside the given open and close
// delimiters. If the parser emitted an [*syntax.Error] in place of the
// open delimiter (typically because the closing one was missing), the
// error is emitted into the IR and iteration proceeds on the remaining
// children so the rest of the construct still gets lowered.
func (ns *nodes) inside(open, close syntax.Kind) iter.Seq[syntax.Node] {
	ns.takeDelim(open)
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
	}
}

// leaf returns the text of n. When n's kind matches, the literal text
// is returned. A [*syntax.Error] in n's place is emitted into the IR and
// "" is returned so the caller can degrade gracefully (typically by
// emitting an invalid name that resolves to a downstream diagnostic).
// Any other kind mismatch is drift and escalates to [internal].
func (a *analyzer) leaf(n syntax.Node, kind syntax.Kind) string {
	if n.Kind() == kind {
		return n.(*syntax.Leaf).Text()
	}
	if err, ok := n.(*syntax.Error); ok {
		a.emitSyntaxError(err)
		return ""
	}
	a.internal(n, "leaf: expected %s, got %s", kind.String(), n.Kind().String())
	return ""
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
			case '\\', '"', '\'':
				sb.WriteRune(r)
			case 'u':
				// The scanner validates both the shape and the code point of a
				// `\u{...}` escape, so a string literal that reaches the
				// analyzer always decodes. Anything else is drift: keep the
				// text as written rather than panicking on user input.
				if i >= len(s) || s[i] != '{' {
					sb.WriteRune('\\')
					sb.WriteRune(r)
					continue
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
					sb.WriteString(`\u{`)
					sb.WriteString(hex)
					sb.WriteString("}")
					continue
				}
				sb.WriteRune(rune(num))
			default:
				// Likewise: the scanner rejects unknown escapes, so this is
				// only reachable on drift.
				sb.WriteRune('\\')
				sb.WriteRune(r)
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
