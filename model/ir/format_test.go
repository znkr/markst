package ir

import (
	"testing"
	"unique"

	"znkr.io/diff/textdiff"
	"znkr.io/writst/syntax"
)

// Helper functions for creating IR nodes
func id(s string) *Ident                     { return &Ident{Name: unique.Make(s)} }
func text(s string) *Const                   { return &Const{Value: &Text{Value: s}} }
func str(s string) *Const                    { return &Const{Value: String(s)} }
func num(n int) *Const                       { return &Const{Value: Int(n)} }
func param(s string) *PositionalParam        { return &PositionalParam{id(s)} }
func destruct(s string) *DestructIdent       { return &DestructIdent{id(s)} }
func narg(name string, expr Expr) *NamedArg  { return &NamedArg{Name: unique.Make(name), Expr: expr} }

func TestFormat(t *testing.T) {
	tests := []struct {
		name    string
		content ContentExpr
		want    string
	}{
		{
			name:    "empty_content",
			content: ContentExpr{},
			want:    "",
		},
		{
			name:    "text",
			content: ContentExpr{text("hello")},
			want:    "#text(\"hello\")\n",
		},
		{
			name:    "heading",
			content: ContentExpr{&HeadingExpr{Level: 1, Body: ContentExpr{text("Title")}}},
			want:    "#heading(level: 1)[#text(\"Title\")]\n",
		},
		{
			name:    "strong",
			content: ContentExpr{&StrongExpr{Body: ContentExpr{text("bold")}}},
			want:    "#strong[#text(\"bold\")]\n",
		},
		{
			name:    "emph",
			content: ContentExpr{&EmphExpr{Body: ContentExpr{text("italic")}}},
			want:    "#emph[#text(\"italic\")]\n",
		},
		{
			name:    "raw_block",
			content: ContentExpr{&Const{Value: &Raw{Block: true, Lang: "go", Lines: []string{"package main", "func main() {}"}}}},
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
			content: ContentExpr{&Const{Value: &Raw{Block: false, Lines: []string{"code"}}}},
			want:    "#raw(\"code\")\n",
		},
		{
			name:    "linebreak",
			content: ContentExpr{&Const{Value: &Linebreak{}}},
			want:    "#linebreak()\n",
		},
		{
			name:    "parbreak",
			content: ContentExpr{&Const{Value: &Parbreak{}}},
			want:    "#parbreak()\n",
		},
		{
			name:    "link",
			content: ContentExpr{&LinkExpr{Dest: "https://example.com", Body: ContentExpr{text("example")}}},
			want:    "#link(dest: \"https://example.com\")[#text(\"example\")]\n",
		},
		{
			name: "list",
			content: ContentExpr{&Const{Value: &List{Items: []ListItem{
				{Body: &Text{Value: "item 1"}},
				{Body: &Text{Value: "item 2"}},
			}}}},
			want: "#list(list.item[#text(\"item 1\")], list.item[#text(\"item 2\")])\n",
		},
		{
			name: "enum",
			content: ContentExpr{&Const{Value: &Enum{Items: []EnumItem{
				{Number: 1, Body: &Text{Value: "first"}},
				{Number: 2, Body: &Text{Value: "second"}},
			}}}},
			want: "#enum(enum.item(1)[#text(\"first\")], enum.item(2)[#text(\"second\")])\n",
		},
		{
			name: "terms",
			content: ContentExpr{&Const{Value: &Terms{Items: []TermItem{
				{Term: &Text{Value: "key"}, Description: &Text{Value: "value"}},
			}}}},
			want: "#terms(terms.item[#text(\"key\")][#text(\"value\")])\n",
		},
		{
			name: "nested_content",
			content: ContentExpr{
				&HeadingExpr{Level: 1, Body: ContentExpr{&StrongExpr{Body: ContentExpr{text("Important")}}}},
				text("Some text with "),
				&EmphExpr{Body: ContentExpr{text("emphasis")}},
			},
			want: `#heading(level: 1)[#strong[#text("Important")]]
#text("Some text with ")
#emph[#text("emphasis")]
`,
		},
		{
			name: "multi_item_content_block",
			content: ContentExpr{&Const{Value: &List{Items: []ListItem{
				{Body: Contents{
					&Text{Value: "first "},
					&Strong{Body: &Text{Value: "bold"}},
					&Text{Value: " last"},
				}},
			}}}},
			want: `#list(list.item[
  #text("first ")
  #strong[#text("bold")]
  #text(" last")
])
`,
		},
		{
			name: "heading_with_multi_item_body",
			content: ContentExpr{&HeadingExpr{Level: 2, Body: ContentExpr{
				text("Hello "),
				&EmphExpr{Body: ContentExpr{text("world")}},
			}}},
			want: `#heading(level: 2)[
  #text("Hello ")
  #emph[#text("world")]
]
`,
		},
		{
			name: "deeply_nested_markup",
			content: ContentExpr{&Const{Value: &List{Items: []ListItem{
				{Body: Contents{
					&Text{Value: "outer"},
					&List{Items: []ListItem{
						{Body: Contents{
							&Text{Value: "middle"},
							&List{Items: []ListItem{{Body: &Text{Value: "inner"}}}},
						}},
					}},
				}},
			}}}},
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
			content: ContentExpr{&FuncCall{
				Callee: id("outer"),
				Args: []Arg{&ExprArg{&FuncCall{
					Callee: id("middle"),
					Args:   []Arg{&ExprArg{&FuncCall{Callee: id("inner"), Args: []Arg{&ExprArg{num(42)}}}}},
				}}},
			}},
			want: "#outer(middle(inner(42)))\n",
		},
		{
			name: "nested_conditionals",
			content: ContentExpr{&Conditional{
				Condition: &Const{Value: Bool(true)},
				Then: &CodeBlock{Body: []Expr{&Conditional{
					Condition: &Const{Value: Bool(false)},
					Then:      &CodeBlock{Body: []Expr{num(1)}},
					Else:      &CodeBlock{Body: []Expr{num(2)}},
				}}},
			}},
			want: "#if true { if false { 1 } else { 2 } }\n",
		},
		{
			name: "nested_arrays_and_dicts",
			content: ContentExpr{&ArrayExpr{Elements: []Expr{
				&ArrayExpr{Elements: []Expr{
					&DictExpr{Entries: []DictItemExpr{
						{Key: id("key"), Value: &ArrayExpr{Elements: []Expr{num(1), num(2)}}},
					}},
				}},
			}}},
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
		content ContentExpr
		want    string
	}{
		// Literals
		{
			name:    "none",
			content: ContentExpr{&Const{Value: None{}}},
			want:    "#none\n",
		},
		{
			name:    "auto",
			content: ContentExpr{&Const{Value: Auto{}}},
			want:    "#auto\n",
		},
		{
			name:    "bool_true",
			content: ContentExpr{&Const{Value: Bool(true)}},
			want:    "#true\n",
		},
		{
			name:    "bool_false",
			content: ContentExpr{&Const{Value: Bool(false)}},
			want:    "#false\n",
		},
		{
			name:    "int",
			content: ContentExpr{num(42)},
			want:    "#42\n",
		},
		{
			name:    "float",
			content: ContentExpr{&Const{Value: Float(3.14)}},
			want:    "#3.14\n",
		},
		{
			name:    "numeric",
			content: ContentExpr{&Const{Value: Numeric{Value: 12, Unit: UnitPt}}},
			want:    "#12pt\n",
		},
		{
			name:    "string",
			content: ContentExpr{str("hello")},
			want:    "#\"hello\"\n",
		},
		{
			name:    "string_with_escapes",
			content: ContentExpr{str("line1\nline2\ttab")},
			want:    "#\"line1\\nline2\\ttab\"\n",
		},
		{
			name:    "ident",
			content: ContentExpr{id("foo")},
			want:    "#foo\n",
		},
		{
			name:    "underscore",
			content: ContentExpr{id("_")},
			want:    "#_\n",
		},

		// Code blocks
		{
			name:    "code_block_empty",
			content: ContentExpr{&CodeBlock{Body: nil}},
			want:    "#{ }\n",
		},
		{
			name:    "code_block_single",
			content: ContentExpr{&CodeBlock{Body: []Expr{num(1)}}},
			want:    "#{ 1 }\n",
		},
		{
			name:    "code_block_multiple",
			content: ContentExpr{&CodeBlock{Body: []Expr{num(1), num(2), num(3)}}},
			want:    "#{ 1; 2; 3 }\n",
		},

		// Content block
		{
			name:    "content_block",
			content: ContentExpr{&ContentBlock{Body: ContentExpr{text("hello")}}},
			want:    "#[#text(\"hello\")]\n",
		},

		// Parenthesized
		{
			name:    "parenthesized",
			content: ContentExpr{&Parenthesized{Body: num(1)}},
			want:    "#(1)\n",
		},

		// Arrays
		{
			name:    "array_empty",
			content: ContentExpr{&ArrayExpr{Elements: nil}},
			want:    "#()\n",
		},
		{
			name:    "array_single",
			content: ContentExpr{&ArrayExpr{Elements: []Expr{num(1)}}},
			want:    "#(1,)\n",
		},
		{
			name:    "array_multiple",
			content: ContentExpr{&ArrayExpr{Elements: []Expr{num(1), num(2), num(3)}}},
			want:    "#(1, 2, 3)\n",
		},

		// Dictionaries
		{
			name:    "dict_empty",
			content: ContentExpr{&DictExpr{Entries: nil}},
			want:    "#(:)\n",
		},
		{
			name:    "dict_named",
			content: ContentExpr{&DictExpr{Entries: []DictItemExpr{{Key: id("a"), Value: num(1)}}}},
			want:    "#(a: 1)\n",
		},
		{
			name:    "dict_keyed",
			content: ContentExpr{&DictExpr{Entries: []DictItemExpr{{Key: str("key"), Value: str("value")}}}},
			want:    "#(\"key\": \"value\")\n",
		},

		// Operators
		{
			name:    "unary_minus",
			content: ContentExpr{&Unary{Op: syntax.Neg, Operand: num(1)}},
			want:    "#-1\n",
		},
		{
			name:    "unary_not",
			content: ContentExpr{&Unary{Op: syntax.Not, Operand: &Const{Value: Bool(true)}}},
			want:    "#not true\n",
		},
		{
			name:    "binary_add",
			content: ContentExpr{&Binary{Left: num(1), Op: syntax.Add, Right: num(2)}},
			want:    "#1 + 2\n",
		},
		{
			name:    "binary_eq",
			content: ContentExpr{&Binary{Left: id("x"), Op: syntax.Eq, Right: num(1)}},
			want:    "#x == 1\n",
		},
		{
			name:    "binary_assign",
			content: ContentExpr{&Binary{Left: id("x"), Op: syntax.Assign, Right: num(1)}},
			want:    "#x = 1\n",
		},

		// Field access
		{
			name:    "field_access",
			content: ContentExpr{&FieldAccess{Target: id("foo"), Field: "bar"}},
			want:    "#foo.bar\n",
		},
		{
			name:    "field_access_nested",
			content: ContentExpr{&FieldAccess{Target: &FieldAccess{Target: id("a"), Field: "b"}, Field: "c"}},
			want:    "#a.b.c\n",
		},

		// Function calls
		{
			name:    "func_call_no_args",
			content: ContentExpr{&FuncCall{Callee: id("foo")}},
			want:    "#foo()\n",
		},
		{
			name:    "func_call_with_args",
			content: ContentExpr{&FuncCall{Callee: id("foo"), Args: []Arg{&ExprArg{num(1)}, &ExprArg{num(2)}}}},
			want:    "#foo(1, 2)\n",
		},
		{
			name:    "func_call_with_named_args",
			content: ContentExpr{&FuncCall{Callee: id("foo"), Args: []Arg{narg("a", num(1))}}},
			want:    "#foo(a: 1)\n",
		},
		{
			name:    "func_call_with_content",
			content: ContentExpr{&FuncCall{Callee: id("foo"), Content: []ContentExpr{{text("hello")}}}},
			want:    "#foo()[#text(\"hello\")]\n",
		},
		{
			name:    "func_call_with_args_and_content",
			content: ContentExpr{&FuncCall{Callee: id("foo"), Args: []Arg{&ExprArg{num(1)}}, Content: []ContentExpr{{text("hello")}}}},
			want:    "#foo(1)[#text(\"hello\")]\n",
		},
		{
			name:    "method_call",
			content: ContentExpr{&FuncCall{Callee: &FieldAccess{Target: id("foo"), Field: "bar"}}},
			want:    "#foo.bar()\n",
		},

		// Closures
		{
			name:    "closure_single_param",
			content: ContentExpr{&Closure{Params: []Param{param("x")}, Body: &Binary{Left: id("x"), Op: syntax.Add, Right: num(1)}}},
			want:    "#x => x + 1\n",
		},
		{
			name:    "closure_multi_param",
			content: ContentExpr{&Closure{Params: []Param{param("x"), param("y")}, Body: &Binary{Left: id("x"), Op: syntax.Add, Right: id("y")}}},
			want:    "#(x, y) => x + y\n",
		},
		{
			name:    "closure_spread_param",
			content: ContentExpr{&Closure{Params: []Param{&SpreadParam{id("args")}}, Body: id("args")}},
			want:    "#(..args) => args\n",
		},
		{
			name:    "closure_named",
			content: ContentExpr{&Closure{Name: id("add"), Params: []Param{param("x"), param("y")}, Body: &Binary{Left: id("x"), Op: syntax.Add, Right: id("y")}}},
			want:    "#add(x, y) = x + y\n",
		},

		// Let bindings
		{
			name:    "let_simple",
			content: ContentExpr{&LetBinding{Pattern: []DestructPattern{destruct("x")}, Value: num(1)}},
			want:    "#let x = 1\n",
		},
		{
			name:    "let_pattern",
			content: ContentExpr{&LetBinding{Pattern: []DestructPattern{destruct("a"), destruct("b")}, Value: id("pair")}},
			want:    "#let (a, b) = pair\n",
		},
		{
			name:    "let_function",
			content: ContentExpr{&LetBinding{Pattern: []DestructPattern{destruct("add")}, Value: &Closure{Name: id("add"), Params: []Param{param("x"), param("y")}, Body: &Binary{Left: id("x"), Op: syntax.Add, Right: id("y")}}}},
			want:    "#let add(x, y) = x + y\n",
		},

		// Set rules
		{
			name:    "set_rule",
			content: ContentExpr{&SetRule{Target: id("text"), Args: []Arg{narg("size", &Const{Value: Numeric{Value: 12, Unit: UnitPt}})}}},
			want:    "#set text(size: 12pt)\n",
		},
		{
			name:    "set_rule_with_condition",
			content: ContentExpr{&SetRule{Target: id("text"), Args: []Arg{&ExprArg{id("red")}}, Condition: id("enabled")}},
			want:    "#set text(red) if enabled\n",
		},

		// Show rules
		{
			name:    "show_rule_no_selector",
			content: ContentExpr{&ShowRule{Transform: id("emph")}},
			want:    "#show: emph\n",
		},
		{
			name:    "show_rule_with_selector",
			content: ContentExpr{&ShowRule{Selector: id("heading"), Transform: &Closure{Params: []Param{param("it")}, Body: &FuncCall{Callee: id("emph"), Args: []Arg{&ExprArg{&FieldAccess{Target: id("it"), Field: "body"}}}}}}},
			want:    "#show heading: it => emph(it.body)\n",
		},

		// Conditionals
		{
			name:    "if_only",
			content: ContentExpr{&Conditional{Condition: id("x"), Then: &CodeBlock{Body: []Expr{id("y")}}}},
			want:    "#if x { y }\n",
		},
		{
			name:    "if_else",
			content: ContentExpr{&Conditional{Condition: id("x"), Then: &CodeBlock{Body: []Expr{id("y")}}, Else: &CodeBlock{Body: []Expr{id("z")}}}},
			want:    "#if x { y } else { z }\n",
		},
		{
			name: "if_else_if",
			content: ContentExpr{&Conditional{
				Condition: id("a"),
				Then:      &CodeBlock{Body: []Expr{num(1)}},
				Else:      &Conditional{Condition: id("b"), Then: &CodeBlock{Body: []Expr{num(2)}}, Else: &CodeBlock{Body: []Expr{num(3)}}},
			}},
			want: "#if a { 1 } else if b { 2 } else { 3 }\n",
		},

		// Loops
		{
			name:    "while_loop",
			content: ContentExpr{&WhileLoop{Condition: id("x"), Body: &CodeBlock{Body: []Expr{id("y")}}}},
			want:    "#while x { y }\n",
		},
		{
			name:    "for_loop",
			content: ContentExpr{&ForLoop{Pattern: []DestructPattern{destruct("x")}, Iterable: id("items"), Body: &CodeBlock{Body: []Expr{id("x")}}}},
			want:    "#for x in items { x }\n",
		},
		{
			name:    "for_loop_pattern",
			content: ContentExpr{&ForLoop{Pattern: []DestructPattern{destruct("k"), destruct("v")}, Iterable: id("dict"), Body: &CodeBlock{Body: []Expr{id("k")}}}},
			want:    "#for (k, v) in dict { k }\n",
		},

		// Control flow
		{
			name:    "break",
			content: ContentExpr{&LoopBreak{}},
			want:    "#break\n",
		},
		{
			name:    "continue",
			content: ContentExpr{&LoopContinue{}},
			want:    "#continue\n",
		},
		{
			name:    "return_with_value",
			content: ContentExpr{&FuncReturn{Value: num(1)}},
			want:    "#return 1\n",
		},
		{
			name:    "return_no_value",
			content: ContentExpr{&FuncReturn{}},
			want:    "#return\n",
		},

		// Other
		{
			name:    "context",
			content: ContentExpr{&Contextual{Body: &FieldAccess{Target: id("text"), Field: "lang"}}},
			want:    "#context text.lang\n",
		},
		{
			name:    "include",
			content: ContentExpr{&ModuleInclude{Source: str("other.typ")}},
			want:    "#include \"other.typ\"\n",
		},
		{
			name:    "destruct_assign",
			content: ContentExpr{&DestructAssignment{Pattern: []DestructPattern{destruct("a"), destruct("b")}, Value: id("pair")}},
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
