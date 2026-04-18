package analyzer

import (
	"strings"
	"unique"

	"znkr.io/writst/ir"
	"znkr.io/writst/syntax"
)

func Analyze(n syntax.RootNode) ([]ir.Expr, error) {
	a := &analyzer{}
	exprs := a.analyzeMarkup(n)
	if len(a.errors) > 0 {
		return nil, syntax.ErrorList(a.errors)
	}
	return exprs, nil
}

type analyzer struct {
	errors []*syntax.Error
}

func (a *analyzer) error(n *syntax.Error) {
	a.errors = append(a.errors, n)
}

func (a *analyzer) analyzeMarkup(n syntax.Node) []ir.Expr {
	var body []ir.Expr
	ns := a.inner(n, syntax.KindMarkup)
	defer ns.finish()
	for n := range ns.all() {
		switch n.Kind() {
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
		}
	}
	return body
}

func (a *analyzer) analyzeExpr(n syntax.Node) ir.Expr {
	switch n.Kind() {
	case syntax.KindLabel:
		label := a.leaf(n, syntax.KindLabel)
		label = label[1 : len(label)-1] // trim '<>'
		return ir.NewConstExpr(n.Span(), &ir.Label{Name: unique.Make(label)})
	case syntax.KindHeading:
		return a.analyzeHeading(n)
	case syntax.KindListItem:
		return a.analyzeListItem(n)
	case syntax.KindEnumItem:
		return a.analyzeEnumItem(n)
	case syntax.KindTermItem:
		return a.analyzeTermItem(n)
	case syntax.KindText:
		return ir.NewConstExpr(n.Span(), &ir.Text{Text: strings.TrimSpace(n.Text())})
	case syntax.KindEscape:
		return ir.NewConstExpr(n.Span(), &ir.Text{Text: unescape(n.Text())})
	case syntax.KindShorthand:
		return ir.NewConstExpr(n.Span(), &ir.Text{Text: unshorthand(n.Text())})
	case syntax.KindSmartQuote:
		// TODO: implement smart quotes properly.
		return ir.NewConstExpr(n.Span(), &ir.Text{Text: n.Text()})
	case syntax.KindLinebreak:
		return ir.NewConstExpr(n.Span(), &ir.Linebreak{})
	case syntax.KindParbreak:
		return ir.NewConstExpr(n.Span(), &ir.Parbreak{})
	case syntax.KindStrong:
		ns := a.inner(n, syntax.KindStrong)
		defer ns.finish()
		ns.take(syntax.KindStar)
		n0 := ns.node()
		ns.take(syntax.KindStar)
		return ir.NewStrongExpr(n.Span(), a.analyzeMarkup(n0))
	case syntax.KindEmph:
		ns := a.inner(n, syntax.KindEmph)
		defer ns.finish()
		ns.take(syntax.KindUnderscore)
		n0 := ns.node()
		ns.take(syntax.KindUnderscore)
		return ir.NewEmphExpr(n.Span(), a.analyzeMarkup(n0))
	case syntax.KindRaw:
		return a.analyzeRaw(n)
	case syntax.KindLink:
		lit := n.Text()
		return ir.NewLinkExpr(
			n.Span(),
			lit,
			[]ir.Expr{ir.NewConstExpr(n.Span(), &ir.Text{Text: lit})},
		)
	case syntax.KindRef:
		return a.analyzeRef(n)
	case syntax.KindNone:
		return ir.NewConstExpr(n.Span(), ir.None{})
	case syntax.KindAuto:
		return ir.NewConstExpr(n.Span(), ir.Auto{})
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
		return ir.NewIdent(n.Span(), underscore)
	case syntax.KindError:
		a.unexpected(n)
	default:
		panic("unsupported syntax kind: " + n.Kind().String())
	}
	panic("never reached")
}
