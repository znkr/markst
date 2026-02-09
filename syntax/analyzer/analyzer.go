package analyzer

import (
	"strings"
	"unique"

	"znkr.io/writst/model/ir"
	"znkr.io/writst/syntax"
)

func Analyze(n syntax.RootNode) (*ir.ContentExpr, error) {
	a := &analyzer{}
	exprs := a.analyzeContent(n)
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

func (a *analyzer) analyzeContent(n syntax.Node) *ir.ContentExpr {
	var body []ir.Expr
	for n := range a.inner(n, syntax.KindMarkup).all() {
		switch n.Kind() {
		case syntax.KindHash:
			// skip hash markers
		default:
			n0 := a.analyzeExpr(n)
			if n0 == nil {
				continue
			}
			body = append(body, n0)
		}
	}
	return ir.NewContentExpr(n.Span(), body)
}

func (a *analyzer) analyzeExpr(n syntax.Node) ir.Expr {
	switch n.Kind() {
	case syntax.KindLabel:
		label := a.leaf(n, syntax.KindLabel)
		label = label[1 : len(label)-1] // trim '<>'
		return ir.NewConst(n.Span(), &ir.Label{Name: unique.Make(label)})
	case syntax.KindHeading:
		return a.analyzeHeading(n)
	case syntax.KindListItem:
		return a.analyzeListItem(n)
	case syntax.KindEnumItem:
		return a.analyzeEnumItem(n)
	case syntax.KindTermItem:
		return a.analyzeTermItem(n)
	case syntax.KindText:
		return ir.NewConst(n.Span(), &ir.Text{Value: strings.TrimSpace(n.Text())})
	case syntax.KindEscape:
		return ir.NewConst(n.Span(), &ir.Text{Value: unescape(n.Text())})
	case syntax.KindShorthand:
		return ir.NewConst(n.Span(), &ir.Text{Value: unshorthand(n.Text())})
	case syntax.KindSmartQuote:
		// TODO: implement smart quotes properly.
		return ir.NewConst(n.Span(), &ir.Text{Value: n.Text()})
	case syntax.KindLinebreak:
		return ir.NewConst(n.Span(), &ir.Linebreak{})
	case syntax.KindParbreak:
		return ir.NewConst(n.Span(), &ir.Parbreak{})
	case syntax.KindStrong:
		ns := a.inner(n, syntax.KindStrong)
		defer ns.finish()
		ns.take(syntax.KindStar)
		n0 := ns.node()
		ns.take(syntax.KindStar)
		return ir.NewStrongExpr(n.Span(), a.analyzeContent(n0))
	case syntax.KindEmph:
		ns := a.inner(n, syntax.KindEmph)
		defer ns.finish()
		ns.take(syntax.KindUnderscore)
		n0 := ns.node()
		ns.take(syntax.KindUnderscore)
		return ir.NewEmphExpr(n.Span(), a.analyzeContent(n0))
	case syntax.KindRaw:
		return a.analyzeRaw(n)
	case syntax.KindLink:
		lit := n.Text()
		return ir.NewLinkExpr(
			n.Span(),
			lit,
			ir.NewContentExpr(n.Span(), []ir.Expr{ir.NewConst(n.Span(), &ir.Text{Value: lit})}),
		)
	case syntax.KindRef:
		return a.analyzeRef(n)
	case syntax.KindNone:
		return ir.NewConst(n.Span(), ir.None{})
	case syntax.KindAuto:
		return ir.NewConst(n.Span(), ir.Auto{})
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
		return a.analyzeCodeContentBlock(n)
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
		a.error(n.(*syntax.Error))
		return nil
	default:
		panic("unsupported syntax kind: " + n.Kind().String())
	}
}
