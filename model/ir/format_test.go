package ir

import (
	"testing"

	"znkr.io/diff/textdiff"
	"znkr.io/writst/syntax"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name    string
		content Content
		want    string
	}{
		{
			name:    "empty_content",
			content: Content{},
			want:    "",
		},
		{
			name: "text",
			content: Content{
				&Text{Value: "hello"},
			},
			want: "#text(\"hello\")\n",
		},
		{
			name: "heading",
			content: Content{
				&Heading{Level: 1, Body: Content{&Text{Value: "Title"}}},
			},
			want: "#heading(level: 1)[#text(\"Title\")]\n",
		},
		{
			name: "strong",
			content: Content{
				&Strong{Body: Content{&Text{Value: "bold"}}},
			},
			want: "#strong[#text(\"bold\")]\n",
		},
		{
			name: "emph",
			content: Content{
				&Emph{Body: Content{&Text{Value: "italic"}}},
			},
			want: "#emph[#text(\"italic\")]\n",
		},
		{
			name: "raw_block",
			content: Content{
				&Raw{Block: true, Lang: "go", Lines: []string{"package main", "func main() {}"}},
			},
			want: `#raw(
  block: true,
  lang: "go",
  "package main",
  "func main() {}",
)
`,
		},
		{
			name: "raw_inline",
			content: Content{
				&Raw{Block: false, Lines: []string{"code"}},
			},
			want: "#raw(\"code\")\n",
		},
		{
			name: "linebreak",
			content: Content{
				&Linebreak{},
			},
			want: "#linebreak()\n",
		},
		{
			name: "parbreak",
			content: Content{
				&Parbreak{},
			},
			want: "#parbreak()\n",
		},
		{
			name: "link",
			content: Content{
				&Link{Dest: "https://example.com", Body: Content{&Text{Value: "example"}}},
			},
			want: "#link(dest: \"https://example.com\")[#text(\"example\")]\n",
		},
		{
			name: "list",
			content: Content{
				&List{Items: []ListItem{
					{Body: Content{&Text{Value: "item 1"}}},
					{Body: Content{&Text{Value: "item 2"}}},
				}},
			},
			want: "#list(list.item[#text(\"item 1\")], list.item[#text(\"item 2\")])\n",
		},
		{
			name: "enum",
			content: Content{
				&Enum{Items: []EnumItem{
					{Number: 1, Body: Content{&Text{Value: "first"}}},
					{Number: 2, Body: Content{&Text{Value: "second"}}},
				}},
			},
			want: `#enum(enum.item(1)[#text("first")], enum.item(2)[#text("second")])
`,
		},
		{
			name: "terms",
			content: Content{
				&Terms{Items: []TermItem{
					{Term: Content{&Text{Value: "key"}}, Description: Content{&Text{Value: "value"}}},
				}},
			},
			want: "#terms(terms.item[#text(\"key\")][#text(\"value\")])\n",
		},
		{
			name: "nested_content",
			content: Content{
				&Heading{Level: 1, Body: Content{
					&Strong{Body: Content{&Text{Value: "Important"}}},
				}},
				&Text{Value: "Some text with "},
				&Emph{Body: Content{&Text{Value: "emphasis"}}},
			},
			want: `#heading(level: 1)[#strong[#text("Important")]]
#text("Some text with ")
#emph[#text("emphasis")]
`,
		},
		{
			name: "multi_item_content_block",
			content: Content{
				&List{Items: []ListItem{
					{Body: Content{
						&Text{Value: "first "},
						&Strong{Body: Content{&Text{Value: "bold"}}},
						&Text{Value: " last"},
					}},
				}},
			},
			want: `#list(list.item[
  #text("first ")
  #strong[#text("bold")]
  #text(" last")
])
`,
		},
		{
			name: "heading_with_multi_item_body",
			content: Content{
				&Heading{Level: 2, Body: Content{
					&Text{Value: "Hello "},
					&Emph{Body: Content{&Text{Value: "world"}}},
				}},
			},
			want: `#heading(level: 2)[
  #text("Hello ")
  #emph[#text("world")]
]
`,
		},
		{
			name: "deeply_nested_markup",
			content: Content{
				&List{Items: []ListItem{
					{Body: Content{
						&Text{Value: "outer"},
						&List{Items: []ListItem{
							{Body: Content{
								&Text{Value: "middle"},
								&List{Items: []ListItem{
									{Body: Content{&Text{Value: "inner"}}},
								}},
							}},
						}},
					}},
				}},
			},
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
			content: Content{
				&FuncCall{
					Callee: &Ident{Name: "outer"},
					Args: []Expr{
						&FuncCall{
							Callee: &Ident{Name: "middle"},
							Args: []Expr{
								&FuncCall{
									Callee: &Ident{Name: "inner"},
									Args:   []Expr{&Int{Value: 42}},
								},
							},
						},
					},
				},
			},
			want: "#outer(middle(inner(42)))\n",
		},
		{
			name: "nested_conditionals",
			content: Content{
				&Conditional{
					Condition: &Bool{Value: true},
					Then: &CodeBlock{Exprs: []Expr{
						&Conditional{
							Condition: &Bool{Value: false},
							Then: &CodeBlock{Exprs: []Expr{
								&Int{Value: 1},
							}},
							Else: &CodeBlock{Exprs: []Expr{
								&Int{Value: 2},
							}},
						},
					}},
				},
			},
			want: "#if true { if false { 1 } else { 2 } }\n",
		},
		{
			name: "nested_arrays_and_dicts",
			content: Content{
				&Array{Items: []Expr{
					&Array{Items: []Expr{
						&Dict{Items: []Expr{
							&Named{Name: "key", Value: &Array{Items: []Expr{
								&Int{Value: 1},
								&Int{Value: 2},
							}}},
						}},
					}},
				}},
			},
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
		content Content
		want    string
	}{
		// Literals
		{
			name:    "none",
			content: Content{&None{}},
			want:    "#none\n",
		},
		{
			name:    "auto",
			content: Content{&Auto{}},
			want:    "#auto\n",
		},
		{
			name:    "bool_true",
			content: Content{&Bool{Value: true}},
			want:    "#true\n",
		},
		{
			name:    "bool_false",
			content: Content{&Bool{Value: false}},
			want:    "#false\n",
		},
		{
			name:    "int",
			content: Content{&Int{Value: 42}},
			want:    "#42\n",
		},
		{
			name:    "float",
			content: Content{&Float{Value: 3.14}},
			want:    "#3.14\n",
		},
		{
			name:    "numeric",
			content: Content{&Numeric{Value: 12, Unit: UnitPt}},
			want:    "#12pt\n",
		},
		{
			name:    "string",
			content: Content{&Str{Value: "hello"}},
			want:    "#\"hello\"\n",
		},
		{
			name:    "string_with_escapes",
			content: Content{&Str{Value: "line1\nline2\ttab"}},
			want:    "#\"line1\\nline2\\ttab\"\n",
		},
		{
			name:    "ident",
			content: Content{&Ident{Name: "foo"}},
			want:    "#foo\n",
		},
		{
			name:    "underscore",
			content: Content{&Underscore{}},
			want:    "#_\n",
		},

		// Code blocks
		{
			name:    "code_block_empty",
			content: Content{&CodeBlock{Exprs: nil}},
			want:    "#{ }\n",
		},
		{
			name:    "code_block_single",
			content: Content{&CodeBlock{Exprs: []Expr{&Int{Value: 1}}}},
			want:    "#{ 1 }\n",
		},
		{
			name:    "code_block_multiple",
			content: Content{&CodeBlock{Exprs: []Expr{&Int{Value: 1}, &Int{Value: 2}, &Int{Value: 3}}}},
			want:    "#{ 1; 2; 3 }\n",
		},

		// Content block
		{
			name:    "content_block",
			content: Content{&ContentBlock{Body: Content{&Text{Value: "hello"}}}},
			want:    "#[#text(\"hello\")]\n",
		},

		// Parenthesized
		{
			name:    "parenthesized",
			content: Content{&Parenthesized{Body: &Int{Value: 1}}},
			want:    "#(1)\n",
		},

		// Arrays
		{
			name:    "array_empty",
			content: Content{&Array{Items: nil}},
			want:    "#()\n",
		},
		{
			name:    "array_single",
			content: Content{&Array{Items: []Expr{&Int{Value: 1}}}},
			want:    "#(1,)\n",
		},
		{
			name:    "array_multiple",
			content: Content{&Array{Items: []Expr{&Int{Value: 1}, &Int{Value: 2}, &Int{Value: 3}}}},
			want:    "#(1, 2, 3)\n",
		},
		{
			name:    "array_single_spread",
			content: Content{&Array{Items: []Expr{&Spread{Expr: &Ident{Name: "items"}}}}},
			want:    "#(..items)\n",
		},

		// Dictionaries
		{
			name:    "dict_empty",
			content: Content{&Dict{Items: nil}},
			want:    "#(:)\n",
		},
		{
			name:    "dict_named",
			content: Content{&Dict{Items: []Expr{&Named{Name: "a", Value: &Int{Value: 1}}}}},
			want:    "#(a: 1)\n",
		},
		{
			name:    "dict_keyed",
			content: Content{&Dict{Items: []Expr{&Keyed{Key: &Str{Value: "key"}, Value: &Str{Value: "value"}}}}},
			want:    "#(\"key\": \"value\")\n",
		},

		// Spread
		{
			name:    "spread",
			content: Content{&Spread{Expr: &Ident{Name: "items"}}},
			want:    "#..items\n",
		},

		// Operators
		{
			name:    "unary_minus",
			content: Content{&Unary{Op: syntax.Neg, Operand: &Int{Value: 1}}},
			want:    "#-1\n",
		},
		{
			name:    "unary_not",
			content: Content{&Unary{Op: syntax.Not, Operand: &Bool{Value: true}}},
			want:    "#not true\n",
		},
		{
			name:    "binary_add",
			content: Content{&Binary{Left: &Int{Value: 1}, Op: syntax.Add, Right: &Int{Value: 2}}},
			want:    "#1 + 2\n",
		},
		{
			name:    "binary_eq",
			content: Content{&Binary{Left: &Ident{Name: "x"}, Op: syntax.Eq, Right: &Int{Value: 1}}},
			want:    "#x == 1\n",
		},
		{
			name:    "binary_assign",
			content: Content{&Binary{Left: &Ident{Name: "x"}, Op: syntax.Assign, Right: &Int{Value: 1}}},
			want:    "#x = 1\n",
		},

		// Field access
		{
			name:    "field_access",
			content: Content{&FieldAccess{Target: &Ident{Name: "foo"}, Field: "bar"}},
			want:    "#foo.bar\n",
		},
		{
			name:    "field_access_nested",
			content: Content{&FieldAccess{Target: &FieldAccess{Target: &Ident{Name: "a"}, Field: "b"}, Field: "c"}},
			want:    "#a.b.c\n",
		},

		// Function calls
		{
			name:    "func_call_no_args",
			content: Content{&FuncCall{Callee: &Ident{Name: "foo"}, Args: nil}},
			want:    "#foo()\n",
		},
		{
			name:    "func_call_with_args",
			content: Content{&FuncCall{Callee: &Ident{Name: "foo"}, Args: []Expr{&Int{Value: 1}, &Int{Value: 2}}}},
			want:    "#foo(1, 2)\n",
		},
		{
			name:    "func_call_with_named_args",
			content: Content{&FuncCall{Callee: &Ident{Name: "foo"}, Args: []Expr{&Named{Name: "a", Value: &Int{Value: 1}}}}},
			want:    "#foo(a: 1)\n",
		},
		{
			name:    "func_call_with_content",
			content: Content{&FuncCall{Callee: &Ident{Name: "foo"}, Args: nil, Content: []Content{{&Text{Value: "hello"}}}}},
			want:    "#foo()[#text(\"hello\")]\n",
		},
		{
			name: "func_call_with_args_and_content",
			content: Content{&FuncCall{
				Callee:  &Ident{Name: "foo"},
				Args:    []Expr{&Int{Value: 1}},
				Content: []Content{{&Text{Value: "hello"}}},
			}},
			want: "#foo(1)[#text(\"hello\")]\n",
		},
		{
			name:    "method_call",
			content: Content{&FuncCall{Callee: &FieldAccess{Target: &Ident{Name: "foo"}, Field: "bar"}, Args: nil}},
			want:    "#foo.bar()\n",
		},

		// Closures
		{
			name:    "closure_single_param",
			content: Content{&Closure{Params: []Expr{&Ident{Name: "x"}}, Body: &Binary{Left: &Ident{Name: "x"}, Op: syntax.Add, Right: &Int{Value: 1}}}},
			want:    "#x => x + 1\n",
		},
		{
			name:    "closure_multi_param",
			content: Content{&Closure{Params: []Expr{&Ident{Name: "x"}, &Ident{Name: "y"}}, Body: &Binary{Left: &Ident{Name: "x"}, Op: syntax.Add, Right: &Ident{Name: "y"}}}},
			want:    "#(x, y) => x + y\n",
		},
		{
			name:    "closure_spread_param",
			content: Content{&Closure{Params: []Expr{&Spread{Expr: &Ident{Name: "args"}}}, Body: &Ident{Name: "args"}}},
			want:    "#(..args) => args\n",
		},
		{
			name:    "closure_named",
			content: Content{&Closure{Name: "add", Params: []Expr{&Ident{Name: "x"}, &Ident{Name: "y"}}, Body: &Binary{Left: &Ident{Name: "x"}, Op: syntax.Add, Right: &Ident{Name: "y"}}}},
			want:    "#add(x, y) = x + y\n",
		},

		// Let bindings
		{
			name:    "let_simple",
			content: Content{&LetBinding{Pattern: &Ident{Name: "x"}, Value: &Int{Value: 1}}},
			want:    "#let x = 1\n",
		},
		{
			name:    "let_pattern",
			content: Content{&LetBinding{Pattern: &Destructuring{Items: []Expr{&Ident{Name: "a"}, &Ident{Name: "b"}}}, Value: &Ident{Name: "pair"}}},
			want:    "#let (a, b) = pair\n",
		},
		{
			name: "let_function",
			content: Content{&LetBinding{
				Pattern: &Ident{Name: "add"},
				Value:   &Closure{Name: "add", Params: []Expr{&Ident{Name: "x"}, &Ident{Name: "y"}}, Body: &Binary{Left: &Ident{Name: "x"}, Op: syntax.Add, Right: &Ident{Name: "y"}}},
			}},
			want: "#let add(x, y) = x + y\n",
		},

		// Set rules
		{
			name:    "set_rule",
			content: Content{&SetRule{Target: &Ident{Name: "text"}, Args: []Expr{&Named{Name: "size", Value: &Numeric{Value: 12, Unit: UnitPt}}}}},
			want:    "#set text(size: 12pt)\n",
		},
		{
			name:    "set_rule_with_condition",
			content: Content{&SetRule{Target: &Ident{Name: "text"}, Args: []Expr{&Ident{Name: "red"}}, Condition: &Ident{Name: "enabled"}}},
			want:    "#set text(red) if enabled\n",
		},

		// Show rules
		{
			name:    "show_rule_no_selector",
			content: Content{&ShowRule{Selector: nil, Transform: &Ident{Name: "emph"}}},
			want:    "#show: emph\n",
		},
		{
			name:    "show_rule_with_selector",
			content: Content{&ShowRule{Selector: &Ident{Name: "heading"}, Transform: &Closure{Params: []Expr{&Ident{Name: "it"}}, Body: &FuncCall{Callee: &Ident{Name: "emph"}, Args: []Expr{&FieldAccess{Target: &Ident{Name: "it"}, Field: "body"}}}}}},
			want:    "#show heading: it => emph(it.body)\n",
		},

		// Conditionals
		{
			name:    "if_only",
			content: Content{&Conditional{Condition: &Ident{Name: "x"}, Then: &CodeBlock{Exprs: []Expr{&Ident{Name: "y"}}}}},
			want:    "#if x { y }\n",
		},
		{
			name:    "if_else",
			content: Content{&Conditional{Condition: &Ident{Name: "x"}, Then: &CodeBlock{Exprs: []Expr{&Ident{Name: "y"}}}, Else: &CodeBlock{Exprs: []Expr{&Ident{Name: "z"}}}}},
			want:    "#if x { y } else { z }\n",
		},
		{
			name: "if_else_if",
			content: Content{&Conditional{
				Condition: &Ident{Name: "a"},
				Then:      &CodeBlock{Exprs: []Expr{&Int{Value: 1}}},
				Else: &Conditional{
					Condition: &Ident{Name: "b"},
					Then:      &CodeBlock{Exprs: []Expr{&Int{Value: 2}}},
					Else:      &CodeBlock{Exprs: []Expr{&Int{Value: 3}}},
				},
			}},
			want: "#if a { 1 } else if b { 2 } else { 3 }\n",
		},

		// Loops
		{
			name:    "while_loop",
			content: Content{&WhileLoop{Condition: &Ident{Name: "x"}, Body: &CodeBlock{Exprs: []Expr{&Ident{Name: "y"}}}}},
			want:    "#while x { y }\n",
		},
		{
			name:    "for_loop",
			content: Content{&ForLoop{Pattern: &Ident{Name: "x"}, Iterable: &Ident{Name: "items"}, Body: &CodeBlock{Exprs: []Expr{&Ident{Name: "x"}}}}},
			want:    "#for x in items { x }\n",
		},
		{
			name:    "for_loop_pattern",
			content: Content{&ForLoop{Pattern: &Destructuring{Items: []Expr{&Ident{Name: "k"}, &Ident{Name: "v"}}}, Iterable: &Ident{Name: "dict"}, Body: &CodeBlock{Exprs: []Expr{&Ident{Name: "k"}}}}},
			want:    "#for (k, v) in dict { k }\n",
		},

		// Control flow
		{
			name:    "break",
			content: Content{&LoopBreak{}},
			want:    "#break\n",
		},
		{
			name:    "continue",
			content: Content{&LoopContinue{}},
			want:    "#continue\n",
		},
		{
			name:    "return_with_value",
			content: Content{&FuncReturn{Value: &Int{Value: 1}}},
			want:    "#return 1\n",
		},
		{
			name:    "return_no_value",
			content: Content{&FuncReturn{Value: nil}},
			want:    "#return\n",
		},

		// Other
		{
			name:    "context",
			content: Content{&Contextual{Body: &FieldAccess{Target: &Ident{Name: "text"}, Field: "lang"}}},
			want:    "#context text.lang\n",
		},
		{
			name:    "include",
			content: Content{&ModuleInclude{Source: &Str{Value: "other.typ"}}},
			want:    "#include \"other.typ\"\n",
		},
		{
			name:    "destruct_assign",
			content: Content{&DestructAssignment{Pattern: &Underscore{}, Value: &Ident{Name: "expr"}}},
			want:    "#_ = expr\n",
		},
		{
			name:    "destructuring",
			content: Content{&Destructuring{Items: []Expr{&Ident{Name: "x"}, &Ident{Name: "y"}}}},
			want:    "#(x, y)\n",
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
