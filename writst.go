package writst

import (
	"errors"

	"znkr.io/writst/eval"
	"znkr.io/writst/syntax/analyzer"
	"znkr.io/writst/syntax/parser"
	"znkr.io/writst/value"
)

func Render(src string) (*value.Document, error) {
	root := parser.Parse(src)
	mod := analyzer.Analyze(root)
	doc, warnings, errs := eval.Eval(mod)
	var outErrs []error
	for _, w := range warnings {
		outErrs = append(outErrs, w)
	}
	for _, e := range errs {
		outErrs = append(outErrs, e)
	}
	return doc, errors.Join(outErrs...)
}
