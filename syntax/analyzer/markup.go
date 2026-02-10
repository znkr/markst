package analyzer

import (
	"strconv"
	"strings"
	"unique"

	"znkr.io/writst/ir"
	"znkr.io/writst/syntax"
)

func (a *analyzer) analyzeHeading(n syntax.Node) *ir.HeadingExpr {
	ns := a.inner(n, syntax.KindHeading)
	defer ns.finish()
	level := len(ns.take(syntax.KindHeadingMarker))
	body := a.analyzeMarkup(ns.node())
	return ir.NewHeadingExpr(n.Span(), level, body)
}

func (a *analyzer) analyzeListItem(n syntax.Node) *ir.ListItemExpr {
	ns := a.inner(n, syntax.KindListItem)
	defer ns.finish()
	ns.take(syntax.KindListMarker)
	body := a.analyzeMarkup(ns.node())
	return ir.NewListItemExpr(n.Span(), body)
}

func (a *analyzer) analyzeEnumItem(n syntax.Node) *ir.EnumItemExpr {
	ns := a.inner(n, syntax.KindEnumItem)
	defer ns.finish()
	number := -1
	marker := ns.take(syntax.KindEnumMarker)
	if marker != "+" {
		n, err := strconv.ParseInt(strings.TrimSuffix(marker, "."), 10, 64)
		if err != nil {
			panic("invalid enum marker: " + marker)
		}
		number = int(n)
	}
	body := a.analyzeMarkup(ns.node())
	return ir.NewEnumItemExpr(n.Span(), number, body)
}

func (a *analyzer) analyzeTermItem(n syntax.Node) *ir.TermItemExpr {
	ns := a.inner(n, syntax.KindTermItem)
	defer ns.finish()
	ns.take(syntax.KindTermMarker)
	term := a.analyzeMarkup(ns.node())
	ns.take(syntax.KindColon)
	body := a.analyzeMarkup(ns.node())
	return ir.NewTermItemExpr(n.Span(), term, body)
}

func (a *analyzer) analyzeRef(n syntax.Node) *ir.RefExpr {
	ns := a.inner(n, syntax.KindRef)
	defer ns.finish()
	marker := ns.take(syntax.KindRefMarker)
	target := marker[1:] // trim '@'
	var supplement *ir.ContentBlock
	if !ns.done() {
		supplement = a.analyzeContentBlock(ns.node())
	}
	return ir.NewRefExpr(n.Span(), unique.Make(target), supplement)
}

func (a *analyzer) analyzeContentBlock(n syntax.Node) *ir.ContentBlock {
	ns := a.inner(n, syntax.KindContentBlock)
	defer ns.finish()
	ns.take(syntax.KindLeftBracket)
	n0 := ns.node()
	ns.take(syntax.KindRightBracket)
	return ir.NewContentBlock(n.Span(), a.analyzeMarkup(n0))
}

func (a *analyzer) analyzeRaw(n syntax.Node) *ir.Const {
	ns := a.inner(n, syntax.KindRaw)
	defer ns.finish()
	marker := ns.take(syntax.KindRawDelim)
	var lang string
	if ns.at(syntax.KindRawLang) {
		lang = ns.take(syntax.KindRawLang)
	}
	var lines []string
	for child := range ns.all() {
		switch child.Kind() {
		case syntax.KindRawDelim:
			// closing delimiter - we're done
		case syntax.KindRawTrimmed:
			continue
		default:
			if child.Kind() != syntax.KindText {
				panic("invalid node kind in raw: " + child.Kind().String())
			}
			lines = append(lines, child.Text())
		}
	}
	return ir.NewConst(n.Span(), &ir.Raw{Block: marker != "`", Lang: lang, Lines: lines})
}
