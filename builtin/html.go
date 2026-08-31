package builtin

import (
	"strings"

	"znkr.io/writst/internal/names"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

// Html is the `html` module: the way a document writes markup writst has no
// element of its own for.
var Html = &value.Module{
	Name: "html",
	Def: value.SimpleModuleDef{
		names.Elem: HtmlElem,
	},
}

// HtmlElem is an HTML element; it builds a [value.HTMLElem]. The body is
// ordinary content, so what is written inside an element is realized like the
// rest of the document — `html.elem("div")[Some *text*.]` holds a paragraph
// with emphasis in it, not an opaque blob.
//
// Whether the element occupies a line of its own defaults to what the tag
// implies and can be overridden with `block:`. What its body may contain
// cannot: that follows from the tag alone, because it is the tag that decides
// whether a paragraph inside would be legal markup.
var HtmlElem = value.NewElement[*value.HTMLElem](value.Function{
	Name: "html.elem",
	Positional: []value.Param{
		{Name: "tag", Type: types.SetOf(types.Str)},
		{Name: "body", Type: types.SetOf(types.Content, types.None), Default: value.None{}},
	},
	Named: value.NamedParams{
		names.Attrs: {Name: "attrs", Type: types.SetOf(types.Dict), Default: new(value.Dict)},
		names.Block: {Name: "block", Type: types.SetOf(types.Bool, types.Auto), Default: value.Auto{}},
	},
	F: htmlElemImpl,
})

func htmlElemImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	tag := string(args[0].(value.Str))
	if !validHtmlName(tag, false) {
		return nil, value.ArgErrorPosf(0, "not a valid HTML tag name: %q", tag)
	}

	body, _ := args[1].(value.Content)
	if body != nil && value.HtmlTagVoid(tag) {
		return nil, value.ArgErrorPosf(1, "<%s> is a void element and cannot have a body", tag)
	}

	attrs, err := htmlAttrs(named.Get(names.Attrs).(*value.Dict))
	if err != nil {
		return nil, err
	}

	block := value.HtmlTagBlock(tag)
	if b, ok := named.Get(names.Block).(value.Bool); ok {
		block = bool(b)
	}

	return &value.HTMLElem{Tag: tag, Attrs: attrs, Body: body, Block: block}, nil
}

// htmlAttrs validates an attribute dict and returns it, or nil when it is
// empty — an element without attributes carries none rather than an empty dict,
// so that `html.elem("br")` and `html.elem("br", attrs: (:))` are the same
// element.
func htmlAttrs(attrs *value.Dict) (*value.Dict, error) {
	if attrs.Elems.Len() == 0 {
		return nil, nil
	}
	for k, v := range attrs.Elems.All() {
		if !validHtmlName(string(k), true) {
			return nil, value.ArgErrorNamedf(names.Attrs, "not a valid HTML attribute name: %q", string(k))
		}
		if _, ok := v.(value.Str); !ok {
			return nil, value.ArgErrorNamedf(names.Attrs, "attribute %s must be a string, found %s", string(k), v.Type())
		}
	}
	return attrs, nil
}

// validHtmlName reports whether s is usable as a tag or attribute name. Both
// must start with a letter and continue with letters, digits, or `-` (which is
// what makes `my-callout` and `data-foo` work); an attribute name may also use
// `_`, `.` and `:`, which framework attributes lean on.
//
// The check is deliberately narrower than HTML's own grammar. Its purpose is
// that a name written in a document cannot close a tag or open an attribute the
// document did not write, and a name that survives it needs no escaping.
func validHtmlName(s string, attr bool) bool {
	if s == "" || !isASCIILetter(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case isASCIILetter(c) || c >= '0' && c <= '9' || c == '-':
		case attr && strings.IndexByte("_.:", c) >= 0:
		default:
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
