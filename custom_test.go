package markst_test

import (
	"strings"
	"testing"

	"znkr.io/markst"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// payload is what a host puts inside a [value.Custom]: a Go value markst knows
// nothing about and hands straight back to whatever presents the document.
type payload struct{ path string }

// customBindings supplies `block-elem` and `inline-elem`, both of which take a
// path and wrap it in a Custom. They stand in for a host's real directives.
func customBindings() map[name.Name]value.Value {
	elem := func(elemName string, block bool) *value.Function {
		return &value.Function{
			Name:       elemName,
			Positional: []value.Param{{Name: "path", Type: types.SetOf(types.Str)}},
			F: func(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
				p := string(args[0].(value.Str))
				if !strings.HasSuffix(p, ".go") {
					return nil, value.ArgErrorPosf(0, "not a Go file: %s", p)
				}
				return &value.Custom{Elem: elemName, Block: block, Value: &payload{path: p}}, nil
			},
		}
	}
	return map[name.Name]value.Value{
		name.Make("block-elem"):  elem("block-elem", true),
		name.Make("inline-elem"): elem("inline-elem", false),
	}
}

// findCustom returns the single Custom in doc.
func findCustom(t *testing.T, doc *value.Document) *value.Custom {
	t.Helper()
	var found *value.Custom
	for c := range value.Preorder(doc, value.SetOf(value.KindCustom)) {
		if found != nil {
			t.Fatalf("document has more than one Custom element")
		}
		found = c.Node().(*value.Custom)
	}
	if found == nil {
		t.Fatalf("document has no Custom element:\n%s", value.FormatContent(doc))
	}
	return found
}

// inParagraph reports whether c ended up inside one of doc's paragraphs, which
// is the whole of what block-ness decides for a leaf element.
func inParagraph(doc *value.Document, c *value.Custom) bool {
	for e := range value.Preorder(doc, value.SetOf(value.KindCustom)) {
		if e.Node() != value.Content(c) {
			continue
		}
		for range e.Enclosing(value.SetOf(value.KindPar)) {
			return true
		}
	}
	return false
}

// TestCustomBlockStaysOutOfParagraphs is what the whole feature rests on: a
// block-level host element is a block like any other, so realization puts it
// beside the paragraphs around it rather than inside one.
func TestCustomBlockStaysOutOfParagraphs(t *testing.T) {
	doc := compile(t, `Before.

#block-elem("diff.go")

After.
`, markst.WithBindings(customBindings()))

	custom := findCustom(t, doc)
	if inParagraph(doc, custom) {
		t.Errorf("block Custom was wrapped in a paragraph:\n%s", value.FormatContent(doc))
	}
	if got := custom.Value.(*payload).path; got != "diff.go" {
		t.Errorf("payload.path = %q, want %q — the payload did not survive the pipeline", got, "diff.go")
	}
	if custom.Elem != "block-elem" || !custom.IsBlock() {
		t.Errorf("Custom = %+v, want block-elem, block", custom)
	}
}

// TestCustomInlineJoinsTheParagraph is the other half: an inline host element
// shares its paragraph with the text around it.
func TestCustomInlineJoinsTheParagraph(t *testing.T) {
	doc := compile(t, `Before #inline-elem("diff.go") after.
`, markst.WithBindings(customBindings()))

	custom := findCustom(t, doc)
	if !inParagraph(doc, custom) {
		t.Errorf("inline Custom did not join the paragraph around it:\n%s", value.FormatContent(doc))
	}
}

// TestCustomLabel checks that a host element takes a label like any other
// content, so a document can point at one and Query can find it.
func TestCustomLabel(t *testing.T) {
	doc := compile(t, `#block-elem("diff.go") <impl>
`, markst.WithBindings(customBindings()))

	custom := findCustom(t, doc)
	if l := custom.GetLabel(); l == nil || l.Name != name.Make("impl") {
		t.Errorf("GetLabel() = %v, want <impl>", l)
	}
}

// TestCustomBindingErrorIsDiagnosed is the reason the host does its work while
// compiling rather than while rendering: a failure points at the argument that
// caused it, in the document, the way any other error does.
func TestCustomBindingErrorIsDiagnosed(t *testing.T) {
	diags := diagnostics(t, `#block-elem("notes.txt")
`, markst.WithName("index.mst"), markst.WithBindings(customBindings()))

	if len(diags) != 1 {
		t.Fatalf("Compile() diagnostics = %v, want exactly one", diags)
	}
	d := diags[0]
	if !strings.Contains(d.Msg, "not a Go file") {
		t.Errorf("diagnostic = %q, want it to mention the binding's error", d.Msg)
	}
	if d.Origin != "index.mst" {
		t.Errorf("diagnostic origin = %q, want %q", d.Origin, "index.mst")
	}
	if d.Loc.Start.Line != 1 {
		t.Errorf("diagnostic line = %d, want 1 — it should point at the argument", d.Loc.Start.Line)
	}
}
