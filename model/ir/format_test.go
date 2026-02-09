package ir

import (
	"testing"
	"unique"

	"znkr.io/diff/textdiff"
	"znkr.io/writst/syntax"
)

// Helper functions for creating IR nodes
func id(s string) *Ident                    { return &Ident{name: unique.Make(s)} }
func text(s string) *Const                  { return &Const{value: &Text{Value: s}} }
func str(s string) *Const                   { return &Const{value: String(s)} }
func num(n int) *Const                      { return &Const{value: Int(n)} }
func param(s string) *PositionalParam       { return &PositionalParam{ident: id(s)} }
func destruct(s string) *DestructIdent      { return &DestructIdent{ident: id(s)} }
func narg(name string, expr Expr) *NamedArg { return &NamedArg{name: unique.Make(name), expr: expr} }

func ce(exprs ...Expr) *ContentExpr { return &ContentExpr{exprs: exprs} }

func TestFormat(t *testing.T) {
	tests := []struct {
		name    string
		content *ContentExpr
		want    string
	}{
		{
			name:    "empty_content",
			content: &ContentExpr{},
			want:    "",
		},
		{
			name:    "text",
			content: ce(text("hello")),
			want:    "#text(\"hello\")\n",
		},
		{
			name:    "heading",
			content: ce(&HeadingExpr{level: 1, body: ce(text("Title"))}),
			want:    "#heading(level: 1)[#text(\"Title\")]\n",
		},
		{
			name:    "strong",
			content: ce(&StrongExpr{body: ce(text("bold"))}),
			want:    "#strong[#text(\"bold\")]\n",
		},
		{
			name:    "emph",
			content: ce(&EmphExpr{body: ce(text("italic"))}),
			want:    "#emph[#text(\"italic\")]\n",
		},
		{
			name:    "raw_block",
			content: ce(&Const{value: &Raw{Block: true, Lang: "go", Lines: []string{"package main", "func main() {}"}}}),
			want: `#raw(
  block: true,
  lang: "go",
  "package main",
  "func main() {}",
)
`,
		},
		{
			name:    "raw_inline",
			content: ce(&Const{value: &Raw{Block: false, Lines: []string{"code"}}}),
			want:    "#raw(\"code\")\n",
		},
		{
			name:    "linebreak",
			content: ce(&Const{value: &Linebreak{}}),
			want:    "#linebreak()\n",
		},
		{
			name:    "parbreak",
			content: ce(&Const{value: &Parbreak{}}),
			want:    "#parbreak()\n",
		},
		{
			name:    "link",
			content: ce(&LinkExpr{dest: "https://example.com", body: ce(text("example"))}),
			want:    "#link(dest: \"https://example.com\")[#text(\"example\")]\n",
		},
		{
			name: "list",
			content: ce(&Const{value: &List{Items: []ListItem{
				{Body: &Text{Value: "item 1"}},
				{Body: &Text{Value: "item 2"}},
			}}}),
			want: "#list(list.item[#text(\"item 1\")], list.item[#text(\"item 2\")])\n",
		},
		{
			name: "enum",
			content: ce(&Const{value: &Enum{Items: []EnumItem{
				{Number: 1, Body: &Text{Value: "first"}},
				{Number: 2, Body: &Text{Value: "second"}},
			}}}),
			want: "#enum(enum.item(1)[#text(\"first\")], enum.item(2)[#text(\"second\")])\n",
		},
		{
			name: "terms",
			content: ce(&Const{value: &Terms{Items: []TermItem{
				{Term: &Text{Value: "key"}, Description: &Text{Value: "value"}},
			}}}),
			want: "#terms(terms.item[#text(\"key\")][#text(\"value\")])\n",
		},
		{
			name: "nested_content",
			content: ce(
				&HeadingExpr{level: 1, body: ce(&StrongExpr{body: ce(text("Important"))})},
				text("Some text with "),
				&EmphExpr{body: ce(text("emphasis"))},
			),
			want: `#heading(level: 1)[#strong[#text("Important")]]
#text("Some text with ")
#emph[#text("emphasis")]
`,
		},
		{
			name: "multi_item_content_block",
			content: ce(&Const{value: &List{Items: []ListItem{
				{Body: Contents{
					&Text{Value: "first "},
					&Strong{Body: &Text{Value: "bold"}},
					&Text{Value: " last"},
				}},
			}}}),
			want: `#list(list.item[
  #text("first ")
  #strong[#text("bold")]
  #text(" last")
])
`,
		},
		{
			name: "heading_with_multi_item_body",
			content: ce(&HeadingExpr{level: 2, body: ce(
				text("Hello "),
				&EmphExpr{body: ce(text("world"))},
			)}),
			want: `#heading(level: 2)[
  #text("Hello ")
  #emph[#text("world")]
]
`,
		},
		{
			name: "deeply_nested_markup",
			content: ce(&Const{value: &List{Items: []ListItem{
				{Body: Contents{
					&Text{Value: "outer"},
					&List{Items: []ListItem{
						{Body: Contents{
							&Text{Value: "middle"},
							&List{Items: []ListItem{{Body: &Text{Value: "inner"}}}},
						}},
					}},
				}},
			}}}),
			want: `#list(list.item[
  #text("outer")
  #list(list.item[
    #text("middle")
    #list(list.item[#text("inner")])
  ])
])
`,
		},
		{
			name: "deeply_nested_code_expressions",
			content: ce(&FuncCall{
				callee: id("outer"),
				args: []Arg{&ExprArg{expr: &FuncCall{
					callee: id("middle"),
					args:   []Arg{&ExprArg{expr: &FuncCall{callee: id("inner"), args: []Arg{&ExprArg{expr: num(42)}}}}},
				}}},
			}),
			want: "#outer(middle(inner(42)))\n",
		},
		{
			name: "nested_conditionals",
			content: ce(&Conditional{
				condition: &Const{value: Bool(true)},
				then: &CodeBlock{body: ce(&Conditional{
					condition: &Const{value: Bool(false)},
					then:      &CodeBlock{body: ce(num(1))},
					els:       &CodeBlock{body: ce(num(2))},
				})},
			}),
			want: "#if true { if false { 1 } else { 2 } }\n",
		},
		{
			name: "nested_arrays_and_dicts",
			content: ce(&ArrayExpr{elements: []Expr{
				&ArrayExpr{elements: []Expr{
					&DictExpr{entries: []DictItemExpr{
						{key: id("key"), value: &ArrayExpr{elements: []Expr{num(1), num(2)}}},
					}},
				}},
			}}),
			want: "#(((key: (1, 2)),),)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.content)
			if diff := textdiff.Unified(tt.want, got); diff != "" {
				t.Errorf("Format() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatCodeExpressions(t *testing.T) {
	tests := []struct {
		name    string
		content *ContentExpr
		want    string
	}{
		// Literals
		{
			name:    "none",
			content: ce(&Const{value: None{}}),
			want:    "#none\n",
		},
		{
			name:    "auto",
			content: ce(&Const{value: Auto{}}),
			want:    "#auto\n",
		},
		{
			name:    "bool_true",
			content: ce(&Const{value: Bool(true)}),
			want:    "#true\n",
		},
		{
			name:    "bool_false",
			content: ce(&Const{value: Bool(false)}),
			want:    "#false\n",
		},
		{
			name:    "int",
			content: ce(num(42)),
			want:    "#42\n",
		},
		{
			name:    "float",
			content: ce(&Const{value: Float(3.14)}),
			want:    "#3.14\n",
		},
		{
			name:    "numeric",
			content: ce(&Const{value: Numeric{Value: 12, Unit: UnitPt}}),
			want:    "#12pt\n",
		},
		{
			name:    "string",
			content: ce(str("hello")),
			want:    "#\"hello\"\n",
		},
		{
			name:    "string_with_escapes",
			content: ce(str("line1\nline2\ttab")),
			want:    "#\"line1\\nline2\\ttab\"\n",
		},
		{
			name:    "ident",
			content: ce(id("foo")),
			want:    "#foo\n",
		},
		{
			name:    "underscore",
			content: ce(id("_")),
			want:    "#_\n",
		},

		// Code blocks
		{
			name:    "code_block_empty",
			content: ce(&CodeBlock{body: &ContentExpr{}}),
			want:    "#{ }\n",
		},
		{
			name:    "code_block_single",
			content: ce(&CodeBlock{body: ce(num(1))}),
			want:    "#{ 1 }\n",
		},
		{
			name:    "code_block_multiple",
			content: ce(&CodeBlock{body: ce(num(1), num(2), num(3))}),
			want:    "#{ 1; 2; 3 }\n",
		},

		// Content block
		{
			name:    "content_block",
			content: ce(&ContentBlock{body: ce(text("hello"))}),
			want:    "#[#text(\"hello\")]\n",
		},

		// Parenthesized
		{
			name:    "parenthesized",
			content: ce(&Parenthesized{body: num(1)}),
			want:    "#(1)\n",
		},

		// Arrays
		{
			name:    "array_empty",
			content: ce(&ArrayExpr{elements: nil}),
			want:    "#()\n",
		},
		{
			name:    "array_single",
			content: ce(&ArrayExpr{elements: []Expr{num(1)}}),
			want:    "#(1,)\n",
		},
		{
			name:    "array_multiple",
			content: ce(&ArrayExpr{elements: []Expr{num(1), num(2), num(3)}}),
			want:    "#(1, 2, 3)\n",
		},

		// Dictionaries
		{
			name:    "dict_empty",
			content: ce(&DictExpr{entries: nil}),
			want:    "#(:)\n",
		},
		{
			name:    "dict_named",
			content: ce(&DictExpr{entries: []DictItemExpr{{key: id("a"), value: num(1)}}}),
			want:    "#(a: 1)\n",
		},
		{
			name:    "dict_keyed",
			content: ce(&DictExpr{entries: []DictItemExpr{{key: str("key"), value: str("value")}}}),
			want:    "#(\"key\": \"value\")\n",
		},

		// Operators
		{
			name:    "unary_minus",
			content: ce(&Unary{op: syntax.Neg, operand: num(1)}),
			want:    "#-1\n",
		},
		{
			name:    "unary_not",
			content: ce(&Unary{op: syntax.Not, operand: &Const{value: Bool(true)}}),
			want:    "#not true\n",
		},
		{
			name:    "binary_add",
			content: ce(&Binary{left: num(1), op: syntax.Add, right: num(2)}),
			want:    "#1 + 2\n",
		},
		{
			name:    "binary_eq",
			content: ce(&Binary{left: id("x"), op: syntax.Eq, right: num(1)}),
			want:    "#x == 1\n",
		},
		{
			name:    "binary_assign",
			content: ce(&Binary{left: id("x"), op: syntax.Assign, right: num(1)}),
			want:    "#x = 1\n",
		},

		// Field access
		{
			name:    "field_access",
			content: ce(&FieldAccess{target: id("foo"), field: "bar"}),
			want:    "#foo.bar\n",
		},
		{
			name:    "field_access_nested",
			content: ce(&FieldAccess{target: &FieldAccess{target: id("a"), field: "b"}, field: "c"}),
			want:    "#a.b.c\n",
		},

		// Function calls
		{
			name:    "func_call_no_args",
			content: ce(&FuncCall{callee: id("foo")}),
			want:    "#foo()\n",
		},
		{
			name:    "func_call_with_args",
			content: ce(&FuncCall{callee: id("foo"), args: []Arg{&ExprArg{expr: num(1)}, &ExprArg{expr: num(2)}}}),
			want:    "#foo(1, 2)\n",
		},
		{
			name:    "func_call_with_named_args",
			content: ce(&FuncCall{callee: id("foo"), args: []Arg{narg("a", num(1))}}),
			want:    "#foo(a: 1)\n",
		},
		{
			name:    "func_call_with_content",
			content: ce(&FuncCall{callee: id("foo"), content: []*ContentExpr{ce(text("hello"))}}),
			want:    "#foo()[#text(\"hello\")]\n",
		},
		{
			name:    "func_call_with_args_and_content",
			content: ce(&FuncCall{callee: id("foo"), args: []Arg{&ExprArg{expr: num(1)}}, content: []*ContentExpr{ce(text("hello"))}}),
			want:    "#foo(1)[#text(\"hello\")]\n",
		},
		{
			name:    "method_call",
			content: ce(&FuncCall{callee: &FieldAccess{target: id("foo"), field: "bar"}}),
			want:    "#foo.bar()\n",
		},

		// Closures
		{
			name:    "closure_single_param",
			content: ce(&Closure{params: []Param{param("x")}, body: &Binary{left: id("x"), op: syntax.Add, right: num(1)}}),
			want:    "#x => x + 1\n",
		},
		{
			name:    "closure_multi_param",
			content: ce(&Closure{params: []Param{param("x"), param("y")}, body: &Binary{left: id("x"), op: syntax.Add, right: id("y")}}),
			want:    "#(x, y) => x + y\n",
		},
		{
			name:    "closure_spread_param",
			content: ce(&Closure{params: []Param{&SpreadParam{ident: id("args")}}, body: id("args")}),
			want:    "#(..args) => args\n",
		},
		{
			name:    "closure_named",
			content: ce(&Closure{name: id("add"), params: []Param{param("x"), param("y")}, body: &Binary{left: id("x"), op: syntax.Add, right: id("y")}}),
			want:    "#add(x, y) = x + y\n",
		},

		// Let bindings
		{
			name:    "let_simple",
			content: ce(&LetBinding{pattern: []DestructPattern{destruct("x")}, value: num(1)}),
			want:    "#let x = 1\n",
		},
		{
			name:    "let_pattern",
			content: ce(&LetBinding{pattern: []DestructPattern{destruct("a"), destruct("b")}, value: id("pair")}),
			want:    "#let (a, b) = pair\n",
		},
		{
			name:    "let_function",
			content: ce(&LetBinding{pattern: []DestructPattern{destruct("add")}, value: &Closure{name: id("add"), params: []Param{param("x"), param("y")}, body: &Binary{left: id("x"), op: syntax.Add, right: id("y")}}}),
			want:    "#let add(x, y) = x + y\n",
		},

		// Set rules
		{
			name:    "set_rule",
			content: ce(&SetRule{target: id("text"), args: []Arg{narg("size", &Const{value: Numeric{Value: 12, Unit: UnitPt}})}}),
			want:    "#set text(size: 12pt)\n",
		},
		{
			name:    "set_rule_with_condition",
			content: ce(&SetRule{target: id("text"), args: []Arg{&ExprArg{expr: id("red")}}, condition: id("enabled")}),
			want:    "#set text(red) if enabled\n",
		},

		// Show rules
		{
			name:    "show_rule_no_selector",
			content: ce(&ShowRule{transform: id("emph")}),
			want:    "#show: emph\n",
		},
		{
			name:    "show_rule_with_selector",
			content: ce(&ShowRule{selector: id("heading"), transform: &Closure{params: []Param{param("it")}, body: &FuncCall{callee: id("emph"), args: []Arg{&ExprArg{expr: &FieldAccess{target: id("it"), field: "body"}}}}}}),
			want:    "#show heading: it => emph(it.body)\n",
		},

		// Conditionals
		{
			name:    "if_only",
			content: ce(&Conditional{condition: id("x"), then: &CodeBlock{body: ce(id("y"))}}),
			want:    "#if x { y }\n",
		},
		{
			name:    "if_else",
			content: ce(&Conditional{condition: id("x"), then: &CodeBlock{body: ce(id("y"))}, els: &CodeBlock{body: ce(id("z"))}}),
			want:    "#if x { y } else { z }\n",
		},
		{
			name: "if_else_if",
			content: ce(&Conditional{
				condition: id("a"),
				then:      &CodeBlock{body: ce(num(1))},
				els:       &Conditional{condition: id("b"), then: &CodeBlock{body: ce(num(2))}, els: &CodeBlock{body: ce(num(3))}},
			}),
			want: "#if a { 1 } else if b { 2 } else { 3 }\n",
		},

		// Loops
		{
			name:    "while_loop",
			content: ce(&WhileLoop{condition: id("x"), body: &CodeBlock{body: ce(id("y"))}}),
			want:    "#while x { y }\n",
		},
		{
			name:    "for_loop",
			content: ce(&ForLoop{pattern: []DestructPattern{destruct("x")}, iterable: id("items"), body: &CodeBlock{body: ce(id("x"))}}),
			want:    "#for x in items { x }\n",
		},
		{
			name:    "for_loop_pattern",
			content: ce(&ForLoop{pattern: []DestructPattern{destruct("k"), destruct("v")}, iterable: id("dict"), body: &CodeBlock{body: ce(id("k"))}}),
			want:    "#for (k, v) in dict { k }\n",
		},

		// Control flow
		{
			name:    "break",
			content: ce(&LoopBreak{}),
			want:    "#break\n",
		},
		{
			name:    "continue",
			content: ce(&LoopContinue{}),
			want:    "#continue\n",
		},
		{
			name:    "return_with_value",
			content: ce(&FuncReturn{value: num(1)}),
			want:    "#return 1\n",
		},
		{
			name:    "return_no_value",
			content: ce(&FuncReturn{}),
			want:    "#return\n",
		},

		// Other
		{
			name:    "context",
			content: ce(&Contextual{body: &FieldAccess{target: id("text"), field: "lang"}}),
			want:    "#context text.lang\n",
		},
		{
			name:    "include",
			content: ce(&ModuleInclude{source: str("other.typ")}),
			want:    "#include \"other.typ\"\n",
		},
		{
			name:    "destruct_assign",
			content: ce(&DestructAssignment{pattern: []DestructPattern{destruct("a"), destruct("b")}, value: id("pair")}),
			want:    "#(a, b) = pair\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.content)
			if diff := textdiff.Unified(tt.want, got); diff != "" {
				t.Errorf("Format() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
