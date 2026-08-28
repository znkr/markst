package expr

import (
	"fmt"
	"strconv"
	"strings"

	"znkr.io/writst/value"
)

// FormatModule returns a textual SSA dump of mod. The format is informally
// modelled on LLVM IR / Cranelift CLIF: each [Function] becomes a labelled
// block, with one [BasicBlock] per `bN:` (or `bN(vX, vY):`) section.
// Instructions print as `vN = <opcode> <operands…>`. Block parameters
// appear in the header; terminators carry args targeting the successor's
// parameters (e.g. `jump b1(v0)`, `branch v, b1(v2), b2(v3, v4)`).
//
// This is the format consumed by analyzer golden tests under the new IR. It
// is not pretty-printed writst source — see the migration plan for why.
func FormatModule(mod *Module) string {
	f := formatter{mod: mod}
	f.formatFunction("$top", mod.Top)
	for i, fn := range mod.Functions {
		f.sb.WriteString("\n")
		f.formatFunction(fmt.Sprintf("$fn_%d", i), fn)
	}
	return f.sb.String()
}

// FormatFunction returns the SSA dump of a single function with the given
// label (e.g. "$top", "$fn_0"). Module-constant refs render as `c<id>` since
// the pool isn't reachable without the enclosing module; use [FormatModule]
// for inline-literal rendering.
func FormatFunction(label string, fn *Function) string {
	f := formatter{}
	f.formatFunction(label, fn)
	return f.sb.String()
}

// formatter carries the rendering state for one FormatModule/FormatFunction
// call. mod is nil when only a single function is being formatted; in that
// case module-constant refs render as `c<id>` rather than as inline literals.
type formatter struct {
	sb  strings.Builder
	mod *Module
}

func (f *formatter) formatFunction(label string, fn *Function) {
	sb := &f.sb
	sb.WriteString("fn ")
	sb.WriteString(label)
	if fn.Name != "" {
		fmt.Fprintf(sb, " name=%q", fn.Name)
	}
	if len(fn.Captures) > 0 {
		sb.WriteString(" captures=[")
		for i, r := range fn.Captures {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.ref(r))
		}
		sb.WriteString("]")
	}
	if fn.SelfRef != NoRef {
		fmt.Fprintf(sb, " self=%s", f.ref(fn.SelfRef))
	}
	if len(fn.Params) > 0 {
		sb.WriteString(" params=[")
		for i, p := range fn.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			switch p.Kind {
			case ParamSink:
				fmt.Fprintf(sb, "%s=..%s", f.ref(p.Ref), p.Name.String())
			case ParamNamed:
				fmt.Fprintf(sb, "%s=%s", f.ref(p.Ref), p.Name.String())
				if p.Default != NoRef {
					fmt.Fprintf(sb, ":%s", f.ref(p.Default))
				}
			default:
				fmt.Fprintf(sb, "%s=%s", f.ref(p.Ref), p.Name.String())
			}
		}
		sb.WriteString("]")
	}
	sb.WriteString(":\n")
	for _, block := range fn.Blocks {
		f.formatBlock(block)
	}
}

func (f *formatter) formatBlock(block *BasicBlock) {
	sb := &f.sb
	sb.WriteString("  ")
	sb.WriteString(formatBlockID(block.ID))
	if len(block.Params) > 0 {
		sb.WriteString("(")
		for i, p := range block.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.ref(p.Result()))
		}
		sb.WriteString(")")
	}
	sb.WriteString(":\n")
	for _, inst := range block.Instrs {
		sb.WriteString("    ")
		f.formatInst(inst)
		sb.WriteString("\n")
	}
	if block.Term != nil {
		sb.WriteString("    ")
		f.formatTerm(block.Term)
		sb.WriteString("\n")
	}
}

func (f *formatter) formatInst(inst Instruction) {
	sb := &f.sb
	if r := inst.Result(); r != NoRef {
		sb.WriteString(f.ref(r))
		sb.WriteString(" = ")
	}
	switch i := inst.(type) {
	case *Const:
		sb.WriteString("const ")
		sb.WriteString(formatConst(i.Value))
	case *Unary:
		fmt.Fprintf(sb, "%s %s", i.Op.String(), f.ref(i.X))
	case *Binary:
		fmt.Fprintf(sb, "%s %s %s", f.ref(i.L), i.Op.String(), f.ref(i.R))
	case *MakeArray:
		sb.WriteString("make_array [")
		for j, item := range i.Items {
			if j > 0 {
				sb.WriteString(", ")
			}
			if item.Spread {
				sb.WriteString("..")
			}
			sb.WriteString(f.ref(item.Value))
		}
		sb.WriteString("]")
	case *MakeDict:
		sb.WriteString("make_dict (")
		for j, e := range i.Entries {
			if j > 0 {
				sb.WriteString(", ")
			}
			switch {
			case e.Spread:
				sb.WriteString("..")
				sb.WriteString(f.ref(e.Value))
			default:
				sb.WriteString(f.ref(e.Key))
				sb.WriteString(": ")
				sb.WriteString(f.ref(e.Value))
			}
		}
		sb.WriteString(")")
	case *FieldRead:
		fmt.Fprintf(sb, "field_read %s.%s", f.ref(i.Target), i.Field.String())
	case *MethodField:
		fmt.Fprintf(sb, "method_field %s.%s", f.ref(i.Target), i.Field.String())
	case *Call:
		sb.WriteString("call ")
		sb.WriteString(f.ref(i.Callee.Ref))
		sb.WriteString("(")
		for j, a := range i.Args {
			if j > 0 {
				sb.WriteString(", ")
			}
			switch a.Kind {
			case ArgSpread:
				sb.WriteString("..")
				sb.WriteString(f.ref(a.Value))
			case ArgNamed:
				sb.WriteString(a.Name.String())
				sb.WriteString(": ")
				sb.WriteString(f.ref(a.Value))
			default:
				sb.WriteString(f.ref(a.Value))
			}
		}
		sb.WriteString(")")
		for _, blk := range i.Blocks {
			sb.WriteString("[")
			sb.WriteString(f.ref(blk))
			sb.WriteString("]")
		}
		if i.AllowSetter {
			sb.WriteString(" lvalue")
		}
		if i.Mut != nil {
			switch {
			case i.Mut.RecvTemporary:
				sb.WriteString(" mut(temp)")
			case len(i.Mut.RecvAccessors) > 0:
				sb.WriteString(" mut(place if")
				for _, a := range i.Mut.RecvAccessors {
					sb.WriteString(" ")
					sb.WriteString(f.ref(a))
				}
				sb.WriteString(" accessors)")
			default:
				sb.WriteString(" mut(place)")
			}
		}
	case *DestructArray:
		fmt.Fprintf(sb, "destruct_array %s, before=%d, after=%d", f.ref(i.Source), i.Before, i.After)
		if i.HasSink {
			sb.WriteString(", has_sink")
		}
	case *ArrayElem:
		if i.FromEnd {
			fmt.Fprintf(sb, "array_elem %s[-%d]", f.ref(i.Source), i.Index+1)
		} else {
			fmt.Fprintf(sb, "array_elem %s[%d]", f.ref(i.Source), i.Index)
		}
	case *ArraySlice:
		fmt.Fprintf(sb, "array_slice %s[%d:len-%d]", f.ref(i.Source), i.Before, i.After)
	case *DestructDict:
		fmt.Fprintf(sb, "destruct_dict %s, consumed=[", f.ref(i.Source))
		for j, k := range i.Consumed {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(k.String())
		}
		sb.WriteString("]")
		if i.HasSink {
			sb.WriteString(", has_sink")
		}
	case *DictField:
		fmt.Fprintf(sb, "dict_field %s.%s", f.ref(i.Source), i.Field.String())
	case *DictRest:
		fmt.Fprintf(sb, "dict_rest %s, consumed=[", f.ref(i.Source))
		for j, k := range i.Consumed {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(k.String())
		}
		sb.WriteString("]")
	case *IterOpen:
		fmt.Fprintf(sb, "iter_open %s", f.ref(i.Iterable))
	case *IterHasNext:
		fmt.Fprintf(sb, "iter_has_next %s", f.ref(i.Iter))
	case *IterAdvance:
		fmt.Fprintf(sb, "iter_advance %s", f.ref(i.Iter))
	case *MakeClosure:
		fmt.Fprintf(sb, "make_closure $fn_%d", i.Func)
		if len(i.Captures) > 0 {
			sb.WriteString(" captures=[")
			for j, c := range i.Captures {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(f.ref(c))
			}
			sb.WriteString("]")
		}
	case *ContentResult:
		if i.Math {
			sb.WriteString("content_result math [")
		} else {
			sb.WriteString("content_result [")
		}
		for j, r := range i.Items {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.ref(r))
		}
		sb.WriteString("]")
	case *AttachLabel:
		fmt.Fprintf(sb, "attach_label %s, @%s", f.ref(i.Content), i.Label.String())
	case *CodeJoin:
		sb.WriteString("code_join [")
		for j, r := range i.Items {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.ref(r))
		}
		sb.WriteString("]")
	case *JoinBegin:
		sb.WriteString("join_begin")
	case *JoinAdd:
		fmt.Fprintf(sb, "join_add %s, %s", f.ref(i.Acc), f.ref(i.Item))
	case *JoinResult:
		fmt.Fprintf(sb, "join_result %s", f.ref(i.Acc))
	case *Heading:
		fmt.Fprintf(sb, "heading level=%d %s", i.Level, f.ref(i.Body))
	case *Strong:
		fmt.Fprintf(sb, "strong %s", f.ref(i.Body))
	case *Emph:
		fmt.Fprintf(sb, "emph %s", f.ref(i.Body))
	case *Link:
		fmt.Fprintf(sb, "link %q %s", i.Dest, f.ref(i.Body))
	case *RefMarkup:
		fmt.Fprintf(sb, "ref @%s", i.Target.String())
		if i.Supplement != NoRef {
			fmt.Fprintf(sb, " %s", f.ref(i.Supplement))
		}
	case *ListItem:
		fmt.Fprintf(sb, "list_item %s", f.ref(i.Body))
	case *EnumItem:
		if i.Number < 0 {
			fmt.Fprintf(sb, "enum_item %s", f.ref(i.Body))
		} else {
			fmt.Fprintf(sb, "enum_item number=%d %s", i.Number, f.ref(i.Body))
		}
	case *TermItem:
		fmt.Fprintf(sb, "term_item %s, %s", f.ref(i.Term), f.ref(i.Description))
	case *Equation:
		if i.Block {
			fmt.Fprintf(sb, "equation block %s", f.ref(i.Body))
		} else {
			fmt.Fprintf(sb, "equation %s", f.ref(i.Body))
		}
	case *MathAttach:
		fmt.Fprintf(sb, "math_attach %s", f.ref(i.Base))
		if i.Top != NoRef {
			fmt.Fprintf(sb, " top=%s", f.ref(i.Top))
		}
		if i.Bottom != NoRef {
			fmt.Fprintf(sb, " bottom=%s", f.ref(i.Bottom))
		}
	case *MathFrac:
		fmt.Fprintf(sb, "math_frac %s, %s", f.ref(i.Num), f.ref(i.Denom))
	case *MathRoot:
		if i.Index != NoRef {
			fmt.Fprintf(sb, "math_root index=%s %s", f.ref(i.Index), f.ref(i.Radicand))
		} else {
			fmt.Fprintf(sb, "math_root %s", f.ref(i.Radicand))
		}
	case *MathPrimes:
		fmt.Fprintf(sb, "math_primes count=%d %s", i.Count, f.ref(i.Base))
	case *MathDelimited:
		fmt.Fprintf(sb, "math_delimited %s, %s, %s", f.ref(i.Open), f.ref(i.Body), f.ref(i.Close))
	case *SetRule:
		fmt.Fprintf(sb, "set_rule %s", f.ref(i.Target))
	case *ShowRule:
		sb.WriteString("show_rule")
		if i.Selector != NoRef {
			fmt.Fprintf(sb, " %s", f.ref(i.Selector))
		}
		fmt.Fprintf(sb, ": %s", f.ref(i.Transform))
	case *DiscardCheck:
		fmt.Fprintf(sb, "discard_check %s", f.ref(i.Value))
	case *Warn:
		fmt.Fprintf(sb, "warn msg=%q", i.Msg)
		for _, h := range i.Hints {
			fmt.Fprintf(sb, " hint=%q", h)
		}
	case *Contextual:
		fmt.Fprintf(sb, "contextual %s", f.ref(i.Body))
	case *ModuleInclude:
		fmt.Fprintf(sb, "module_include %s", f.ref(i.Source))
	case *Error:
		fmt.Fprintf(sb, "error msg=%q", i.Msg)
		if i.From != NoRef {
			fmt.Fprintf(sb, " from=%s", f.ref(i.From))
		}
		for _, h := range i.Hints {
			fmt.Fprintf(sb, " hint=%q", h)
		}
	default:
		fmt.Fprintf(sb, "%T", inst)
	}
}

func (f *formatter) formatTerm(t Terminator) {
	sb := &f.sb
	switch t := t.(type) {
	case *Jump:
		sb.WriteString("jump ")
		sb.WriteString(formatBlockID(t.Target))
		f.formatArgs(t.Args)
	case *Branch:
		sb.WriteString("branch ")
		sb.WriteString(f.ref(t.Cond))
		sb.WriteString(", ")
		sb.WriteString(formatBlockID(t.Then))
		f.formatArgs(t.ThenArgs)
		sb.WriteString(", ")
		sb.WriteString(formatBlockID(t.Else))
		f.formatArgs(t.ElseArgs)
	case *Return:
		sb.WriteString("return")
		if t.Value != NoRef {
			sb.WriteString(" ")
			sb.WriteString(f.ref(t.Value))
		}
	case *Unreachable:
		sb.WriteString("unreachable")
	default:
		fmt.Fprintf(sb, "%T", t)
	}
}

// formatArgs appends a parenthesized arg list to sb. Emits nothing when
// args is empty, so blocks with no params stay readable.
func (f *formatter) formatArgs(args []Ref) {
	if len(args) == 0 {
		return
	}
	sb := &f.sb
	sb.WriteString("(")
	for i, a := range args {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(f.ref(a))
	}
	sb.WriteString(")")
}

// ref renders r as an SSA-ref string. Module-constant refs render as the
// inline constant literal when the formatter has the enclosing module
// available; otherwise as `c<id>`. Function-local refs render as `vN`.
func (f *formatter) ref(r Ref) string {
	if r == NoRef {
		return "<noref>"
	}
	if r.IsModConst() {
		id := r.ModConstID()
		if f.mod != nil && int(id) < len(f.mod.Constants) {
			return formatConst(f.mod.Constants[id])
		}
		return "c" + strconv.Itoa(int(id))
	}
	return "v" + strconv.Itoa(int(r))
}

func formatBlockID(id BlockID) string {
	return "b" + strconv.Itoa(int(id))
}

// formatConst prints a constant value in a stable form for golden tests.
func formatConst(v any) string {
	switch v := v.(type) {
	case nil:
		return "<nil>"
	case *value.Text:
		return fmt.Sprintf("text %q", v.Text)
	case *value.Linebreak:
		return "linebreak"
	case *value.Parbreak:
		return "parbreak"
	case *value.SmartQuote:
		return fmt.Sprintf("smartquote double=%v", v.Double)
	case *value.Label:
		return fmt.Sprintf("label %q", v.Name.String())
	case *value.Raw:
		if v.Block {
			return fmt.Sprintf("raw_block lang=%q %q", v.Lang, v.Text)
		}
		return fmt.Sprintf("raw_inline %q", v.Text)
	case *value.Function:
		if v.Name != "" {
			return "<function " + v.Name + ">"
		}
		return "<function>"
	case *value.Element:
		if v.Name != "" {
			return "<function " + v.Name + ">"
		}
		return "<function>"
	case *value.MathText:
		return fmt.Sprintf("math_text %q", v.Text)
	case *value.MathAlignPoint:
		return "math_align_point"
	case *value.Type:
		return "<type " + v.Reflected.String() + ">"
	case *value.Module:
		return "<module " + v.Name + ">"
	case fmt.Stringer:
		return v.String()
	}
	return fmt.Sprintf("%v", v)
}
