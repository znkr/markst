package builtin

import (
	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// The math font-style functions: `bold(x)`, `sans(x)`, `bb(x)`, … Each applies a
// font style to everything in its body.
//
// These are plain functions rather than elements, mirroring Typst, where a style
// function is a scoped `set` on the equation element rather than an element of
// its own. Each returns a [value.Styled] recording that set; the realization
// pass folds it into the math leaves it reaches (see session.applySet).
var (
	Bold    = mathStyle("math.bold", names.Bold, value.Bool(true))
	Italic  = mathStyle("math.italic", names.Italic, value.Bool(true))
	Upright = mathStyle("math.upright", names.Italic, value.Bool(false))
	Serif   = mathStyle("math.serif", names.Variant, value.Str("serif"))
	Sans    = mathStyle("math.sans", names.Variant, value.Str("sans"))
	Cal     = mathStyle("math.cal", names.Variant, value.Str("cal"))
	Frak    = mathStyle("math.frak", names.Variant, value.Str("frak"))
	Mono    = mathStyle("math.mono", names.Variant, value.Str("mono"))
	Bb      = mathStyle("math.bb", names.Variant, value.Str("bb"))
)

// mathStyleFields are the equation properties the style functions set. Listing
// them here keeps the realization pass (which folds exactly these into math
// leaves) and the functions that record them in agreement.
var mathStyleFields = []name.Name{names.Bold, names.Italic, names.Variant}

// mathStyle builds one math font-style function: it wraps its body in a scope
// that sets field to val on math.equation.
func mathStyle(fname string, field name.Name, val value.Value) *value.Function {
	return &value.Function{
		Name:       fname,
		Positional: []value.Param{{Name: "body", Type: mathContentType}},
		F: func(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
			body, err := mathContent(args[0])
			if err != nil {
				return nil, value.ArgErrorPosf(0, "%s", err.Error())
			}
			set := &value.Set{Element: Equation}
			set.Fields.Put(field, val)
			return &value.Styled{Sets: []*value.Set{set}, Body: body}, nil
		},
	}
}
