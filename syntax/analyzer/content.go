package analyzer

import (
	"strings"
	"unique"

	"znkr.io/writst/model/ir"
	"znkr.io/writst/syntax"
)

func (a *analyzer) analyzeContent(n syntax.Node) ir.Content {
	var body ir.Content
	for n := range inner(n, syntax.KindMarkup).all() {
		switch n.Kind {
		case syntax.KindHash:
			// skip hash markers
		default:
			body = append(body, a.analyzeExpr(n))
		}
	}
	return body
}

func (a *analyzer) analyzeExpr(n syntax.Node) ir.Expr {
	switch n.Kind {
	case syntax.KindLabel:
		label := leaf(n, syntax.KindLabel)
		label = label[1 : len(label)-1] // trim '<>'
		return &ir.Label{Name: unique.Make(label)}
	case syntax.KindHeading:
		return a.analyzeHeading(n)
	case syntax.KindListItem:
		return a.analyzeListItem(n)
	case syntax.KindEnumItem:
		return a.analyzeEnumItem(n)
	case syntax.KindTermItem:
		return a.analyzeTermItem(n)
	case syntax.KindText:
		return &ir.Text{Value: strings.TrimSpace(n.AsLeaf().Literal)}
	case syntax.KindEscape:
		return &ir.Text{Value: unescape(n.AsLeaf().Literal)}
	case syntax.KindShorthand:
		return &ir.Text{Value: unshorthand(n.AsLeaf().Literal)}
	case syntax.KindSmartQuote:
		// TODO: implement smart quotes properly.
		return &ir.Text{Value: n.AsLeaf().Literal}
	case syntax.KindLinebreak:
		return &ir.Linebreak{}
	case syntax.KindParbreak:
		return &ir.Parbreak{}
	case syntax.KindStrong:
		ns := inner(n, syntax.KindStrong)
		defer ns.finish()
		ns.take(syntax.KindStar)
		n0 := ns.node()
		ns.take(syntax.KindStar)
		return &ir.Strong{Body: a.analyzeContent(n0)}
	case syntax.KindEmph:
		ns := inner(n, syntax.KindEmph)
		defer ns.finish()
		ns.take(syntax.KindUnderscore)
		n0 := ns.node()
		ns.take(syntax.KindUnderscore)
		return &ir.Emph{Body: a.analyzeContent(n0)}
	case syntax.KindRaw:
		return a.analyzeRaw(n)
	case syntax.KindLink:
		lit := n.AsLeaf().Literal
		return &ir.Link{
			Dest: lit,
			Body: ir.Content{&ir.Text{Value: lit}},
		}
	case syntax.KindRef:
		return a.analyzeRef(n)
	case syntax.KindNone:
		return none
	case syntax.KindAuto:
		return auto
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
	case syntax.KindNamed:
		return a.analyzeNamed(n)
	case syntax.KindKeyed:
		return a.analyzeKeyed(n)
	case syntax.KindSpread:
		return a.analyzeSpread(n)
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
	case syntax.KindDestructuring:
		return a.analyzeDestructuring(n)
	case syntax.KindUnderscore:
		return &ir.Underscore{}
	default:
		panic("unsupported syntax kind: " + n.Kind.String())
	}
}

var none = &ir.None{}
var auto = &ir.Auto{}
