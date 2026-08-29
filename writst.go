package writst

import (
	"znkr.io/writst/eval"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax/analyzer"
	"znkr.io/writst/syntax/parser"
	"znkr.io/writst/value"
)

// Compile turns writst source into a realized document, running the whole
// pipeline over it: parse, analyze, evaluate, realize. Turning the result into
// output — HTML, or anything else — is the presenter's job; see
// [znkr.io/writst/smartquote] for the one part of that a presenter cannot do
// on its own.
//
// Warnings are returned separately from err, because they describe a document
// that compiled: a label used twice, content discarded where it can have no
// effect. A caller that folded them into failure would reject documents that
// are fine. err is an [eval.ErrorList] when it is non-nil, so the individual
// diagnostics — each carrying the span it was reported at — are reachable
// through errors.As or its Unwrap.
func Compile(src string) (*value.Document, []eval.Error, error) {
	root := parser.Parse(src)
	mod := analyzer.Analyze(root)
	doc, warnings, errs := eval.Eval(mod)
	if len(errs) > 0 {
		return doc, warnings, eval.ErrorList(errs)
	}
	return doc, warnings, nil
}

// Query returns the value carried by the [value.Metadata] element in doc
// labelled label — the way a document hands data to the program presenting it:
//
//	#metadata("2024-02-29") <published>
//
// Metadata without a label is never returned: the label is what identifies it.
// A label shared by two metadata elements is reported as a warning by
// [Compile]; Query answers with the first in document order.
func Query(doc *value.Document, label name.Name) (value.Value, bool) {
	if doc == nil || doc.Body == nil {
		return nil, false
	}
	for c := range value.All(doc.Body) {
		m, ok := c.(*value.Metadata)
		if !ok || m.Label == nil || m.Label.Name != label {
			continue
		}
		return m.Value, true
	}
	return nil, false
}
