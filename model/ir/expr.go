package ir

import (
	"unique"

	"znkr.io/writst/syntax"
)

type Expr interface {
	Span() syntax.Span
	Eval(ec *EvalContext) Value

	format(f *formatter)
	aExpr()
}

type expr struct {
	span syntax.Span
}

func (e *expr) Span() syntax.Span { return e.span }
func (e *expr) aExpr()            {}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

type ContentExpr struct {
	expr
	exprs []Expr
}

func NewContentExpr(span syntax.Span, exprs []Expr) *ContentExpr {
	return &ContentExpr{expr: expr{span: span}, exprs: exprs}
}

func (n *ContentExpr) Exprs() []Expr { return n.exprs }

type HeadingExpr struct {
	expr
	level int
	body  *ContentExpr
}

func NewHeadingExpr(span syntax.Span, level int, body *ContentExpr) *HeadingExpr {
	return &HeadingExpr{expr: expr{span: span}, level: level, body: body}
}

func (n *HeadingExpr) Level() int         { return n.level }
func (n *HeadingExpr) Body() *ContentExpr { return n.body }

type StrongExpr struct {
	expr
	body *ContentExpr
}

func NewStrongExpr(span syntax.Span, body *ContentExpr) *StrongExpr {
	return &StrongExpr{expr: expr{span: span}, body: body}
}

func (n *StrongExpr) Body() *ContentExpr { return n.body }

type EmphExpr struct {
	expr
	body *ContentExpr
}

func NewEmphExpr(span syntax.Span, body *ContentExpr) *EmphExpr {
	return &EmphExpr{expr: expr{span: span}, body: body}
}

func (n *EmphExpr) Body() *ContentExpr { return n.body }

type LinkExpr struct {
	expr
	dest string
	body *ContentExpr
}

func NewLinkExpr(span syntax.Span, dest string, body *ContentExpr) *LinkExpr {
	return &LinkExpr{expr: expr{span: span}, dest: dest, body: body}
}

func (n *LinkExpr) Dest() string       { return n.dest }
func (n *LinkExpr) Body() *ContentExpr { return n.body }

type RefExpr struct {
	expr
	target     unique.Handle[string]
	supplement *ContentExpr
}

func NewRefExpr(span syntax.Span, target unique.Handle[string], supplement *ContentExpr) *RefExpr {
	return &RefExpr{expr: expr{span: span}, target: target, supplement: supplement}
}

func (n *RefExpr) Target() unique.Handle[string] { return n.target }
func (n *RefExpr) Supplement() *ContentExpr      { return n.supplement }

type ListItemExpr struct {
	expr
	body *ContentExpr
}

func NewListItemExpr(span syntax.Span, body *ContentExpr) *ListItemExpr {
	return &ListItemExpr{expr: expr{span: span}, body: body}
}

func (n *ListItemExpr) Body() *ContentExpr { return n.body }

type EnumItemExpr struct {
	expr
	number int
	body   *ContentExpr
}

func NewEnumItemExpr(span syntax.Span, number int, body *ContentExpr) *EnumItemExpr {
	return &EnumItemExpr{expr: expr{span: span}, number: number, body: body}
}

func (n *EnumItemExpr) Number() int        { return n.number }
func (n *EnumItemExpr) Body() *ContentExpr { return n.body }

type TermItemExpr struct {
	expr
	term        *ContentExpr
	description *ContentExpr
}

func NewTermItemExpr(span syntax.Span, term, description *ContentExpr) *TermItemExpr {
	return &TermItemExpr{expr: expr{span: span}, term: term, description: description}
}

func (n *TermItemExpr) Term() *ContentExpr        { return n.term }
func (n *TermItemExpr) Description() *ContentExpr { return n.description }

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

type Const struct {
	expr
	value Value
}

func NewConst(span syntax.Span, value Value) *Const {
	return &Const{expr: expr{span: span}, value: value}
}

func (n *Const) Value() Value { return n.value }

type Ident struct {
	expr
	name unique.Handle[string]
}

func NewIdent(span syntax.Span, name unique.Handle[string]) *Ident {
	return &Ident{expr: expr{span: span}, name: name}
}

func (n *Ident) Name() unique.Handle[string] { return n.name }

type CodeBlock struct {
	expr
	body *ContentExpr
}

func NewCodeBlock(span syntax.Span, body *ContentExpr) *CodeBlock {
	return &CodeBlock{expr: expr{span: span}, body: body}
}

func (n *CodeBlock) Body() *ContentExpr { return n.body }

type ContentBlock struct {
	expr
	body *ContentExpr
}

func NewContentBlock(span syntax.Span, body *ContentExpr) *ContentBlock {
	return &ContentBlock{expr: expr{span: span}, body: body}
}

func (n *ContentBlock) Body() *ContentExpr { return n.body }

type Parenthesized struct {
	expr
	body Expr
}

func NewParenthesized(span syntax.Span, body Expr) *Parenthesized {
	return &Parenthesized{expr: expr{span: span}, body: body}
}

func (n *Parenthesized) Body() Expr { return n.body }

// Collections /////////////////////////////////////////////////////////////////////////////////////

type ArrayExpr struct {
	expr
	elements []Expr
}

func NewArrayExpr(span syntax.Span, elements []Expr) *ArrayExpr {
	return &ArrayExpr{expr: expr{span: span}, elements: elements}
}

func (n *ArrayExpr) Elements() []Expr { return n.elements }

type DictExpr struct {
	expr
	entries []DictItemExpr
}

func NewDictExpr(span syntax.Span, entries []DictItemExpr) *DictExpr {
	return &DictExpr{expr: expr{span: span}, entries: entries}
}

func (n *DictExpr) Entries() []DictItemExpr { return n.entries }

type DictItemExpr struct {
	key   Expr
	value Expr
}

func NewDictItemExpr(key, value Expr) DictItemExpr {
	return DictItemExpr{key: key, value: value}
}

func (n DictItemExpr) Key() Expr   { return n.key }
func (n DictItemExpr) Value() Expr { return n.value }

// Operators ///////////////////////////////////////////////////////////////////////////////////////

type Unary struct {
	expr
	op      syntax.UnaryOp
	operand Expr
}

func NewUnary(span syntax.Span, op syntax.UnaryOp, operand Expr) *Unary {
	return &Unary{expr: expr{span: span}, op: op, operand: operand}
}

func (n *Unary) Op() syntax.UnaryOp { return n.op }
func (n *Unary) Operand() Expr      { return n.operand }

type Binary struct {
	expr
	left  Expr
	op    syntax.BinaryOp
	right Expr
}

func NewBinary(span syntax.Span, left Expr, op syntax.BinaryOp, right Expr) *Binary {
	return &Binary{expr: expr{span: span}, left: left, op: op, right: right}
}

func (n *Binary) Left() Expr          { return n.left }
func (n *Binary) Op() syntax.BinaryOp { return n.op }
func (n *Binary) Right() Expr         { return n.right }

type FieldAccess struct {
	expr
	target Expr
	field  string
}

func NewFieldAccess(span syntax.Span, target Expr, field string) *FieldAccess {
	return &FieldAccess{expr: expr{span: span}, target: target, field: field}
}

func (n *FieldAccess) Target() Expr  { return n.target }
func (n *FieldAccess) Field() string { return n.field }

// Arguments ///////////////////////////////////////////////////////////////////////////////////////

type Arg interface {
	aArg()
}

type ExprArg struct {
	expr Expr
}

func NewExprArg(expr Expr) *ExprArg {
	return &ExprArg{expr: expr}
}

func (n *ExprArg) Expr() Expr { return n.expr }

type NamedArg struct {
	name unique.Handle[string]
	expr Expr
}

func NewNamedArg(name unique.Handle[string], expr Expr) *NamedArg {
	return &NamedArg{name: name, expr: expr}
}

func (n *NamedArg) Name() unique.Handle[string] { return n.name }
func (n *NamedArg) Expr() Expr                  { return n.expr }

type SpreadArg struct {
	expr Expr
}

func NewSpreadArg(expr Expr) *SpreadArg {
	return &SpreadArg{expr: expr}
}

func (n *SpreadArg) Expr() Expr { return n.expr }

func (*ExprArg) aArg()   {}
func (*NamedArg) aArg()  {}
func (*SpreadArg) aArg() {}

// Parameters //////////////////////////////////////////////////////////////////////////////////////

type Param interface {
	aParam()
}

type PositionalParam struct {
	ident *Ident
}

func NewPositionalParam(ident *Ident) *PositionalParam {
	return &PositionalParam{ident: ident}
}

func (n *PositionalParam) Ident() *Ident { return n.ident }

type NamedParam struct {
	name unique.Handle[string]
	def  Expr
}

func NewNamedParam(name unique.Handle[string], def Expr) *NamedParam {
	return &NamedParam{name: name, def: def}
}

func (n *NamedParam) Name() unique.Handle[string] { return n.name }
func (n *NamedParam) Default() Expr               { return n.def }

type SpreadParam struct {
	ident *Ident
}

func NewSpreadParam(ident *Ident) *SpreadParam {
	return &SpreadParam{ident: ident}
}

func (n *SpreadParam) Ident() *Ident { return n.ident }

func (*PositionalParam) aParam() {}
func (*NamedParam) aParam()      {}
func (*SpreadParam) aParam()     {}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type FuncCall struct {
	expr
	callee  Expr
	args    []Arg
	content []*ContentExpr
}

func NewFuncCall(span syntax.Span, callee Expr, args []Arg, content []*ContentExpr) *FuncCall {
	return &FuncCall{expr: expr{span: span}, callee: callee, args: args, content: content}
}

func (n *FuncCall) Callee() Expr            { return n.callee }
func (n *FuncCall) Args() []Arg             { return n.args }
func (n *FuncCall) Content() []*ContentExpr { return n.content }

type Closure struct {
	expr
	name   *Ident
	params []Param
	body   Expr
}

func NewClosure(span syntax.Span, name *Ident, params []Param, body Expr) *Closure {
	return &Closure{expr: expr{span: span}, name: name, params: params, body: body}
}

func (n *Closure) Name() *Ident    { return n.name }
func (n *Closure) Params() []Param { return n.params }
func (n *Closure) Body() Expr      { return n.body }

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

type LetBinding struct {
	expr
	pattern []DestructPattern
	value   Expr
}

func NewLetBinding(span syntax.Span, pattern []DestructPattern, value Expr) *LetBinding {
	return &LetBinding{expr: expr{span: span}, pattern: pattern, value: value}
}

func (n *LetBinding) Pattern() []DestructPattern { return n.pattern }
func (n *LetBinding) Value() Expr                { return n.value }

type SetRule struct {
	expr
	target    Expr
	args      []Arg
	condition Expr
}

func NewSetRule(span syntax.Span, target Expr, args []Arg, condition Expr) *SetRule {
	return &SetRule{expr: expr{span: span}, target: target, args: args, condition: condition}
}

func (n *SetRule) Target() Expr    { return n.target }
func (n *SetRule) Args() []Arg     { return n.args }
func (n *SetRule) Condition() Expr { return n.condition }

type ShowRule struct {
	expr
	selector  Expr
	transform Expr
}

func NewShowRule(span syntax.Span, selector, transform Expr) *ShowRule {
	return &ShowRule{expr: expr{span: span}, selector: selector, transform: transform}
}

func (n *ShowRule) Selector() Expr  { return n.selector }
func (n *ShowRule) Transform() Expr { return n.transform }

// Destructuring ///////////////////////////////////////////////////////////////////////////////////

type Destructuring struct {
	expr
	items []Expr
}

func NewDestructuring(span syntax.Span, items []Expr) *Destructuring {
	return &Destructuring{expr: expr{span: span}, items: items}
}

func (n *Destructuring) Items() []Expr { return n.items }

type DestructAssignment struct {
	expr
	pattern []DestructPattern
	value   Expr
}

func NewDestructAssignment(span syntax.Span, pattern []DestructPattern, value Expr) *DestructAssignment {
	return &DestructAssignment{expr: expr{span: span}, pattern: pattern, value: value}
}

func (n *DestructAssignment) Pattern() []DestructPattern { return n.pattern }
func (n *DestructAssignment) Value() Expr                { return n.value }

type DestructPattern interface {
	aDestructPattern()
	format(f *formatter)
}

type DestructIdent struct {
	ident *Ident
}

func NewDestructIdent(ident *Ident) *DestructIdent {
	return &DestructIdent{ident: ident}
}

func (n *DestructIdent) Ident() *Ident { return n.ident }

type DestructNamed struct {
	name    unique.Handle[string]
	pattern *Ident
}

func NewDestructNamed(name unique.Handle[string], pattern *Ident) *DestructNamed {
	return &DestructNamed{name: name, pattern: pattern}
}

func (n *DestructNamed) Name() unique.Handle[string] { return n.name }
func (n *DestructNamed) Pattern() *Ident             { return n.pattern }

type DestructSink struct {
	ident *Ident
}

func NewDestructSink(ident *Ident) *DestructSink {
	return &DestructSink{ident: ident}
}

func (n *DestructSink) Ident() *Ident { return n.ident }

func (*DestructIdent) aDestructPattern() {}
func (*DestructNamed) aDestructPattern() {}
func (*DestructSink) aDestructPattern()  {}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

type Conditional struct {
	expr
	condition Expr
	then      Expr
	els       Expr
}

func NewConditional(span syntax.Span, condition, then, els Expr) *Conditional {
	return &Conditional{expr: expr{span: span}, condition: condition, then: then, els: els}
}

func (n *Conditional) Condition() Expr { return n.condition }
func (n *Conditional) Then() Expr      { return n.then }
func (n *Conditional) Else() Expr      { return n.els }

type WhileLoop struct {
	expr
	condition Expr
	body      Expr
}

func NewWhileLoop(span syntax.Span, condition, body Expr) *WhileLoop {
	return &WhileLoop{expr: expr{span: span}, condition: condition, body: body}
}

func (n *WhileLoop) Condition() Expr { return n.condition }
func (n *WhileLoop) Body() Expr      { return n.body }

type ForLoop struct {
	expr
	pattern  []DestructPattern
	iterable Expr
	body     Expr
}

func NewForLoop(span syntax.Span, pattern []DestructPattern, iterable, body Expr) *ForLoop {
	return &ForLoop{expr: expr{span: span}, pattern: pattern, iterable: iterable, body: body}
}

func (n *ForLoop) Pattern() []DestructPattern { return n.pattern }
func (n *ForLoop) Iterable() Expr             { return n.iterable }
func (n *ForLoop) Body() Expr                 { return n.body }

type LoopBreak struct {
	expr
}

func NewLoopBreak(span syntax.Span) *LoopBreak {
	return &LoopBreak{expr: expr{span: span}}
}

type LoopContinue struct {
	expr
}

func NewLoopContinue(span syntax.Span) *LoopContinue {
	return &LoopContinue{expr: expr{span: span}}
}

type FuncReturn struct {
	expr
	value Expr
}

func NewFuncReturn(span syntax.Span, value Expr) *FuncReturn {
	return &FuncReturn{expr: expr{span: span}, value: value}
}

func (n *FuncReturn) Value() Expr { return n.value }

// Other ///////////////////////////////////////////////////////////////////////////////////////////

type Contextual struct {
	expr
	body Expr
}

func NewContextual(span syntax.Span, body Expr) *Contextual {
	return &Contextual{expr: expr{span: span}, body: body}
}

func (n *Contextual) Body() Expr { return n.body }

type ModuleInclude struct {
	expr
	source Expr
}

func NewModuleInclude(span syntax.Span, source Expr) *ModuleInclude {
	return &ModuleInclude{expr: expr{span: span}, source: source}
}

func (n *ModuleInclude) Source() Expr { return n.source }
