package expr

import (
	"fmt"
	"strconv"
	"strings"

	"znkr.io/writst/value"
)

// FormatModule returns a textual SSA dump of mod. The format is informally
// modelled on LLVM IR / Cranelift CLIF: each [Function] becomes a labelled
// block, with one [BasicBlock] per `bN:` section. Instructions print as
// `vN = <opcode> <operands…>`; phi nodes appear at the top of their block.
//
// This is the format consumed by analyzer golden tests under the new IR. It
// is not pretty-printed writst source — see the migration plan for why.
func FormatModule(mod *Module) string {
	var sb strings.Builder
	formatFunction(&sb, "$top", mod.Top)
	for i, fn := range mod.Functions {
		sb.WriteString("\n")
		formatFunction(&sb, fmt.Sprintf("$fn_%d", i), fn)
	}
	return sb.String()
}

// FormatFunction returns the SSA dump of a single function with the given
// label (e.g. "$top", "$fn_0").
func FormatFunction(label string, fn *Function) string {
	var sb strings.Builder
	formatFunction(&sb, label, fn)
	return sb.String()
}

func formatFunction(sb *strings.Builder, label string, fn *Function) {
	sb.WriteString("fn ")
	sb.WriteString(label)
	if fn.Name != "" {
		fmt.Fprintf(sb, " name=%q", fn.Name)
	}
	// Build reverse lookups so each capture / param entry shows the Ref
	// the body references.
	captureRefs := make([]Ref, len(fn.Captures))
	paramRefs := make([]Ref, len(fn.Params))
	selfRef := NoRef
	for i, d := range fn.Defs {
		switch d := d.(type) {
		case *DefCapture:
			captureRefs[d.Idx] = Ref(i)
		case *DefParam:
			paramRefs[d.Idx] = Ref(i)
		case *DefSelf:
			selfRef = Ref(i)
		}
	}
	if len(fn.Captures) > 0 {
		sb.WriteString(" captures=[")
		for i, c := range fn.Captures {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(sb, "%s=%s", formatRef(captureRefs[i]), c.String())
		}
		sb.WriteString("]")
	}
	if selfRef != NoRef {
		fmt.Fprintf(sb, " self=%s", formatRef(selfRef))
	}
	if len(fn.Params) > 0 {
		sb.WriteString(" params=[")
		for i, p := range fn.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			switch p.Kind {
			case ParamSink:
				fmt.Fprintf(sb, "%s=..%s", formatRef(paramRefs[i]), p.Name.String())
			case ParamNamed:
				fmt.Fprintf(sb, "%s=%s", formatRef(paramRefs[i]), p.Name.String())
				if p.Default != NoRef {
					fmt.Fprintf(sb, ":%s", formatRef(p.Default))
				}
			default:
				fmt.Fprintf(sb, "%s=%s", formatRef(paramRefs[i]), p.Name.String())
			}
		}
		sb.WriteString("]")
	}
	sb.WriteString(":\n")
	for _, block := range fn.Blocks {
		formatBlock(sb, block)
	}
}

func formatBlock(sb *strings.Builder, block *BasicBlock) {
	sb.WriteString("  ")
	sb.WriteString(formatBlockID(block.ID))
	sb.WriteString(":\n")
	for _, phi := range block.Phis {
		sb.WriteString("    ")
		sb.WriteString(formatRef(phi.result))
		sb.WriteString(" = phi [")
		for i, op := range phi.operands {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(formatBlockID(op.Pred))
			sb.WriteString(" ")
			sb.WriteString(formatRef(op.Value))
		}
		sb.WriteString("]\n")
	}
	for _, inst := range block.Instrs {
		sb.WriteString("    ")
		formatInst(sb, inst)
		sb.WriteString("\n")
	}
	if block.Term != nil {
		sb.WriteString("    ")
		formatTerm(sb, block.Term)
		sb.WriteString("\n")
	}
}

func formatInst(sb *strings.Builder, inst Instruction) {
	if r := inst.Result(); r != NoRef {
		sb.WriteString(formatRef(r))
		sb.WriteString(" = ")
	}
	switch i := inst.(type) {
	case *Const:
		sb.WriteString("const ")
		sb.WriteString(formatConst(i.Value))
	case *Unary:
		fmt.Fprintf(sb, "%s %s", i.Op.String(), formatRef(i.X))
	case *Binary:
		fmt.Fprintf(sb, "%s %s %s", formatRef(i.L), i.Op.String(), formatRef(i.R))
	case *MakeArray:
		sb.WriteString("make_array [")
		for j, item := range i.Items {
			if j > 0 {
				sb.WriteString(", ")
			}
			if item.Spread {
				sb.WriteString("..")
			}
			sb.WriteString(formatRef(item.Value))
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
				sb.WriteString(formatRef(e.Value))
			default:
				sb.WriteString(formatRef(e.Key))
				sb.WriteString(": ")
				sb.WriteString(formatRef(e.Value))
			}
		}
		sb.WriteString(")")
	case *FieldRead:
		fmt.Fprintf(sb, "field_read %s.%s", formatRef(i.Target), i.Field.String())
	case *Call:
		sb.WriteString("call ")
		sb.WriteString(formatRef(i.Callee))
		sb.WriteString("(")
		for j, a := range i.Args {
			if j > 0 {
				sb.WriteString(", ")
			}
			switch a.Kind {
			case ArgSpread:
				sb.WriteString("..")
				sb.WriteString(formatRef(a.Value))
			case ArgNamed:
				sb.WriteString(a.Name.String())
				sb.WriteString(": ")
				sb.WriteString(formatRef(a.Value))
			default:
				sb.WriteString(formatRef(a.Value))
			}
		}
		sb.WriteString(")")
		for _, blk := range i.Blocks {
			sb.WriteString("[")
			sb.WriteString(formatRef(blk))
			sb.WriteString("]")
		}
		if i.AllowSetter {
			sb.WriteString(" lvalue")
		}
	case *Extract:
		fmt.Fprintf(sb, "extract %s[%d]", formatRef(i.Source), i.Index)
	case *LengthCheck:
		fmt.Fprintf(sb, "length_check %s, want=%d", formatRef(i.Source), i.Want)
		if i.HasSink {
			sb.WriteString(", has_sink")
		}
	case *IterOpen:
		fmt.Fprintf(sb, "iter_open %s", formatRef(i.Iterable))
	case *IterHasNext:
		fmt.Fprintf(sb, "iter_has_next %s", formatRef(i.Iter))
	case *IterAdvance:
		fmt.Fprintf(sb, "iter_advance %s", formatRef(i.Iter))
	case *MakeClosure:
		fmt.Fprintf(sb, "make_closure $fn_%d", i.Func)
		if len(i.Captures) > 0 {
			sb.WriteString(" captures=[")
			for j, c := range i.Captures {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(formatRef(c))
			}
			sb.WriteString("]")
		}
	case *ContentResult:
		sb.WriteString("content_result [")
		for j, r := range i.Items {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(formatRef(r))
		}
		sb.WriteString("]")
	case *AttachLabel:
		fmt.Fprintf(sb, "attach_label %s, @%s", formatRef(i.Content), i.Label.String())
	case *CodeJoin:
		sb.WriteString("code_join [")
		for j, r := range i.Items {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(formatRef(r))
		}
		sb.WriteString("]")
	case *LoopAccBegin:
		sb.WriteString("loop_acc_begin")
	case *LoopAccAdd:
		fmt.Fprintf(sb, "loop_acc_add %s, %s", formatRef(i.Acc), formatRef(i.Item))
	case *LoopAccResult:
		fmt.Fprintf(sb, "loop_acc_result %s", formatRef(i.Acc))
	case *Heading:
		fmt.Fprintf(sb, "heading level=%d %s", i.Level, formatRef(i.Body))
	case *Strong:
		fmt.Fprintf(sb, "strong %s", formatRef(i.Body))
	case *Emph:
		fmt.Fprintf(sb, "emph %s", formatRef(i.Body))
	case *Link:
		fmt.Fprintf(sb, "link %q %s", i.Dest, formatRef(i.Body))
	case *RefMarkup:
		fmt.Fprintf(sb, "ref @%s", i.Target.String())
		if i.Supplement != NoRef {
			fmt.Fprintf(sb, " %s", formatRef(i.Supplement))
		}
	case *ListItem:
		fmt.Fprintf(sb, "list_item %s", formatRef(i.Body))
	case *EnumItem:
		if i.Number < 0 {
			fmt.Fprintf(sb, "enum_item %s", formatRef(i.Body))
		} else {
			fmt.Fprintf(sb, "enum_item number=%d %s", i.Number, formatRef(i.Body))
		}
	case *TermItem:
		fmt.Fprintf(sb, "term_item %s, %s", formatRef(i.Term), formatRef(i.Description))
	case *SetRule:
		fmt.Fprintf(sb, "set_rule %s", formatRef(i.Target))
	case *ShowRule:
		fmt.Fprintf(sb, "show_rule")
		if i.Selector != NoRef {
			fmt.Fprintf(sb, " %s", formatRef(i.Selector))
		}
		fmt.Fprintf(sb, ": %s", formatRef(i.Transform))
	case *Contextual:
		fmt.Fprintf(sb, "contextual %s", formatRef(i.Body))
	case *ModuleInclude:
		fmt.Fprintf(sb, "module_include %s", formatRef(i.Source))
	default:
		fmt.Fprintf(sb, "%T", inst)
	}
}

func formatTerm(sb *strings.Builder, t Terminator) {
	switch t := t.(type) {
	case *Jump:
		sb.WriteString("jump ")
		sb.WriteString(formatBlockID(t.Target))
	case *Branch:
		sb.WriteString("branch ")
		sb.WriteString(formatRef(t.Cond))
		sb.WriteString(", ")
		sb.WriteString(formatBlockID(t.Then))
		sb.WriteString(", ")
		sb.WriteString(formatBlockID(t.Else))
	case *Return:
		sb.WriteString("return")
		if t.Value != NoRef {
			sb.WriteString(" ")
			sb.WriteString(formatRef(t.Value))
		}
	case *Unreachable:
		sb.WriteString("unreachable")
	default:
		fmt.Fprintf(sb, "%T", t)
	}
}

func formatRef(r Ref) string {
	if r == NoRef {
		return "<noref>"
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
	case *value.Label:
		return fmt.Sprintf("label %q", v.Name.String())
	case *value.Raw:
		if v.Block {
			return fmt.Sprintf("raw_block lang=%q lines=%d", v.Lang, len(v.Lines))
		}
		return fmt.Sprintf("raw_inline lines=%d", len(v.Lines))
	case *value.Function:
		if v.Name != "" {
			return "<function " + v.Name + ">"
		}
		return "<function>"
	case *value.Type:
		return "<type " + v.Reflected.String() + ">"
	case *value.Module:
		return "<module " + v.Name + ">"
	case fmt.Stringer:
		return v.String()
	}
	return fmt.Sprintf("%v", v)
}
