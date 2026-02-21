package ir

import (
	"unique"

	"znkr.io/writst/syntax"
)

type Expr interface {
	Span() syntax.Span

	eval(ec *evalCtx) Value
	aExpr()
	formattable
}

type expr struct {
	span syntax.Span
}

func (e *expr) Span() syntax.Span { return e.span }
func (e *expr) aExpr()            {}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

type HeadingExpr struct {
	expr
	level int
	body  []Expr
}

func NewHeadingExpr(span syntax.Span, level int, body []Expr) *HeadingExpr {
	return &HeadingExpr{expr: expr{span: span}, level: level, body: body}
}

func (n *HeadingExpr) Level() int   { return n.level }
func (n *HeadingExpr) Body() []Expr { return n.body }

type StrongExpr struct {
	expr
	body []Expr
}

func NewStrongExpr(span syntax.Span, body []Expr) *StrongExpr {
	return &StrongExpr{expr: expr{span: span}, body: body}
}

func (n *StrongExpr) Body() []Expr { return n.body }

type EmphExpr struct {
	expr
	body []Expr
}

func NewEmphExpr(span syntax.Span, body []Expr) *EmphExpr {
	return &EmphExpr{expr: expr{span: span}, body: body}
}

func (n *EmphExpr) Body() []Expr { return n.body }

type LinkExpr struct {
	expr
	dest string
	body []Expr
}

func NewLinkExpr(span syntax.Span, dest string, body []Expr) *LinkExpr {
	return &LinkExpr{expr: expr{span: span}, dest: dest, body: body}
}

func (n *LinkExpr) Dest() string { return n.dest }
func (n *LinkExpr) Body() []Expr { return n.body }

type RefExpr struct {
	expr
	target     unique.Handle[string]
	supplement *ContentBlock
}

func NewRefExpr(span syntax.Span, target unique.Handle[string], supplement *ContentBlock) *RefExpr {
	return &RefExpr{expr: expr{span: span}, target: target, supplement: supplement}
}

func (n *RefExpr) Target() unique.Handle[string] { return n.target }
func (n *RefExpr) Supplement() *ContentBlock     { return n.supplement }

type ListItemExpr struct {
	expr
	body []Expr
}

func NewListItemExpr(span syntax.Span, body []Expr) *ListItemExpr {
	return &ListItemExpr{expr: expr{span: span}, body: body}
}

func (n *ListItemExpr) Body() []Expr { return n.body }

type EnumItemExpr struct {
	expr
	number int
	body   []Expr
}

func NewEnumItemExpr(span syntax.Span, number int, body []Expr) *EnumItemExpr {
	return &EnumItemExpr{expr: expr{span: span}, number: number, body: body}
}

func (n *EnumItemExpr) Number() int  { return n.number }
func (n *EnumItemExpr) Body() []Expr { return n.body }

type TermItemExpr struct {
	expr
	term        []Expr
	description []Expr
}

func NewTermItemExpr(span syntax.Span, term, description []Expr) *TermItemExpr {
	return &TermItemExpr{expr: expr{span: span}, term: term, description: description}
}

func (n *TermItemExpr) Term() []Expr        { return n.term }
func (n *TermItemExpr) Description() []Expr { return n.description }

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

type ConstExpr struct {
	expr
	value Value
}

func NewConstExpr(span syntax.Span, value Value) *ConstExpr {
	return &ConstExpr{expr: expr{span: span}, value: value}
}

func (n *ConstExpr) Value() Value { return n.value }

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
	exprs []Expr
}

func NewCodeBlock(span syntax.Span, body []Expr) *CodeBlock {
	return &CodeBlock{expr: expr{span: span}, exprs: body}
}

func (n *CodeBlock) Body() []Expr { return n.exprs }

type ContentBlock struct {
	expr
	exprs []Expr
}

func NewContentBlock(span syntax.Span, body []Expr) *ContentBlock {
	return &ContentBlock{expr: expr{span: span}, exprs: body}
}

func (n *ContentBlock) Body() []Expr { return n.exprs }

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
	field  *Ident
}

func NewFieldAccess(span syntax.Span, target Expr, field *Ident) *FieldAccess {
	return &FieldAccess{expr: expr{span: span}, target: target, field: field}
}

func (n *FieldAccess) Target() Expr  { return n.target }
func (n *FieldAccess) Field() *Ident { return n.field }

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

// Closure Parameters //////////////////////////////////////////////////////////////////////////////

type ClosureParam interface {
	aClosureParam()
}

type PositionalClosureParam struct {
	ident *Ident
}

func NewPositionalClosureParam(ident *Ident) *PositionalClosureParam {
	return &PositionalClosureParam{ident: ident}
}

func (n *PositionalClosureParam) Ident() *Ident { return n.ident }

type NamedClosureParam struct {
	name unique.Handle[string]
	def  Expr
}

func NewNamedClosureParam(name unique.Handle[string], def Expr) *NamedClosureParam {
	return &NamedClosureParam{name: name, def: def}
}

func (n *NamedClosureParam) Name() unique.Handle[string] { return n.name }
func (n *NamedClosureParam) Default() Expr               { return n.def }

type SpreadClosureParam struct {
	ident *Ident
}

func NewSpreadClosureParam(ident *Ident) *SpreadClosureParam {
	return &SpreadClosureParam{ident: ident}
}

func (n *SpreadClosureParam) Ident() *Ident { return n.ident }

func (*PositionalClosureParam) aClosureParam() {}
func (*NamedClosureParam) aClosureParam()      {}
func (*SpreadClosureParam) aClosureParam()     {}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type FuncCall struct {
	expr
	callee Expr
	args   []Arg
	blocks []*ContentBlock
}

func NewFuncCall(span syntax.Span, callee Expr, args []Arg, blocks []*ContentBlock) *FuncCall {
	return &FuncCall{expr: expr{span: span}, callee: callee, args: args, blocks: blocks}
}

func (n *FuncCall) Callee() Expr             { return n.callee }
func (n *FuncCall) Args() []Arg              { return n.args }
func (n *FuncCall) Content() []*ContentBlock { return n.blocks }

type Closure struct {
	expr
	name   *Ident
	params []ClosureParam
	body   Expr
}

func NewClosure(span syntax.Span, name *Ident, params []ClosureParam, body Expr) *Closure {
	return &Closure{expr: expr{span: span}, name: name, params: params, body: body}
}

func (n *Closure) Name() *Ident           { return n.name }
func (n *Closure) Params() []ClosureParam { return n.params }
func (n *Closure) Body() Expr             { return n.body }

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
	conditions []Expr
	blocks     []Expr
	def        Expr
}

func NewConditional(span syntax.Span, conditions []Expr, blocks []Expr, def Expr) *Conditional {
	return &Conditional{expr: expr{span: span}, conditions: conditions, blocks: blocks, def: def}
}

func (n *Conditional) Conditions() []Expr { return n.conditions }
func (n *Conditional) Blocks() []Expr     { return n.blocks }
func (n *Conditional) Default() Expr      { return n.def }

type WhileLoop struct {
	expr
	condition Expr
	body      *CodeBlock
}

func NewWhileLoop(span syntax.Span, condition Expr, body *CodeBlock) *WhileLoop {
	return &WhileLoop{expr: expr{span: span}, condition: condition, body: body}
}

func (n *WhileLoop) Condition() Expr  { return n.condition }
func (n *WhileLoop) Body() *CodeBlock { return n.body }

type ForLoop struct {
	expr
	pattern  []DestructPattern
	iterable Expr
	body     *CodeBlock
}

func NewForLoop(span syntax.Span, pattern []DestructPattern, iterable Expr, body *CodeBlock) *ForLoop {
	return &ForLoop{expr: expr{span: span}, pattern: pattern, iterable: iterable, body: body}
}

func (n *ForLoop) Pattern() []DestructPattern { return n.pattern }
func (n *ForLoop) Iterable() Expr             { return n.iterable }
func (n *ForLoop) Body() *CodeBlock           { return n.body }

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
