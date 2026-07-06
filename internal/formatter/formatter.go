package formatter

import (
	"fmt"
	"strings"

	"znkr.io/writst/syntax"
)

type Formattable interface {
	Format(f *Formatter)
}

// Formatter provides a simple DSL for formatting IR nodes.
type Formatter struct {
	sb         *strings.Builder
	mode       syntax.Mode
	indent     int
	inlineCode bool // true when formatting inline code like { expr }
}

func New(mode syntax.Mode) *Formatter {
	return &Formatter{sb: new(strings.Builder), mode: mode}
}

func (f *Formatter) Len() int       { return f.sb.Len() }
func (f *Formatter) String() string { return f.sb.String() }

func (f *Formatter) Mode() syntax.Mode        { return f.mode }
func (f *Formatter) SetMode(mode syntax.Mode) { f.mode = mode }

func (f *Formatter) IncreaseIndent() { f.indent++ }
func (f *Formatter) DecreaseIndent() { f.indent-- }

func (f *Formatter) Inline(fn func(f *Formatter)) {
	prev := f.inlineCode
	f.inlineCode = true
	fn(f)
	f.inlineCode = prev
}

func (f *Formatter) Str(s string)                   { f.sb.WriteString(s) }
func (f *Formatter) Linebreak()                     { f.Str("\n"); f.Indent() }
func (f *Formatter) Indent()                        { f.Str(strings.Repeat("  ", f.indent)) }
func (f *Formatter) Print(v any)                    { fmt.Fprint(f.sb, v) }
func (f *Formatter) Printf(format string, a ...any) { fmt.Fprintf(f.sb, format, a...) }

// Prefix writes # when in markup mode (transitioning into code)
func (f *Formatter) Prefix() {
	if f.mode == syntax.ModeMarkup {
		f.Str("#")
	}
}

// Keyword writes a keyword followed by a space
func (f *Formatter) Keyword(s string) {
	f.Str(s)
	f.Str(" ")
}

type Arg struct {
	name       string
	positional bool
	value      any
}

func NamedArg(name string, v any) Arg { return Arg{name: name, value: v} }
func PositionalArg(v any) Arg         { return Arg{positional: true, value: v} }

// format writes the argument (with its `name: ` prefix, if named) to f.
func (a Arg) format(f *Formatter) {
	if !a.positional {
		f.Str(a.name)
		f.Str(": ")
	}
	switch v := a.value.(type) {
	case Formattable:
		v.Format(f)
	case string:
		f.Printf("%q", v)
	default:
		f.Print(v)
	}
}

// FuncCall formats #name(args)[blocks...] for content nodes
func (f *Formatter) FuncCall(name string, args []Arg, blocks ...Formattable) {
	f.Prefix()
	f.Str(name)
	if len(args) > 0 {
		// Pre-render each argument at the nested indent. An argument that spans
		// multiple lines forces block layout so the nesting stays legible: the
		// call breaks across lines and every continuation is indented one level.
		rendered := make([]string, len(args))
		multiline := false
		for i, a := range args {
			sub := &Formatter{sb: new(strings.Builder), mode: f.mode, indent: f.indent + 1, inlineCode: f.inlineCode}
			a.format(sub)
			rendered[i] = sub.String()
			if strings.Contains(rendered[i], "\n") {
				multiline = true
			}
		}
		inline := len(args) <= 2 && !multiline
		f.Str("(")
		if inline {
			for i, s := range rendered {
				if i > 0 {
					f.Str(", ")
				}
				f.Str(s)
			}
		} else {
			f.indent++
			for _, s := range rendered {
				f.Linebreak()
				f.Str(s)
				f.Str(",")
			}
			f.indent--
			f.Linebreak()
		}
		f.Str(")")
	}
	for _, block := range blocks {
		block.Format(f)
	}
	if len(args) == 0 && len(blocks) == 0 {
		f.Str("()")
	}
}
