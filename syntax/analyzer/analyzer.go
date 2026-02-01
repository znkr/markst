package analyzer

import (
	"fmt"
	"strings"

	"znkr.io/writst/model/ir"
	"znkr.io/writst/syntax"
)

func Analyze(n syntax.RootNode) (ir.ContentExpr, error) {
	a := &analyzer{}
	exprs := a.analyzeContent(n.Node)
	var err error
	if len(a.errors) > 0 {
		err = ErrorList(a.errors)
	}
	return exprs, err
}

type analyzer struct {
	errors []*Error
}

func (a *analyzer) error(span syntax.Span, val *syntax.ErrorValue) {
	a.errors = append(a.errors, &Error{span, val})
}

type ErrorList []*Error

func (l ErrorList) Error() string {
	var sb strings.Builder
	sb.WriteString("analysis failed:\n")
	for _, err := range l {
		fmt.Fprintf(&sb, "- %s\n", err.Error())
	}
	return sb.String()
}

type Error struct {
	span syntax.Span
	val  *syntax.ErrorValue
}

func (err *Error) Span() syntax.Span {
	return err.span
}

func (err *Error) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %s", err.val.Message, err.val.Literal)
	for _, hint := range err.val.Hints {
		fmt.Fprintf(&sb, "\n  %s", hint)
	}
	return sb.String()
}
