package analyzer_test

import (
	"fmt"
	"testing"

	"znkr.io/markst/syntax/analyzer"
	"znkr.io/markst/syntax/parser"
)

// TestEmbeddedCodeContexts guards a structural hazard: parseEmbeddedCodeExpr
// splices the leading `#` and the optional statement-terminating `;` into the
// tree as flat siblings of the code expression, so no node stands for "an
// embedded code expression". Every context that admits `#code` has to step
// over both markers, and a context that doesn't takes an analyzer internal
// panic on perfectly valid input rather than producing a diagnostic. The
// cursor skips them centrally (see `skipped` in util.go); this sweeps the
// contexts to keep it that way.
//
// Output isn't checked here — the interesting cases have golden tests in
// testdata/math.test. This only asserts that nothing panics.
func TestEmbeddedCodeContexts(t *testing.T) {
	contexts := []string{
		"%s", "#[ %s ]", "#{ %s }", "#let f() = { %s }",
		"$ %s $", "$ (%s) $", "$ #(%s) $",
		"$ a^%s $", "$ a_%s $", "$ %s^2 $", "$ a^%s_b $", "$ x^(%s) $", "$ %s' $",
		"$ %s/b $", "$ a/%s $", "$ (%s)/(b) $",
		"$ root(%s, b) $", "$ sqrt(%s) $",
		"$ f(%s, b) $", "$ vec(%s) $", "$ vec(a: %s) $", "$ vec(..%s) $",
		"$ mat(%s; b) $", "$ mat(a; %s) $",
		"$ lr(%s) $", "$ [|%s|] $",
	}
	codes := []string{
		"#a", "#1", "#none", "#\"s\"", "#a.b", "#a(1)", "#f(..a)",
		"#(1+2)", "#(a: 1)", "#(1,2)", "#[x]", "#{1}",
		"#let x = 1", "#if true [x]", "#",
		// The same, statement-terminated.
		"#a;", "#1;", "#let x = 1;", "#a(1);",
	}
	for _, context := range contexts {
		for _, code := range codes {
			in := fmt.Sprintf(context, code)
			t.Run(in, func(t *testing.T) {
				analyzer.Analyze(parser.Parse([]byte(in)))
			})
		}
	}
}
