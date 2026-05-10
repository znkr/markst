// Package analyzer performs semantic analysis on Writst's untyped concrete
// syntax tree and produces the typed intermediate representation (IR) defined
// in package [ir].
//
// The entry point is [Analyze], which walks the [syntax.RootNode] produced by
// [parser.Parse] and converts each syntax node into an [expr.Expr] based on its
// [syntax.Kind]. Markup constructs become content expressions (headings,
// emphasis, etc.), code constructs become code expressions (let bindings,
// function calls, closures, etc.).
//
// # Error Handling
//
// Scanner and parser errors are embedded in the syntax tree as [syntax.Error]
// nodes. The analyzer collects these (along with any semantic errors it
// discovers) into a [syntax.ErrorList] returned as the error value. If the tree
// contains errors, the returned expression slice is nil.
package analyzer

import (
	"fmt"
	"regexp"
	"strings"
	"unique"

	"znkr.io/writst/builtin"
	"znkr.io/writst/expr"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// Option configures the analyzer.
type Option func(*analyzer)

// WithBindings adds names to the analyzer's scope, making them available
// as known variables during analysis.
func WithBindings(names ...unique.Handle[string]) Option {
	return func(a *analyzer) {
		for _, name := range names {
			a.bind(name)
		}
	}
}

// Analyze converts the syntax tree rooted at n into a slice of IR expressions.
// If the tree contains any syntax errors, they are collected and returned as
// a [syntax.ErrorList]; in that case the expression slice is nil.
func Analyze(n syntax.RootNode, opts ...Option) ([]expr.Expr, error) {
	a := &analyzer{}
	a.initScope()
	for _, opt := range opts {
		opt(a)
	}
	exprs := a.analyzeMarkup(n)
	if len(a.errors) > 0 {
		return nil, syntax.ErrorList(a.errors)
	}
	return exprs, nil
}

type analyzer struct {
	errors []*syntax.Error
	scope  *scope
}

func (a *analyzer) error(n *syntax.Error) {
	a.errors = append(a.errors, n)
}

// Scope ///////////////////////////////////////////////////////////////////////

type scope struct {
	parent   *scope
	bindings map[unique.Handle[string]]bool
	boundary bool                                  // true for closure scope frames
	captures map[unique.Handle[string]]syntax.Span // populated during analysis of the closure body; value is the span of the first reference
}

func (a *analyzer) initScope() {
	bindings := make(map[unique.Handle[string]]bool, len(builtin.Universe))
	for name := range builtin.Universe {
		bindings[name] = true
	}
	a.scope = &scope{bindings: bindings}
}

func (a *analyzer) openScope() *scope {
	a.scope = &scope{parent: a.scope}
	return a.scope
}

func (a *analyzer) openClosureScope() *scope {
	a.scope = &scope{parent: a.scope, boundary: true}
	return a.scope
}

func (a *analyzer) closeScope() {
	a.scope = a.scope.parent
}

func (a *analyzer) bind(name unique.Handle[string]) {
	if a.scope.bindings == nil {
		a.scope.bindings = make(map[unique.Handle[string]]bool)
	}
	a.scope.bindings[name] = true
}

// lookup checks whether name is in scope. If the lookup crosses one or more
// closure boundaries before finding the binding, the name is recorded as a
// capture on every boundary frame crossed. The span is attached to the capture
// for error reporting.
//
// Recording on every boundary (not just the innermost) is necessary because
// each closure carries its own captured environment at runtime. Consider:
//
//	let x = 1
//	let outer = () => {
//	  let inner = () => x  // references x from two scopes out
//	  inner()
//	}
//
// When inner references x, the lookup crosses both inner's and outer's closure
// boundaries. Inner must capture x so it can read it when invoked. Outer must
// also capture x so that, when outer runs and constructs the inner closure, it
// has x available to put into inner's environment. If outer didn't capture x,
// it would have no way to forward x into inner once outer is invoked outside
// the scope where x is bound.
//
// The walk is two-phase: boundary frames crossed are collected speculatively
// and captures are committed only if the name is found. An unresolved name
// must not leave capture entries behind on closures it happened to be nested
// inside of.
func (a *analyzer) lookup(name unique.Handle[string], span syntax.Span) bool {
	var crossed []*scope // boundary frames crossed before finding the name
	for s := a.scope; s != nil; s = s.parent {
		if s.bindings[name] {
			for _, b := range crossed {
				a.addCapture(b, name, span)
			}
			return true
		}
		if s.boundary {
			crossed = append(crossed, s)
		}
	}
	return false
}

// addCapture records a capture on a boundary scope frame. Only the first
// reference span is kept for any given name.
func (a *analyzer) addCapture(s *scope, name unique.Handle[string], span syntax.Span) {
	if _, ok := s.captures[name]; ok {
		return
	}
	if s.captures == nil {
		s.captures = make(map[unique.Handle[string]]syntax.Span)
	}
	s.captures[name] = span
}

// couldBeSubtractionRe matches identifiers like "x-1" that might be
// subtraction with missing spaces.
var couldBeSubtractionRe = regexp.MustCompile(`(-)(\d+)$`)

func (a *analyzer) checkIdent(name unique.Handle[string], span syntax.Span) {
	if a.lookup(name, span) {
		return
	}
	var hints []string
	if m := couldBeSubtractionRe.FindAllStringSubmatch(name.Value(), -1); m != nil {
		sign := m[0][1]
		num := m[0][2]
		hints = append(hints, fmt.Sprintf("if you meant to use subtraction, try adding spaces around the minus sign: `%s %s %s`", name.Value()[:len(name.Value())-len(m[0][0])], sign, num))
	}
	a.error(syntax.NewError(span, fmt.Sprintf("unknown variable: %s", name.Value()), name.Value(), hints...))
}

func (a *analyzer) bindPatterns(patterns []expr.DestructPattern) {
	for _, p := range patterns {
		switch p := p.(type) {
		case *expr.DestructIdent:
			a.bind(p.Ident().Name())
		case *expr.DestructNamed:
			a.bind(p.Pattern().Name())
		case *expr.DestructSink:
			if p.Ident() != nil {
				a.bind(p.Ident().Name())
			}
		}
	}
}

func (a *analyzer) analyzeMarkup(n syntax.Node) []expr.Expr {
	var body []expr.Expr
	ns := a.inner(n, syntax.KindMarkup)
	defer ns.finish()
	for n := range ns.all() {
		switch n.Kind() {
		case syntax.KindSemicolon:
			continue
		case syntax.KindHash:
			// skip hash markers
		case syntax.KindError:
			// handle errors gracefully
			a.error(n.(*syntax.Error))
		default:
			n0 := a.analyzeExpr(n)
			if n0 == nil {
				continue
			}
			body = append(body, n0)
			// Let bindings accumulate in scope for subsequent expressions.
			if let, ok := n0.(*expr.LetBinding); ok && let != nil {
				a.bindPatterns(let.Pattern())
			}
		}
	}
	return body
}

func (a *analyzer) analyzeExpr(n syntax.Node) expr.Expr {
	switch n.Kind() {
	case syntax.KindLabel:
		label := a.leaf(n, syntax.KindLabel)
		label = label[1 : len(label)-1] // trim '<>'
		return expr.NewConstExpr(n.Span(), &value.Label{Name: unique.Make(label)})
	case syntax.KindHeading:
		return a.analyzeHeading(n)
	case syntax.KindListItem:
		return a.analyzeListItem(n)
	case syntax.KindEnumItem:
		return a.analyzeEnumItem(n)
	case syntax.KindTermItem:
		return a.analyzeTermItem(n)
	case syntax.KindText:
		return expr.NewConstExpr(n.Span(), &value.Text{Text: strings.TrimSpace(n.Text())})
	case syntax.KindEscape:
		return expr.NewConstExpr(n.Span(), &value.Text{Text: unescape(n.Text())})
	case syntax.KindShorthand:
		return expr.NewConstExpr(n.Span(), &value.Text{Text: unshorthand(n.Text())})
	case syntax.KindSmartQuote:
		// TODO: implement smart quotes properly.
		return expr.NewConstExpr(n.Span(), &value.Text{Text: n.Text()})
	case syntax.KindLinebreak:
		return expr.NewConstExpr(n.Span(), &value.Linebreak{})
	case syntax.KindParbreak:
		return expr.NewConstExpr(n.Span(), &value.Parbreak{})
	case syntax.KindStrong:
		ns := a.inner(n, syntax.KindStrong)
		defer ns.finish()
		ns.take(syntax.KindStar)
		n0 := ns.node()
		ns.take(syntax.KindStar)
		return expr.NewStrongExpr(n.Span(), a.analyzeMarkup(n0))
	case syntax.KindEmph:
		ns := a.inner(n, syntax.KindEmph)
		defer ns.finish()
		ns.take(syntax.KindUnderscore)
		n0 := ns.node()
		ns.take(syntax.KindUnderscore)
		return expr.NewEmphExpr(n.Span(), a.analyzeMarkup(n0))
	case syntax.KindRaw:
		return a.analyzeRaw(n)
	case syntax.KindLink:
		lit := n.Text()
		return expr.NewLinkExpr(
			n.Span(),
			lit,
			[]expr.Expr{expr.NewConstExpr(n.Span(), &value.Text{Text: lit})},
		)
	case syntax.KindRef:
		return a.analyzeRef(n)
	case syntax.KindNone:
		return expr.NewConstExpr(n.Span(), value.None{})
	case syntax.KindAuto:
		return expr.NewConstExpr(n.Span(), value.Auto{})
	case syntax.KindBool:
		return a.analyzeBool(n)
	case syntax.KindInt:
		return a.analyzeInt(n)
	case syntax.KindFloat:
		return a.analyzeFloat(n)
	case syntax.KindNumeric:
		return a.analyzeNumeric(n)
	case syntax.KindStr:
		return a.analyzeStr(n)
	case syntax.KindIdent:
		return a.analyzeIdent(n)
	case syntax.KindCodeBlock:
		return a.analyzeCodeBlock(n)
	case syntax.KindContentBlock:
		return a.analyzeContentBlock(n)
	case syntax.KindParenthesized:
		return a.analyzeParenthesized(n)
	case syntax.KindArray:
		return a.analyzeArray(n)
	case syntax.KindDict:
		return a.analyzeDict(n)
	case syntax.KindUnary:
		return a.analyzeUnary(n)
	case syntax.KindBinary:
		return a.analyzeBinary(n)
	case syntax.KindFieldAccess:
		return a.analyzeFieldAccess(n)
	case syntax.KindFuncCall:
		return a.analyzeFuncCall(n)
	case syntax.KindClosure:
		return a.analyzeClosure(n)
	case syntax.KindLetBinding:
		return a.analyzeLetBinding(n)
	case syntax.KindSetRule:
		return a.analyzeSetRule(n)
	case syntax.KindShowRule:
		return a.analyzeShowRule(n)
	case syntax.KindConditional:
		return a.analyzeConditional(n)
	case syntax.KindWhileLoop:
		return a.analyzeWhileLoop(n)
	case syntax.KindForLoop:
		return a.analyzeForLoop(n)
	case syntax.KindLoopBreak:
		return a.analyzeLoopBreak(n)
	case syntax.KindLoopContinue:
		return a.analyzeLoopContinue(n)
	case syntax.KindFuncReturn:
		return a.analyzeFuncReturn(n)
	case syntax.KindContextual:
		return a.analyzeContextual(n)
	case syntax.KindModuleInclude:
		return a.analyzeModuleInclude(n)
	case syntax.KindDestructAssignment:
		return a.analyzeDestructAssignment(n)
	case syntax.KindUnderscore:
		return expr.NewIdent(n.Span(), underscore)
	case syntax.KindError:
		a.unexpected(n)
	default:
		panic("unsupported syntax kind: " + n.Kind().String())
	}
	panic("never reached")
}
