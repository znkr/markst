package analyzer

import (
	"znkr.io/writst/model/ir"
	"znkr.io/writst/syntax"
)

func Analyze(n syntax.Node) ir.Content {
	a := &analyzer{}
	return a.analyzeContent(n)
}

type analyzer struct {
}
