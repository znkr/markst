package html_test

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"testing"

	"znkr.io/markst"
	"znkr.io/markst/html"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// compile compiles src, failing the test on any diagnostic.
func compile(t *testing.T, src string, opts ...markst.Option) *value.Document {
	t.Helper()
	doc, diags, err := markst.Compile([]byte(src), append(opts, markst.WithName("test.mst"))...)
	if err != nil {
		t.Fatalf("compiling:\n%s", formatDiags(err.(markst.DiagnosticList)))
	}
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", formatDiags(diags))
	}
	return doc
}

func renderDoc(t *testing.T, doc value.Content, opts ...html.Option) string {
	t.Helper()
	var b strings.Builder
	if err := html.Render(&b, doc, opts...); err != nil {
		t.Fatalf("rendering: %v", err)
	}
	return b.String()
}

func TestWithElementReplaces(t *testing.T) {
	doc := compile(t, "Call `printf` and ```go\nx\n``` today.\n")
	got := renderDoc(t, doc, html.WithElement(func(e *html.Encoder, c value.Content) (bool, error) {
		raw, ok := c.(*value.Raw)
		if !ok {
			return false, nil
		}
		e.HTML("<code data-lang=\"" + raw.Lang + "\">")
		e.Text(raw.Text)
		e.HTML("</code>")
		return true, nil
	}))
	// The block raw is block-level, so it breaks the paragraph around it.
	want := "<p>Call <code data-lang=\"\">printf</code> and</p>\n" +
		"<code data-lang=\"go\">x</code>" +
		"<p>today.</p>\n"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestWithElementDecoratesWithDefault(t *testing.T) {
	doc := compile(t, "= Title\n")
	got := renderDoc(t, doc, html.WithElement(func(e *html.Encoder, c value.Content) (bool, error) {
		h, ok := c.(*value.Heading)
		if !ok {
			return false, nil
		}
		e.Default(h)
		e.HTML("<!-- after -->")
		return true, nil
	}))
	want := "<h1 id=\"title\">Title</h1>\n<!-- after -->"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestWithElementHooksAreTriedInOrder(t *testing.T) {
	doc := compile(t, "= Title\n")
	first := func(e *html.Encoder, c value.Content) (bool, error) {
		if _, ok := c.(*value.Heading); !ok {
			return false, nil
		}
		e.HTML("<first>")
		return true, nil
	}
	second := func(e *html.Encoder, c value.Content) (bool, error) {
		if _, ok := c.(*value.Heading); !ok {
			return false, nil
		}
		e.HTML("<second>")
		return true, nil
	}
	if got, want := renderDoc(t, doc, html.WithElement(first), html.WithElement(second)), "<first>"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
	if got, want := renderDoc(t, doc, html.WithElement(second), html.WithElement(first)), "<second>"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestWithElementDescendsWithContent(t *testing.T) {
	// A hook that rewrites the container still gets the built-in rendering of
	// what is inside it, hooks and all.
	doc := compile(t, "A *bold* word.\n")
	got := renderDoc(t, doc, html.WithElement(func(e *html.Encoder, c value.Content) (bool, error) {
		par, ok := c.(*value.Par)
		if !ok {
			return false, nil
		}
		e.Start("div", html.Attr{Name: "class", Value: "para"})
		e.Content(par.Body)
		e.End("div")
		return true, nil
	}))
	want := `<div class="para">A <strong>bold</strong> word.</div>`
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestWithHeadingLevel(t *testing.T) {
	doc := compile(t, "= One\n\n== Two\n")
	for _, tc := range []struct {
		level int
		want  string
	}{
		{1, "<h1 id=\"one\">One</h1>\n<h2 id=\"two\">Two</h2>\n"},
		{2, "<h2 id=\"one\">One</h2>\n<h3 id=\"two\">Two</h3>\n"},
		{6, "<h6 id=\"one\">One</h6>\n<h6 id=\"two\">Two</h6>\n"},
	} {
		if got := renderDoc(t, doc, html.WithHeadingLevel(tc.level)); got != tc.want {
			t.Errorf("WithHeadingLevel(%d) = %q, want %q", tc.level, got, tc.want)
		}
	}
}

func TestWithURLOptions(t *testing.T) {
	doc := compile(t, "= Intro <intro>\n\n#image(\"cat.png\") #link(\"a.html\")[a] @intro\n")
	got := renderDoc(t, doc,
		html.WithImageURL(func(p string) (string, error) { return path.Join("/assets", p), nil }),
		html.WithLinkURL(func(d string) (string, error) { return path.Join("/posts", d), nil }),
		html.WithLabelURL(func(l name.Name) (string, error) { return "/posts/#" + l.String(), nil }),
	)
	for _, want := range []string{
		`src="/assets/cat.png"`,
		`href="/posts/a.html"`,
		`href="/posts/#intro"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() = %q, missing %q", got, want)
		}
	}
}

func TestURLHookError(t *testing.T) {
	doc := compile(t, "#image(\"cat.png\")\n")
	boom := errors.New("boom")
	err := html.Render(io.Discard, doc, html.WithImageURL(func(string) (string, error) { return "", boom }))
	if !errors.Is(err, boom) {
		t.Errorf("Render() error = %v, want %v", err, boom)
	}
}

func TestWithoutFootnoteList(t *testing.T) {
	doc := compile(t, "Text.#footnote[A note.]\n")
	got := renderDoc(t, doc, html.WithoutFootnoteList())
	want := "<p>Text.<sup id=\"fnref:1\"><a href=\"#fn:1\">1</a></sup></p>\n"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestFootnotesNumberedOnce(t *testing.T) {
	// One footnote bound to a name and used twice is one footnote: it keeps one
	// number, and the endnote list holds it once.
	doc := compile(t, "#let n = footnote[A note.]\nOne.#n Two.#n\n")
	notes := html.Footnotes(doc)
	if len(notes) != 1 {
		t.Fatalf("Footnotes() returned %d notes, want 1", len(notes))
	}
	got := renderDoc(t, doc)
	if n := strings.Count(got, `href="#fn:1"`); n != 2 {
		t.Errorf("got %d citations of footnote 1, want 2\n%s", n, got)
	}
	if n := strings.Count(got, `<li id="fn:`); n != 1 {
		t.Errorf("got %d endnotes, want 1\n%s", n, got)
	}
}

// customFn binds `box(v)` to a function producing a custom element carrying v.
func customFn() map[name.Name]value.Value {
	return map[name.Name]value.Value{
		name.Make("box"): &value.Function{
			Name:       "box",
			Positional: []value.Param{{Name: "v", Type: types.SetOf(types.Str)}},
			F: func(*value.FunctionCallContext, []value.Value, value.NamedArgsWithDefaults) (value.Value, error) {
				return &value.Custom{Elem: "box", Block: true, Value: "payload"}, nil
			},
		},
	}
}

func TestCustomWithoutHookIsAnError(t *testing.T) {
	doc := compile(t, "#box(\"x\")\n", markst.WithBindings(customFn()))
	err := html.Render(io.Discard, doc)
	if err == nil || !strings.Contains(err.Error(), "custom element \"box\"") {
		t.Errorf("Render() error = %v, want one naming the custom element", err)
	}
}

func TestCustomWithHook(t *testing.T) {
	doc := compile(t, "#box(\"x\")\n", markst.WithBindings(customFn()))
	got := renderDoc(t, doc, html.WithElement(func(e *html.Encoder, c value.Content) (bool, error) {
		box, ok := c.(*value.Custom)
		if !ok {
			return false, nil
		}
		// Encoder is an io.Writer, which is how a template writes into it.
		fmt.Fprintf(e, "<aside>%s</aside>\n", box.Value)
		return true, nil
	}))
	if want := "<aside>payload</aside>\n"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestHookErrorStopsTheRender(t *testing.T) {
	doc := compile(t, "= One\n\n= Two\n")
	boom := errors.New("boom")
	var b strings.Builder
	err := html.Render(&b, doc, html.WithElement(func(e *html.Encoder, c value.Content) (bool, error) {
		h, ok := c.(*value.Heading)
		if !ok {
			return false, nil
		}
		if h.Depth == 1 && strings.Contains(b.String(), "One") {
			return false, boom
		}
		return false, nil
	}))
	if !errors.Is(err, boom) {
		t.Fatalf("Render() error = %v, want %v", err, boom)
	}
	if strings.Contains(b.String(), "Two") {
		t.Errorf("Render() carried on past the failure: %q", b.String())
	}
}

// failWriter fails on the nth write and every one after.
type failWriter struct {
	n   int
	err error
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.n <= 0 {
		return 0, w.err
	}
	w.n--
	return len(p), nil
}

func TestWriteErrorIsReturned(t *testing.T) {
	doc := compile(t, "= One\n\n= Two\n")
	boom := errors.New("boom")
	if err := html.Render(&failWriter{err: boom}, doc); !errors.Is(err, boom) {
		t.Errorf("Render() error = %v, want %v", err, boom)
	}
}

func TestRenderFragment(t *testing.T) {
	// Any content renders, not only a whole document — the summary a document
	// carries in its metadata is a fragment like this one.
	doc := compile(t, "A *summary* of it.\n")
	if got, want := renderDoc(t, doc.Body), "<p>A <strong>summary</strong> of it.</p>\n"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderNil(t *testing.T) {
	var b strings.Builder
	if err := html.Render(&b, nil); err != nil {
		t.Fatalf("Render(nil) error = %v", err)
	}
	if b.String() != "" {
		t.Errorf("Render(nil) wrote %q, want nothing", b.String())
	}
}

func TestWithFootnotesKeepsDocumentNumbers(t *testing.T) {
	// A heading rendered on its own — for a table of contents — cites the
	// footnote by the number it has in the document, not by a new one.
	doc := compile(t, "Body.#footnote[First]\n\n= Head#footnote[Second]\n")
	notes := html.Footnotes(doc)
	if len(notes) != 2 {
		t.Fatalf("Footnotes() returned %d notes, want 2", len(notes))
	}

	heading := markst.Outline(doc)[0].Heading
	got := renderDoc(t, heading.Body, html.WithFootnotes(notes), html.WithoutFootnoteList())
	want := `Head<sup id="fnref:2"><a href="#fn:2">2</a></sup>`
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}

	// Without it the fragment numbers what it can see, from 1.
	got = renderDoc(t, heading.Body, html.WithoutFootnoteList())
	want = `Head<sup id="fnref:1"><a href="#fn:1">1</a></sup>`
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

// TestRenderWithIndex checks that handing the renderer an index changes what a
// render costs and nothing else: the same HTML, footnotes included.
func TestRenderWithIndex(t *testing.T) {
	doc, _, err := markst.Compile([]byte("A note#footnote[first].\n\n= H\n\nAnother#footnote[second].\n"))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}

	var walked, indexed strings.Builder
	if err := html.Render(&walked, doc); err != nil {
		t.Fatalf("Render() = %v", err)
	}
	idx := value.NewIndex(doc)
	if err := html.Render(&indexed, doc, html.WithIndex(&idx)); err != nil {
		t.Fatalf("Render(WithIndex) = %v", err)
	}
	if walked.String() != indexed.String() {
		t.Errorf("Render(WithIndex) wrote\n%s\nwant\n%s", indexed.String(), walked.String())
	}
}
