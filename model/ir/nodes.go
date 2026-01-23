package ir

import (
	"unique"

	"znkr.io/writst/syntax"
)

type Expr interface {
	aExpr()
	format(*formatter)
}

// Content /////////////////////////////////////////////////////////////////////////////////////////

type Content []Expr

func (Content) aExpr() {}

type Label struct {
	Name unique.Handle[string]
}

func (*Label) aExpr() {}

// Markup //////////////////////////////////////////////////////////////////////////////////////////

type Heading struct {
	Level int
	Body  Content
}

type Strong struct {
	Body Content
}

type Emph struct {
	Body Content
}

type Text struct {
	Value string
}

type Raw struct {
	Block bool
	Lang  string
	Lines []string
}

type Linebreak struct{}

type Parbreak struct{}

type Link struct {
	Dest string
	Body Content
}

type Ref struct {
	Target     unique.Handle[string]
	Supplement Content
}

type List struct {
	Items []ListItem
}

type ListItem struct {
	Body Content
}

type Enum struct {
	Items []EnumItem
}

type EnumItem struct {
	Number int
	Body   Content
}

type Terms struct {
	Items []TermItem
}

type TermItem struct {
	Term        Content
	Description Content
}

// Code ////////////////////////////////////////////////////////////////////////////////////////////

type None struct{}

type Auto struct{}

type Bool struct {
	Value bool
}

type Int struct {
	Value int64
}

type Float struct {
	Value float64
}

type Numeric struct {
	Value float64
	Unit  Unit
}

type Str struct {
	Value string
}

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

type Ident struct {
	Name string
}

type CodeBlock struct {
	Exprs []Expr
}

type ContentBlock struct {
	Body Content
}

type Underscore struct{}

type Parenthesized struct {
	Body Expr
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

type Array struct {
	Items []Expr
}

type Dict struct {
	Items []Expr
}

type Named struct {
	Name  string
	Value Expr
}

type Keyed struct {
	Key   Expr
	Value Expr
}

type Spread struct {
	Expr Expr
}

// Operators ///////////////////////////////////////////////////////////////////////////////////////

type Unary struct {
	Op      syntax.UnaryOp
	Operand Expr
}

type Binary struct {
	Left  Expr
	Op    syntax.BinaryOp
	Right Expr
}

type FieldAccess struct {
	Target Expr
	Field  string
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type FuncCall struct {
	Callee  Expr
	Args    []Expr
	Content []Content
}

type Closure struct {
	Name   string
	Params []Expr
	Body   Expr
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

type LetBinding struct {
	Pattern Expr
	Value   Expr
}

type SetRule struct {
	Target    Expr
	Args      []Expr
	Condition Expr
}

type ShowRule struct {
	Selector  Expr
	Transform Expr
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

type Conditional struct {
	Condition Expr
	Then      Expr
	Else      Expr
}

type WhileLoop struct {
	Condition Expr
	Body      Expr
}

type ForLoop struct {
	Pattern  Expr
	Iterable Expr
	Body     Expr
}

type LoopBreak struct{}

type LoopContinue struct{}

type FuncReturn struct {
	Value Expr
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

type Contextual struct {
	Body Expr
}

type ModuleInclude struct {
	Source Expr
}

type DestructAssignment struct {
	Pattern Expr
	Value   Expr
}

type Destructuring struct {
	Items []Expr
}

func (*Heading) aExpr()            {}
func (*Text) aExpr()               {}
func (*Raw) aExpr()                {}
func (*Strong) aExpr()             {}
func (*Emph) aExpr()               {}
func (*Linebreak) aExpr()          {}
func (*Link) aExpr()               {}
func (*Ref) aExpr()                {}
func (*List) aExpr()               {}
func (*ListItem) aExpr()           {}
func (*Enum) aExpr()               {}
func (*EnumItem) aExpr()           {}
func (*Terms) aExpr()              {}
func (*TermItem) aExpr()           {}
func (*Parbreak) aExpr()           {}
func (*None) aExpr()               {}
func (*Auto) aExpr()               {}
func (*Bool) aExpr()               {}
func (*Int) aExpr()                {}
func (*Float) aExpr()              {}
func (*Numeric) aExpr()            {}
func (*Str) aExpr()                {}
func (*Ident) aExpr()              {}
func (*CodeBlock) aExpr()          {}
func (*ContentBlock) aExpr()       {}
func (*Underscore) aExpr()         {}
func (*Parenthesized) aExpr()      {}
func (*Array) aExpr()              {}
func (*Dict) aExpr()               {}
func (*Named) aExpr()              {}
func (*Keyed) aExpr()              {}
func (*Spread) aExpr()             {}
func (*Unary) aExpr()              {}
func (*Binary) aExpr()             {}
func (*FieldAccess) aExpr()        {}
func (*FuncCall) aExpr()           {}
func (*Closure) aExpr()            {}
func (*LetBinding) aExpr()         {}
func (*SetRule) aExpr()            {}
func (*ShowRule) aExpr()           {}
func (*Conditional) aExpr()        {}
func (*WhileLoop) aExpr()          {}
func (*ForLoop) aExpr()            {}
func (*LoopBreak) aExpr()          {}
func (*LoopContinue) aExpr()       {}
func (*FuncReturn) aExpr()         {}
func (*Contextual) aExpr()         {}
func (*ModuleInclude) aExpr()      {}
func (*DestructAssignment) aExpr() {}
func (*Destructuring) aExpr()      {}
