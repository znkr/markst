package markst_test

import (
	"strings"
	"testing"

	"znkr.io/markst"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// frontMatter is the shape this API exists for: a document that hands data to
// the program presenting it, out of band from the content it renders.
const frontMatter = `#set document(title: "My Post", date: "2024-02-29")
#metadata("A short summary.") <summary>
#metadata(("go", "typst")) <tags>
#metadata(false) <draft>
#metadata("nobody asked") 

= Hello
Body text with #metadata(42) <inline> in the middle of it.
`

func TestQuery(t *testing.T) {
	doc, warnings, err := markst.Compile([]byte(frontMatter))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("Compile() warnings = %v, want none", warnings)
	}

	tests := []struct {
		label string
		want  value.Value
	}{
		{"summary", value.Str("A short summary.")},
		{"tags", &value.Array{Elems: []value.Value{value.Str("go"), value.Str("typst")}}},
		{"draft", value.Bool(false)},
		// Reached by descending into the paragraph it sits in, which is why
		// Query walks rather than reading a list off the root.
		{"inline", value.Int(42)},
	}
	for _, tt := range tests {
		got, ok := markst.Query(doc, name.Make(tt.label))
		if !ok {
			t.Errorf("Query(<%s>) not found", tt.label)
			continue
		}
		if !value.Equal(got, tt.want) {
			t.Errorf("Query(<%s>) = %v, want %v",
				tt.label, value.FormatValue(got), value.FormatValue(tt.want))
		}
	}

	// A label nothing carries, and an unlabelled entry — which is in the
	// document but has no name to ask for it by.
	for _, label := range []string{"missing", "nobody asked"} {
		if got, ok := markst.Query(doc, name.Make(label)); ok {
			t.Errorf("Query(<%s>) = %v, want not found", label, value.FormatValue(got))
		}
	}

	if got, ok := markst.Query(nil, name.Make("summary")); ok {
		t.Errorf("Query(nil, …) = %v, want not found", value.FormatValue(got))
	}
}

// TestQueryDocumentOrder pins the answer Query gives when a label is reused:
// the first in document order, and a warning saying so.
func TestQueryDocumentOrder(t *testing.T) {
	doc, warnings, err := markst.Compile([]byte("#metadata(1) <dup>\n#metadata(2) <dup>\n= Hello\n"))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("Compile() warnings = %v, want exactly one", warnings)
	}
	if msg := warnings[0].Error(); !strings.Contains(msg, "used more than once") {
		t.Errorf("Compile() warning = %q, want one about a reused label", msg)
	}
	got, ok := markst.Query(doc, name.Make("dup"))
	if !ok || !value.Equal(got, value.Int(1)) {
		t.Errorf("Query(<dup>) = %v, %v; want 1, true", got, ok)
	}
}

// TestCompileWarningIsNotFailure guards the split Compile makes between a
// document that failed and one that merely has something to say: a warning
// must not come back as err, or callers would reject documents that are fine.
func TestCompileWarningIsNotFailure(t *testing.T) {
	doc, warnings, err := markst.Compile([]byte("= Hello <a> <b>\n"))
	if err != nil {
		t.Fatalf("Compile() = %v, want no error", err)
	}
	if len(warnings) == 0 {
		t.Fatal("Compile() warnings = none, want one about the second label")
	}
	if doc == nil || doc.Body == nil {
		t.Fatal("Compile() document is empty, want the heading")
	}
}

// TestCompileError checks the other half of that split.
func TestCompileError(t *testing.T) {
	_, _, err := markst.Compile([]byte("#metadata()\n"))
	if err == nil {
		t.Fatal("Compile() = nil, want an error for the missing argument")
	}
	if msg := err.Error(); !strings.Contains(msg, "missing argument") {
		t.Errorf("Compile() error = %q, want one about the missing argument", msg)
	}
}
