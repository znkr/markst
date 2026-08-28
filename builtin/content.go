package builtin

import (
	"znkr.io/writst/internal/names"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

// The content elements. Each is a [value.Element]: it reports itself as a
// function type (so `type(heading) == function`), acts as a set/show/`.where`
// target, and — being a function — is its own constructor, producing its
// dedicated content type.

// Emph is the emphasis element; it builds a [value.Emph].
var Emph = value.NewElement[*value.Emph](value.Function{
	Name: "emph",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	F: emphImpl,
})

func emphImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	content := args[0].(value.Content)
	return &value.Emph{Body: content}, nil
}

// Table is the table element; it builds a [value.Table] from its content
// children.
var Table = value.NewElement[*value.Table](value.Function{
	Name: "table",
	Positional: []value.Param{
		{Name: "children", Type: types.SetOf(types.Content)},
	},
	Sink: new(0),
	F:    tableImpl,
})

func tableImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	children := make([]value.Content, len(sink.Positional))
	for i, v := range sink.Positional {
		c, err := value.ToContent(v)
		if err != nil {
			return nil, value.ArgErrorPosf(i, "%s", err.Error())
		}
		children[i] = c
	}
	return &value.Table{Children: children}, nil
}

// Heading is the heading element; it builds a [value.Heading].
var Heading = value.NewElement[*value.Heading](value.Function{
	Name: "heading",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	Named: value.NamedParams{
		names.Level: {Name: "level", Type: types.SetOf(types.Int), Default: value.Int(1)},
	},
	F: headingImpl,
})

func headingImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Heading{
		Depth: int(named.Get(names.Level).(value.Int)),
		Body:  args[0].(value.Content),
	}, nil
}

// Text is the text element; it builds a [value.Text].
var Text = value.NewElement[*value.Text](value.Function{
	Name: "text",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Str)},
	},
	F: textImpl,
})

func textImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Text{Text: string(args[0].(value.Str))}, nil
}

// Strong is the strong-emphasis element; it builds a [value.Strong].
var Strong = value.NewElement[*value.Strong](value.Function{
	Name: "strong",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	F: strongImpl,
})

func strongImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Strong{Body: args[0].(value.Content)}, nil
}

// Par is the paragraph element; it builds a [value.Par].
var Par = value.NewElement[*value.Par](value.Function{
	Name: "par",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	F: parImpl,
})

func parImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Par{Body: args[0].(value.Content)}, nil
}

// Raw is inline or block raw text; it builds a [value.Raw].
var Raw = value.NewElement[*value.Raw](value.Function{
	Name: "raw",
	Positional: []value.Param{
		{Name: "text", Type: types.SetOf(types.Str)},
	},
	Named: value.NamedParams{
		names.Block: {Name: "block", Type: types.SetOf(types.Bool), Default: value.Bool(false)},
		names.Lang:  {Name: "lang", Type: types.SetOf(types.Str), Default: value.Str("")},
	},
	F: rawImpl,
})

func rawImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Raw{
		Text:  string(args[0].(value.Str)),
		Block: bool(named.Get(names.Block).(value.Bool)),
		Lang:  string(named.Get(names.Lang).(value.Str)),
	}, nil
}

// SmartQuote is a quotation mark that adapts to its surroundings; it builds a
// [value.SmartQuote]. It also has dedicated syntax: the quote characters
// themselves, ' and ".
var SmartQuote = value.NewElement[*value.SmartQuote](value.Function{
	Name: "smartquote",
	Named: value.NamedParams{
		names.Double: {Name: "double", Type: types.SetOf(types.Bool), Default: value.Bool(true)},
	},
	F: smartQuoteImpl,
})

func smartQuoteImpl(_ *value.FunctionCallContext, _ []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.SmartQuote{Double: bool(named.Get(names.Double).(value.Bool))}, nil
}

// Linebreak forces a line break; it builds a [value.Linebreak].
var Linebreak = value.NewElement[*value.Linebreak](value.Function{
	Name: "linebreak",
	F:    linebreakImpl,
})

func linebreakImpl(_ *value.FunctionCallContext, _ []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Linebreak{}, nil
}

// Parbreak forces a paragraph break; it builds a [value.Parbreak].
var Parbreak = value.NewElement[*value.Parbreak](value.Function{
	Name: "parbreak",
	F:    parbreakImpl,
})

func parbreakImpl(_ *value.FunctionCallContext, _ []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Parbreak{}, nil
}

// Link is a hyperlink; it builds a [value.Link].
var Link = value.NewElement[*value.Link](value.Function{
	Name: "link",
	Positional: []value.Param{
		{Name: "dest", Type: types.SetOf(types.Str)},
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	F: linkImpl,
})

func linkImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Link{Dest: string(args[0].(value.Str)), Body: args[1].(value.Content)}, nil
}

// Ref is a cross-reference to a label; it builds a [value.Ref].
var Ref = value.NewElement[*value.Ref](value.Function{
	Name: "ref",
	Positional: []value.Param{
		{Name: "target", Type: types.SetOf(types.Label)},
	},
	F: refImpl,
})

func refImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Ref{Target: args[0].(*value.Label).Name}, nil
}

// List is a bullet list; it builds a [value.List]. Content children are wrapped
// in list items; `list.item` results are used as-is.
var List = value.NewElement[*value.List](value.Function{
	Name: "list",
	Positional: []value.Param{
		{Name: "children", Type: types.SetOf(types.Content)},
	},
	Sink:  new(0),
	Scope: map[name.Name]value.Value{names.Item: ListItem},
	F:     listImpl,
})

func listImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	children := make([]*value.ListItem, len(sink.Positional))
	for i, v := range sink.Positional {
		if li, ok := v.(*value.ListItem); ok {
			children[i] = li
			continue
		}
		c, err := value.ToContent(v)
		if err != nil {
			return nil, value.ArgErrorPosf(i, "%s", err.Error())
		}
		children[i] = &value.ListItem{Body: c}
	}
	return &value.List{Children: children}, nil
}

// ListItem is a single bullet-list entry (`list.item`); it builds a
// [value.ListItem].
var ListItem = value.NewElement[*value.ListItem](value.Function{
	Name: "list.item",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	F: listItemImpl,
})

func listItemImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.ListItem{Body: args[0].(value.Content)}, nil
}

// Enum is a numbered list; it builds a [value.Enum]. Content children are
// wrapped and auto-numbered; `enum.item` results are used as-is.
var Enum = value.NewElement[*value.Enum](value.Function{
	Name: "enum",
	Positional: []value.Param{
		{Name: "children", Type: types.SetOf(types.Content)},
	},
	Sink:  new(0),
	Scope: map[name.Name]value.Value{names.Item: EnumItem},
	F:     enumImpl,
})

func enumImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	children := make([]*value.EnumItem, len(sink.Positional))
	for i, v := range sink.Positional {
		if ei, ok := v.(*value.EnumItem); ok {
			children[i] = ei
			continue
		}
		c, err := value.ToContent(v)
		if err != nil {
			return nil, value.ArgErrorPosf(i, "%s", err.Error())
		}
		children[i] = &value.EnumItem{Number: i + 1, Body: c}
	}
	return &value.Enum{Children: children}, nil
}

// EnumItem is a single numbered-list entry (`enum.item`); it builds a
// [value.EnumItem].
var EnumItem = value.NewElement[*value.EnumItem](value.Function{
	Name: "enum.item",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	Named: value.NamedParams{
		names.Number: {Name: "number", Type: types.SetOf(types.Int), Default: value.Int(1)},
	},
	F: enumItemImpl,
})

func enumItemImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.EnumItem{
		Number: int(named.Get(names.Number).(value.Int)),
		Body:   args[0].(value.Content),
	}, nil
}

// Terms is a term/description list; it builds a [value.Terms] from `terms.item`
// children.
var Terms = value.NewElement[*value.Terms](value.Function{
	Name: "terms",
	Positional: []value.Param{
		{Name: "children", Type: types.SetOf(types.Content)},
	},
	Sink:  new(0),
	Scope: map[name.Name]value.Value{names.Item: TermItem},
	F:     termsImpl,
})

func termsImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	children := make([]*value.TermItem, len(sink.Positional))
	for i, v := range sink.Positional {
		ti, ok := v.(*value.TermItem)
		if !ok {
			return nil, value.ArgErrorPosf(i, "expected a term item, found %s", v.Type())
		}
		children[i] = ti
	}
	return &value.Terms{Children: children}, nil
}

// TermItem is a single term/description entry (`terms.item`); it builds a
// [value.TermItem].
var TermItem = value.NewElement[*value.TermItem](value.Function{
	Name: "terms.item",
	Positional: []value.Param{
		{Name: "term", Type: types.SetOf(types.Content)},
		{Name: "description", Type: types.SetOf(types.Content)},
	},
	F: termItemImpl,
})

func termItemImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.TermItem{
		Term:        args[0].(value.Content),
		Description: args[1].(value.Content),
	}, nil
}

// Document is the document root's set target: `#set document(title: …)`
// configures the realized [value.Document]. It has no constructor (F == nil) —
// the root is assembled by the realization pass, not called — so `#document(…)`
// reports "not callable".
var Document = value.NewElement[*value.Document](value.Function{
	Name: "document",
	Named: value.NamedParams{
		names.Title: {Name: "title", Type: types.Any},
	},
})
