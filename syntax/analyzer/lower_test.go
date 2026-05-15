package analyzer

import (
	"strings"
	"testing"

	"znkr.io/writst/expr"
	"znkr.io/writst/syntax/parser"
)

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "int-literal",
			src:  "#1",
			want: `fn $top:
  b0:
    v0 = const 1
    return v0
`,
		},
		{
			name: "binary-arithmetic",
			src:  "#(1 + 2)",
			want: `fn $top:
  b0:
    v0 = const 1
    v1 = const 2
    v2 = v0 + v1
    return v2
`,
		},
		{
			name: "unary-negation",
			src:  "#(-3)",
			want: `fn $top:
  b0:
    v0 = const 3
    v1 = - v0
    return v1
`,
		},
		{
			name: "string-and-array",
			src:  `#("a", "b", "c")`,
			want: `fn $top:
  b0:
    v0 = const a
    v1 = const b
    v2 = const c
    v3 = make_array [v0, v1, v2]
    return v3
`,
		},
		{
			name: "dict-named",
			src:  `#(a: 1, b: 2)`,
			want: `fn $top:
  b0:
    v0 = const a
    v1 = const 1
    v2 = const b
    v3 = const 2
    v4 = make_dict (v0: v1, v2: v3)
    return v4
`,
		},
		{
			name: "builtin-ident",
			src:  `#range`,
			want: `fn $top:
  b0:
    v0 = const <function range>
    return v0
`,
		},
		{
			name: "field-access",
			src:  `#range.with`,
			want: `fn $top:
  b0:
    v0 = const <function range>
    v1 = field_read v0.with
    return v1
`,
		},
		{
			name: "let-then-use",
			src:  "#let x = 5\n#(x + 1)",
			want: `fn $top:
  b0:
    v0 = const 5
    v1 = const 1
    v2 = v0 + v1
    return v2
`,
		},
		{
			name: "let-without-init",
			src:  "#let x",
			want: `fn $top:
  b0:
    v0 = const none
    v1 = const none
    return v1
`,
		},
		{
			name: "call-positional",
			src:  "#range(1, 5)",
			want: `fn $top:
  b0:
    v0 = const <function range>
    v1 = const 1
    v2 = const 5
    v3 = call v0(v1, v2)
    return v3
`,
		},
		{
			name: "call-named",
			src:  "#range(1, 5, step: 2)",
			want: `fn $top:
  b0:
    v0 = const <function range>
    v1 = const 1
    v2 = const 5
    v3 = const 2
    v4 = call v0(v1, v2, step: v3)
    return v4
`,
		},
		{
			name: "destructure-tuple",
			src:  "#let (a, b) = (1, 2)\n#a",
			want: `fn $top:
  b0:
    v0 = const 1
    v1 = const 2
    v2 = make_array [v0, v1]
    length_check v2, want=2
    v4 = extract v2[0]
    v5 = extract v2[1]
    return v4
`,
		},
		{
			name: "code-block-shadow",
			src:  "#{ let x = 1; { let x = 2; x } }",
			want: `fn $top:
  b0:
    v0 = const 1
    v1 = const 2
    return v1
`,
		},
		{
			name: "reassign",
			src:  "#{ let x = 1; x = x + 1; x }",
			want: `fn $top:
  b0:
    v0 = const 1
    v1 = const 1
    v2 = v0 + v1
    v3 = const none
    v4 = code_join [v3, v2]
    return v4
`,
		},
		{
			name: "if-else-phi",
			src:  "#{ let x = if true { 1 } else { 2 }; x }",
			want: `fn $top:
  b0:
    v0 = const true
    branch v0, b2, b3
  b1:
    v3 = phi [b2 v1, b3 v2]
    return v3
  b2:
    v1 = const 1
    jump b1
  b3:
    v2 = const 2
    jump b1
`,
		},
		{
			name: "if-no-else",
			src:  "#if true { 1 }",
			want: `fn $top:
  b0:
    v0 = const true
    branch v0, b2, b3
  b1:
    v3 = phi [b2 v1, b3 v2]
    return v3
  b2:
    v1 = const 1
    jump b1
  b3:
    v2 = const none
    jump b1
`,
		},
		{
			name: "if-rebinds-outer",
			src:  "#{ let x = 0; if true { x = 1 }; x }",
			want: `fn $top:
  b0:
    v0 = const 0
    v1 = const true
    branch v1, b2, b3
  b1:
    v5 = phi [b2 v3, b3 v4]
    v6 = phi [b2 v2, b3 v0]
    v7 = code_join [v5, v6]
    return v7
  b2:
    v2 = const 1
    v3 = const none
    jump b1
  b3:
    v4 = const none
    jump b1
`,
		},
		{
			name: "while-loop",
			src:  "#while true { 1 }",
			want: `fn $top:
  b0:
    v0 = loop_acc_begin
    jump b1
  b1:
    v3 = phi [b0 v0, b2 v4]
    v1 = const true
    branch v1, b2, b3
  b2:
    v2 = const 1
    v4 = loop_acc_add v3, v2
    jump b1
  b3:
    v5 = loop_acc_result v3
    return v5
`,
		},
		{
			name: "for-loop",
			src:  "#for x in (1, 2, 3) { x }",
			want: `fn $top:
  b0:
    v0 = const 1
    v1 = const 2
    v2 = const 3
    v3 = make_array [v0, v1, v2]
    v4 = iter_open v3
    v5 = loop_acc_begin
    jump b1
  b1:
    v8 = phi [b0 v5, b2 v9]
    v6 = iter_has_next v4
    branch v6, b2, b3
  b2:
    v7 = iter_advance v4
    v9 = loop_acc_add v8, v7
    jump b1
  b3:
    v10 = loop_acc_result v8
    return v10
`,
		},
		{
			name: "return",
			src:  "#return 42",
			want: `fn $top:
  b0:
    v0 = const 42
    return v0
  b1:
    v1 = const none
    return v1
`,
		},
		{
			name: "closure-no-captures",
			src:  "#let f = (x) => x + 1\n#f(5)",
			want: `fn $top:
  b0:
    v0 = make_closure $fn_0
    v1 = const 5
    v2 = call v0(v1)
    return v2

fn $fn_0 name="f" params=[v0=x]:
  b0:
    v1 = const 1
    v2 = v0 + v1
    return v2
`,
		},
		{
			name: "closure-shorthand",
			src:  "#let f(x) = x * 2\n#f(3)",
			want: `fn $top:
  b0:
    v0 = make_closure $fn_0
    v1 = const 3
    v2 = call v0(v1)
    return v2

fn $fn_0 name="f" params=[v0=x]:
  b0:
    v1 = const 2
    v2 = v0 * v1
    return v2
`,
		},
		{
			name: "closure-with-capture",
			src:  "#let n = 10\n#let f = (x) => x + n\n#f(5)",
			want: `fn $top:
  b0:
    v0 = const 10
    v1 = make_closure $fn_0 captures=[v0]
    v2 = const 5
    v3 = call v1(v2)
    return v3

fn $fn_0 name="f" captures=[v1=n] params=[v0=x]:
  b0:
    v2 = v0 + v1
    return v2
`,
		},
		{
			name: "markup-strong",
			src:  "hello *bold* world",
			want: `fn $top:
  b0:
    v0 = const text "hello"
    v1 = const text "bold"
    v2 = strong v1
    v3 = const text "world"
    v4 = content_result [v0, v2, v3]
    return v4
`,
		},
		{
			name: "markup-heading",
			src:  "= Title",
			want: `fn $top:
  b0:
    v0 = const text "Title"
    v1 = heading level=1 v0
    return v1
`,
		},
		{
			name: "markup-list",
			src:  "- item one\n- item two",
			want: `fn $top:
  b0:
    v0 = const text "item one"
    v1 = list_item v0
    v2 = const text "item two"
    v3 = list_item v2
    v4 = content_result [v1, v3]
    return v4
`,
		},
		{
			name: "markup-ref",
			src:  "see @label",
			want: `fn $top:
  b0:
    v0 = const text "see"
    v1 = ref @label
    v2 = content_result [v0, v1]
    return v2
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := parser.Parse(tt.src)
			mod, err := Analyze(root)
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			got := expr.FormatModule(mod)
			if got != tt.want {
				t.Errorf("Analyze mismatch:\n--- got ---\n%s--- want ---\n%s", got, tt.want)
				// Show line-by-line diff
				gotLines := strings.Split(got, "\n")
				wantLines := strings.Split(tt.want, "\n")
				n := max(len(wantLines), len(gotLines))
				for i := range n {
					var gl, wl string
					if i < len(gotLines) {
						gl = gotLines[i]
					}
					if i < len(wantLines) {
						wl = wantLines[i]
					}
					if gl != wl {
						t.Logf("line %d: got %q, want %q", i+1, gl, wl)
					}
				}
			}
		})
	}
}
