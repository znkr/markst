package analyzer

import (
	"strconv"
	"strings"
	"unicode"
	"unique"

	"znkr.io/writst/model/ir"
	"znkr.io/writst/syntax"
)

func (a *analyzer) analyzeHeading(n syntax.Node) *ir.HeadingExpr {
	ns := inner(n, syntax.KindHeading)
	defer ns.finish()
	level := len(ns.take(syntax.KindHeadingMarker))
	body := a.analyzeContent(ns.node())
	return &ir.HeadingExpr{Level: level, Body: body}
}

func (a *analyzer) analyzeListItem(n syntax.Node) *ir.ListItemExpr {
	ns := inner(n, syntax.KindListItem)
	defer ns.finish()
	ns.take(syntax.KindListMarker)
	body := a.analyzeContent(ns.node())
	return &ir.ListItemExpr{Body: body}
}

func (a *analyzer) analyzeEnumItem(n syntax.Node) *ir.EnumItemExpr {
	ns := inner(n, syntax.KindEnumItem)
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
	body := a.analyzeContent(ns.node())
	return &ir.EnumItemExpr{Number: number, Body: body}
}

func (a *analyzer) analyzeTermItem(n syntax.Node) *ir.TermItemExpr {
	ns := inner(n, syntax.KindTermItem)
	defer ns.finish()
	ns.take(syntax.KindTermMarker)
	term := a.analyzeContent(ns.node())
	ns.take(syntax.KindColon)
	body := a.analyzeContent(ns.node())
	return &ir.TermItemExpr{Term: term, Description: body}
}

func (a *analyzer) analyzeRef(n syntax.Node) *ir.RefExpr {
	ns := inner(n, syntax.KindRef)
	defer ns.finish()
	marker := ns.take(syntax.KindRefMarker)
	target := marker[1:] // trim '@'
	var supplement ir.ContentExpr
	if !ns.done() {
		supplement = a.analyzeContentBlock(ns.node())
	}
	return &ir.RefExpr{Target: unique.Make(target), Supplement: supplement}
}

func (a *analyzer) analyzeContentBlock(n syntax.Node) ir.ContentExpr {
	ns := inner(n, syntax.KindContentBlock)
	defer ns.finish()
	ns.take(syntax.KindLeftBracket)
	n0 := ns.node()
	ns.take(syntax.KindRightBracket)
	return a.analyzeContent(n0)
}

func (a *analyzer) analyzeRaw(n syntax.Node) *ir.Const {
	ns := inner(n, syntax.KindRaw)
	defer ns.finish()
	marker := ns.take(syntax.KindRawDelim)
	var lang string
	if ns.at(syntax.KindRawLang) {
		lang = ns.take(syntax.KindRawLang)
	}
	var lines []string
	for child := range ns.all() {
		switch child.Kind {
		case syntax.KindRawDelim:
			// closing delimiter - we're done
		case syntax.KindRawTrimmed:
			continue
		default:
			if child.Kind != syntax.KindText {
				panic("invalid node kind in raw: " + child.Kind.String())
			}
			lines = append(lines, child.AsLeaf().Literal)
		}
	}
	return &ir.Const{Value: &ir.Raw{Block: marker != "`", Lang: lang, Lines: lines}}
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

var shorthandMap = map[string]string{
	"--":  "\u2013", // en dash
	"---": "\u2014", // em dash
	"...": "\u2026", // ellipsis
	"-?":  "\u00AD", // soft hyphen
	"-":   "\u2212", // minus
	"~":   "\u00A0", // non-breaking space
}

func unshorthand(lit string) string {
	return shorthandMap[lit]
}
