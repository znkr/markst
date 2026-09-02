package eval

import (
	"fmt"
	"strings"
	"unicode"

	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// assignHeadingLabels labels every heading the source left unlabelled, deriving
// the label from the heading's own text, so that a presenter always has an
// anchor to link a section by. It runs once over a realized document, in
// document order; see [session.realizeDocument] for why it runs after
// realization rather than during it, and why it takes a [value.Tree] rather
// than the document itself.
//
// The labels it hands out are marked [value.Label.Auto] and are deliberately
// not registered on the session: the document's namespace — what `@ref` and
// `#show <x>:` see — is what the source wrote, and settled long before this.
func (s *session) assignHeadingLabels(t value.Tree) {
	ids := newIDPool(s.labels)
	for c := range t.Preorder(value.SetOf(value.KindHeading)) {
		h := c.Node().(*value.Heading)
		if h.Label != nil {
			continue
		}
		h.Label = &value.Label{Name: name.Make(ids.claim(slug(headingText(h)))), Auto: true}
	}
}

// idPool hands out unique label names, disambiguating a name already taken by
// appending -1, -2, … until one is free.
type idPool struct {
	taken map[string]bool
}

// newIDPool returns a pool with every label the document already carries
// reserved, so that a generated name can never collide with one the source
// wrote — including one written further down the document than the heading
// being labelled. (goldmark reserves only the ids it has already walked past.)
func newIDPool(labels map[name.Name]struct{}) *idPool {
	p := &idPool{taken: make(map[string]bool, len(labels))}
	for n := range labels {
		p.taken[n.String()] = true
	}
	return p
}

func (p *idPool) claim(id string) string {
	if !p.taken[id] {
		p.taken[id] = true
		return id
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", id, i)
		if !p.taken[candidate] {
			p.taken[candidate] = true
			return candidate
		}
	}
}

// slug derives a label name from a heading's text, following goldmark's
// parser.WithAutoHeadingID(): letters and digits are lowercased and kept,
// spaces and - and _ become -, everything else is dropped, and text that leaves
// nothing behind becomes "heading". Runs of separators are deliberately not
// collapsed and the result is not trimmed of them — `Hello,  World!` is
// `hello--world` there and here, and an anchor that already exists in the wild
// matters more than a tidier one.
//
// It differs from goldmark in keeping non-ASCII letters, which goldmark drops
// outright (`Über uns` is `ber-uns` to it, and `日本語` is `heading`).
func slug(text string) string {
	var b strings.Builder
	// A slug is never longer than the text it comes from: every rune is kept as
	// itself, replaced by one byte, or dropped.
	b.Grow(len(text))
	for _, r := range strings.TrimSpace(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r) || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "heading"
	}
	return b.String()
}

// headingText is the plain text a heading reads as: the text of the elements
// that carry any, in document order, with everything that renders as markup
// rather than characters contributing nothing. Emphasis and links contribute
// the text inside them; a smart quote contributes nothing, which is how
// `What's new` slugs to `whats-new` — the same answer goldmark reaches by
// dropping the apostrophe from the source line.
func headingText(h *value.Heading) string {
	var b strings.Builder
	for c := range value.Preorder(h.Body, headingTextKinds) {
		switch n := c.Node().(type) {
		case *value.Text:
			b.WriteString(n.Text)
		case *value.Raw:
			b.WriteString(n.Text)
		case *value.MathText:
			b.WriteString(n.Text)
		case *value.Linebreak:
			b.WriteByte(' ')
		case *value.Footnote:
			// A footnote's text is not part of the title it hangs off of.
			c.SkipChildren()
		}
	}
	return b.String()
}

// headingTextKinds is what headingText reads. Footnote carries no text of its
// own and is in the set only so that the walk reaches it and can decline to
// descend.
var headingTextKinds = value.SetOf(
	value.KindText,
	value.KindRaw,
	value.KindMathText,
	value.KindLinebreak,
	value.KindFootnote,
)
