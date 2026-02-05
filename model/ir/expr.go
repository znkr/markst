package ir

import (
	"unique"

	"znkr.io/writst/syntax"
)

type Expr interface {
	Eval(ec *EvalContext) Value

	format(f *formatter)
	aExpr()
}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

type ContentExpr []Expr

func (ContentExpr) aExpr() {}

type HeadingExpr struct {
	Level int
	Body  ContentExpr
}

type StrongExpr struct {
	Body ContentExpr
}

type EmphExpr struct {
	Body ContentExpr
}

type LinkExpr struct {
	Dest string
	Body ContentExpr
}

type RefExpr struct {
	Target     unique.Handle[string]
	Supplement ContentExpr
}

type ListItemExpr struct {
	Body ContentExpr
}

type EnumItemExpr struct {
	Number int
	Body   ContentExpr
}

type TermItemExpr struct {
	Term        ContentExpr
	Description ContentExpr
}

func (*HeadingExpr) aExpr()  {}
func (*StrongExpr) aExpr()   {}
func (*EmphExpr) aExpr()     {}
func (*LinkExpr) aExpr()     {}
func (*RefExpr) aExpr()      {}
func (*ListItemExpr) aExpr() {}
func (*EnumItemExpr) aExpr() {}
func (*TermItemExpr) aExpr() {}

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

type Const struct {
	Value Value
}

type Ident struct {
	Name unique.Handle[string]
}

type CodeBlock struct {
	Body ContentExpr
}

type ContentBlock struct {
	Body ContentExpr
}

type Parenthesized struct {
	Body Expr
}

func (*Const) aExpr()         {}
func (*Ident) aExpr()         {}
func (*CodeBlock) aExpr()     {}
func (*ContentBlock) aExpr()  {}
func (*Parenthesized) aExpr() {}

// Collections /////////////////////////////////////////////////////////////////////////////////////

type ArrayExpr struct {
	Elements []Expr
}

type DictExpr struct {
	Entries []DictItemExpr
}

type DictItemExpr struct {
	Key   Expr
	Value Expr
}

func (*ArrayExpr) aExpr() {}
func (*DictExpr) aExpr()  {}

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

func (*Unary) aExpr()       {}
func (*Binary) aExpr()      {}
func (*FieldAccess) aExpr() {}

// Arguments ///////////////////////////////////////////////////////////////////////////////////////

type Arg interface {
	aArg()
}

type ExprArg struct {
	Expr
}

type NamedArg struct {
	Name unique.Handle[string]
	Expr Expr
}

type SpreadArg struct {
	Expr Expr
}

func (*ExprArg) aArg()   {}
func (*NamedArg) aArg()  {}
func (*SpreadArg) aArg() {}

// Parameters //////////////////////////////////////////////////////////////////////////////////////

type Param interface {
	aParam()
}

type PositionalParam struct {
	*Ident
}

type NamedParam struct {
	Name    unique.Handle[string]
	Default Expr
}

type SpreadParam struct {
	*Ident
}

func (*PositionalParam) aParam() {}
func (*NamedParam) aParam()      {}
func (*SpreadParam) aParam()     {}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type FuncCall struct {
	Callee  Expr
	Args    []Arg
	Content []ContentExpr
}

type Closure struct {
	Name   *Ident
	Params []Param
	Body   Expr
}

func (*FuncCall) aExpr() {}
func (*Closure) aExpr()  {}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

type LetBinding struct {
	Pattern []DestructPattern
	Value   Expr
}

type SetRule struct {
	Target    Expr
	Args      []Arg
	Condition Expr
}

type ShowRule struct {
	Selector  Expr
	Transform Expr
}

func (*LetBinding) aExpr() {}
func (*SetRule) aExpr()    {}
func (*ShowRule) aExpr()   {}

// Destructuring ///////////////////////////////////////////////////////////////////////////////////

type Destructuring struct {
	Items []Expr
}

func (*Destructuring) aExpr() {}

type DestructAssignment struct {
	Pattern []DestructPattern
	Value   Expr
}

func (*DestructAssignment) aExpr() {}

type DestructPattern interface {
	aDestructPattern()
	format(f *formatter)
}

type DestructIdent struct {
	*Ident
}

type DestructNamed struct {
	Name    unique.Handle[string]
	Pattern *Ident
}

type DestructSink struct {
	Ident *Ident
}

func (*DestructIdent) aDestructPattern() {}
func (*DestructNamed) aDestructPattern() {}
func (*DestructSink) aDestructPattern()  {}

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
	Pattern  []DestructPattern
	Iterable Expr
	Body     Expr
}

type LoopBreak struct{}

type LoopContinue struct{}

type FuncReturn struct {
	Value Expr
}

func (*Conditional) aExpr()  {}
func (*WhileLoop) aExpr()    {}
func (*ForLoop) aExpr()      {}
func (*LoopBreak) aExpr()    {}
func (*LoopContinue) aExpr() {}
func (*FuncReturn) aExpr()   {}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

type Contextual struct {
	Body Expr
}

type ModuleInclude struct {
	Source Expr
}

func (*Contextual) aExpr()    {}
func (*ModuleInclude) aExpr() {}
