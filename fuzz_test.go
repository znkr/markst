package markst_test

import (
	"path/filepath"
	"strings"
	"testing"

	"znkr.io/markst/internal/testfile"
	"znkr.io/markst/syntax/analyzer"
	"znkr.io/markst/syntax/parser"
)

// seedGlobs are the golden test files the fuzz corpus is seeded from. They
// cover every stage that feeds the analyzer, so the fuzzer starts from real
// documents rather than from random bytes.
var seedGlobs = []string{
	"testdata/*/*.test",
	"syntax/parser/testdata/*.test",
	"syntax/analyzer/testdata/*.test",
	"builtin/testdata/*.test",
}

// crashers are inputs that once panicked the parser or the analyzer. They are
// seeded explicitly so a plain `go test` re-runs them as regressions without a
// checked-in corpus directory.
var crashers = []string{
	"$0^ ^0$",              // stale wrap marker: error node escaped the MathAttach
	"$a_ _b$",              // same, subscript
	"$x^  ^y$",             // same, multiple spaces
	"$f(a: )$",             // same, in a named math argument
	"#(1 + .b)",            // same, in code mode
	"#(:0)",                // `(:` downgraded to a parenthesized expression
	`#"\'"`,                // escape accepted by the scanner but not by unquote
	`#"\u{}"`,              // unicode escape validated for shape but not for value
	`#"\u{zz}"`,            //
	`#"\u{110000}"`,        //
	"*a",                   // unclosed delimiter replaced the opening `*`
	"_a",                   //
	"$#0[*]$",              // same, reached through math
	"$#(pi = 1)$",          // assignment to a constant (math-module) binding
	"#(emph = 1)",          // assignment to a constant (universe) binding
	"#set .A",              // stale wrap marker in a set rule's target
	"#for A!*0[]{",         // an error belonging to the `for` header pulled into its body
	"#{ show heading it }", // show rule with no colon
	"#{ set 123(1) }",      // set rule with no target
	"#(set!if",             // set rule with no argument list
	"#show *",              // selector marker invalidated by parseCodeExpr's own wrap
	"#0e0cm",               // unit split from the front cut the exponent in half
	"+\r\rb",               // a lone CR did not end a line, so a parbreak landed in the item
	"+\r\r00b",             //

	"0.\n\n  0",   // a parbreak before an item's body landed beside it, not in it
	"- \n\n  b",   //
	"/ t:\n\n  b", //
	"/ \n\n  :",   // a parbreak between a term item's body and its colon

	// Not a panic but a fuzz timeout: Inner.Span recursed into both the first
	// and the last child, so a deeply nested tree cost 2^depth to lower.
	"#{{{{{{{{{{{{{{{{{{{{{{{{{{{{{0\"0",

	// Likewise: constant folding asked for a petabyte-sized string.
	`#(70007000*07000*"00"`,
}

// notLoweredPrefix is the panic message the analyzer uses for a construct it
// does not lower yet.
const notLoweredPrefix = "ssa lowering not yet implemented for: "

// notLowered names the syntax kinds behind that panic that are deliberate gaps
// rather than bugs — modules and imports, which the golden tests mark with
// `skip:` for the same reason. The target skips those and fails on every other
// panic, including a "not yet implemented" naming a kind that can never
// legitimately appear as an expression (that is parser/analyzer drift, which is
// how `#(:0)` used to crash). Entries come off this list as features land.
var notLowered = map[string]bool{
	"KindModuleImport":      true,
	"KindImportItems":       true,
	"KindImportItemPath":    true,
	"KindRenamedImportItem": true,
}

// FuzzAnalyze runs the parser and the analyzer over arbitrary input. Both stages
// are total by contract — syntax errors travel through the tree as
// [syntax.Error] nodes and through the IR as [expr.Error] instructions — so any
// panic is a bug and fails the test. The shape of the tree itself is
// [parser.FuzzParse]'s job.
func FuzzAnalyze(f *testing.F) {
	for _, glob := range seedGlobs {
		files, err := filepath.Glob(glob)
		if err != nil {
			f.Fatal(err)
		}
		for _, file := range files {
			for _, tc := range testfile.Read(f, file) {
				if tc.Skip != "" {
					// Skipped cases exercise constructs that are known not to
					// work yet (imports, modules); seeding them would only feed
					// the fuzzer inputs it must ignore.
					continue
				}
				f.Add(tc.Input)
			}
		}
	}
	for _, src := range crashers {
		f.Add(src)
	}

	f.Fuzz(func(t *testing.T, src string) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if s, ok := r.(string); ok {
				if kind, found := strings.CutPrefix(s, notLoweredPrefix); found && notLowered[kind] {
					t.Skipf("known analyzer gap: %s", s)
				}
			}
			panic(r)
		}()
		root := parser.Parse([]byte(src))
		analyzer.Analyze(root)
		// The approximations-off path lowers strictly more, so it reaches code
		// the default path skips. TestApproximationsPreserveOutput is what
		// checks the two agree; here it is only the panic-freedom that matters.
		analyzer.Analyze(root, analyzer.WithoutApproximations())
	})
}
