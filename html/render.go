// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package html

import (
	"fmt"
	"strconv"

	"znkr.io/markst/value"
)

// render writes c the built-in way. value.Content is a closed interface, so
// this switch has a case for every element and is exhaustive.
func (e *Encoder) render(c value.Content) {
	switch c := c.(type) {
	case *value.Document:
		// The endnote list is not part of the body. Render appends it after the
		// whole document has been written.
		e.Content(c.Body)
	case *value.Sequence:
		e.tagless(c, func() {
			for _, child := range c.Children {
				e.Content(child)
			}
		})
	case *value.Par:
		e.blockTag("p", c, c.Body)
	case *value.Heading:
		tag := "h" + strconv.Itoa(e.headingLevel(c.Depth))
		e.blockTag(tag, c, c.Body)
	case *value.Strong:
		e.inlineTag("strong", c, c.Body)
	case *value.Emph:
		e.inlineTag("em", c, c.Body)
	case *value.Underline:
		e.inlineTag("u", c, c.Body)
	case *value.Text:
		e.tagless(c, func() { e.Text(c.Text) })
	case *value.SmartQuote:
		e.tagless(c, func() { e.Text(e.quote) })
	case *value.Linebreak:
		e.HTML("<br>")
	case *value.HSpace:
		e.Start("span", Attr{"style", "display:inline-block;width:" + cssLength(c.Amount)})
		e.End("span")
	case *value.Raw:
		e.renderRaw(c)

	case *value.List:
		e.itemList("ul", c, len(c.Children), func(i int) { e.Content(c.Children[i]) })
	case *value.ListItem:
		e.item("li", c, c.Body)
	case *value.Enum:
		e.itemList("ol", c, len(c.Children), func(i int) { e.Content(c.Children[i]) })
	case *value.EnumItem:
		// A number written in the document (`3. third`). <ol> continues
		// numbering from a value attribute, which is the behavior wanted here.
		if c.Number >= 0 {
			e.Start("li", Attr{"value", strconv.Itoa(c.Number)}, e.idAttr(c))
			e.itemBody(c.Body)
			e.End("li")
			e.Newline()
			return
		}
		e.item("li", c, c.Body)
	case *value.Terms:
		e.itemList("dl", c, len(c.Children), func(i int) { e.Content(c.Children[i]) })
	case *value.TermItem:
		e.Start("dt", e.idAttr(c))
		e.Content(c.Term)
		e.End("dt")
		e.Newline()
		e.Start("dd")
		e.itemBody(c.Description)
		e.End("dd")
		e.Newline()

	case *value.Table:
		e.renderTable(c)
	case *value.TableHeader:
		// Reached only for a header outside a table, which markst allows to be
		// built even though it has no meaning there. Render its cells alone.
		for _, child := range c.Children {
			e.Content(child)
		}

	case *value.Link:
		href, err := e.cfg.url(e.cfg.linkURL, c.Dest)
		if err != nil {
			e.fail(err)
			return
		}
		e.Start("a", Attr{"href", href}, e.idAttr(c))
		e.Content(c.Body)
		e.End("a")
	case *value.Ref:
		e.renderRef(c)
	case *value.Footnote:
		e.renderFootnote(c)
	case *value.Image:
		src, err := e.cfg.url(e.cfg.imageURL, c.Path)
		if err != nil {
			e.fail(err)
			return
		}
		e.Start("img", Attr{"src", src}, Attr{"alt", c.Alt}, e.idAttr(c))

	case *value.HTMLElem:
		e.renderHTMLElem(c)
	case *value.Custom:
		e.fail(fmt.Errorf("html: no rendering for custom element %q: pass a WithElement hook that handles it", c.Elem))

	case *value.Metadata, *value.StateUpdate, *value.StyleUpdate:
		// These produce no output; they carry data through the document.
	case *value.Parbreak:
		// Realization drops these while grouping content into paragraphs, so
		// one reaches here only from content built by hand.
	case *value.Styled:
		// Realization resolves these away too. What it wraps is still content.
		e.Content(c.Body)

	case *value.Equation:
		e.renderEquation(c)
	default:
		// The math elements have no meaning outside an equation but are content
		// all the same, so render each as an equation of its own.
		e.renderMath(c)
	}
}

// tagless writes an element that has no tag of its own. A label on it still
// needs an anchor, so a labeled element becomes a <span> carrying the id.
// Without a label, only the content is written.
func (e *Encoder) tagless(c value.Content, body func()) {
	id := e.idAttr(c)
	if id.Name == "" {
		body()
		return
	}
	e.Start("span", id)
	body()
	e.End("span")
}

// headingLevel is the HTML level a markst heading of this depth is written at.
// HTML stops at h6, so a document nested deeper than that keeps its structure
// and loses only the distinction the last level would have made.
func (e *Encoder) headingLevel(depth int) int {
	return min(max(depth+e.cfg.headingLevel-1, 1), 6)
}

// blockTag writes a block-level element: its tag, its body, and the newline
// that separates it from what follows.
func (e *Encoder) blockTag(tag string, c value.Content, body value.Content) {
	e.Start(tag, e.idAttr(c))
	e.Content(body)
	e.End(tag)
	e.Newline()
}

// inlineTag writes an inline element, which shares its line with its
// neighbors and so ends without a newline.
func (e *Encoder) inlineTag(tag string, c value.Content, body value.Content) {
	e.Start(tag, e.idAttr(c))
	e.Content(body)
	e.End(tag)
}

// itemList writes a list container: the items each start on their own line.
func (e *Encoder) itemList(tag string, c value.Content, n int, item func(int)) {
	e.Start(tag, e.idAttr(c))
	e.Newline()
	for i := range n {
		item(i)
	}
	e.End(tag)
	e.Newline()
}

// item writes one entry of a list.
func (e *Encoder) item(tag string, c value.Content, body value.Content) {
	e.Start(tag, e.idAttr(c))
	e.itemBody(body)
	e.End(tag)
	e.Newline()
}

// itemBody writes the body of a list item, term description, table cell, or
// endnote. A body holding a single paragraph is written without the <p> around
// it: such an entry reads as a plain one, and only an entry with several
// paragraphs needs them marked off. An item with a nested list under its text
// counts as one, so a nested list does not space its parent out.
func (e *Encoder) itemBody(body value.Content) {
	switch b := body.(type) {
	case *value.Par:
		if b.Label == nil {
			e.unwrapPar(b)
			return
		}
	case *value.Sequence:
		if b.Label == nil && solePar(b.Children) != nil {
			for _, child := range b.Children {
				if par, ok := child.(*value.Par); ok {
					e.unwrapPar(par)
					continue
				}
				e.Content(child)
			}
			return
		}
	}
	e.Content(body)
}

// unwrapPar writes a paragraph's body without the paragraph. The <p> is gone
// from the output but not from the document: a block still ends the quotation
// context around it.
func (e *Encoder) unwrapPar(par *value.Par) {
	e.quote = e.q.Advance(par)
	e.Content(par.Body)
}

// solePar returns the single unlabeled paragraph among children, or nil when
// there is none, more than one, or one that carries a label. A labeled
// paragraph is kept, since the label needs an element to attach to.
func solePar(children []value.Content) *value.Par {
	var found *value.Par
	for _, child := range children {
		par, ok := child.(*value.Par)
		if !ok {
			continue
		}
		if found != nil || par.Label != nil {
			return nil
		}
		found = par
	}
	return found
}

// renderRaw writes raw text: a block of it as <pre><code>, a span as <code>.
// A language becomes a class on the <code>, which is where the highlighters
// that run in a browser look for it.
func (e *Encoder) renderRaw(c *value.Raw) {
	lang := Attr{}
	if c.Lang != "" {
		lang = Attr{"class", "language-" + c.Lang}
	}
	if c.Block {
		e.Start("pre")
	}
	e.Start("code", lang)
	e.Text(c.Text)
	e.End("code")
	if c.Block {
		e.End("pre")
		e.Newline()
	}
}

// renderRef writes a cross-reference. A reference to a footnote repeats that
// footnote's number rather than making a second one, which is how one footnote
// is cited from several places; every other target becomes a link to the
// element carrying the label, with the label's own name as the text when the
// reference gave no supplement.
func (e *Encoder) renderRef(c *value.Ref) {
	if n, ok := e.notes.byLabel[c.Target]; ok {
		e.citation("", n)
		return
	}
	href := "#" + c.Target.String()
	if e.cfg.labelURL != nil {
		var err error
		if href, err = e.cfg.labelURL(c.Target); err != nil {
			e.fail(err)
			return
		}
	}
	e.Start("a", Attr{"href", href}, e.idAttr(c))
	if c.Supplement != nil {
		e.Content(c.Supplement)
	} else {
		e.Text(c.Target.String())
	}
	e.End("a")
}

// renderHTMLElem writes markup the document wrote itself. markst validated the
// tag and the attribute names, so what is left is escaping the values and
// writing the tags.
func (e *Encoder) renderHTMLElem(c *value.HTMLElem) {
	attrs := make([]Attr, 0, 8)
	hasID := false
	if c.Attrs != nil {
		for k, v := range c.Attrs.Elems.All() {
			s, ok := v.(value.Str)
			if !ok {
				e.fail(fmt.Errorf("html: attribute %q of <%s> is a %s, not a string", k, c.Tag, v.Type()))
				return
			}
			if k == "id" {
				hasID = true
			}
			attrs = append(attrs, Attr{string(k), string(s)})
		}
	}
	// A label is how the rest of the document refers to this element, which in
	// HTML is the id. An explicit id attribute takes precedence over it.
	if !hasID {
		attrs = append(attrs, e.idAttr(c))
	}

	e.Start(c.Tag, attrs...)
	if !value.HtmlTagVoid(c.Tag) {
		e.Content(c.Body)
		e.End(c.Tag)
	}
	if c.Block {
		e.Newline()
	}
}

// renderTable writes a table as <table>, with the leading header rows in a
// <thead> and the rest in a <tbody>. A header written in the middle of the
// table stays where it is and keeps its <th> cells.
func (e *Encoder) renderTable(t *value.Table) {
	rows := tableRows(t)
	head := 0
	for head < len(rows) && rows[head].header {
		head++
	}

	e.Start("table", e.idAttr(t))
	e.Newline()
	if head > 0 {
		e.Start("thead")
		e.Newline()
		e.renderRows(rows[:head])
		e.End("thead")
		e.Newline()
	}
	if head < len(rows) {
		e.Start("tbody")
		e.Newline()
		e.renderRows(rows[head:])
		e.End("tbody")
		e.Newline()
	}
	e.End("table")
	e.Newline()
}

func (e *Encoder) renderRows(rows []tableRow) {
	for _, row := range rows {
		tag := "td"
		if row.header {
			tag = "th"
		}
		e.Start("tr")
		for _, cell := range row.cells {
			e.Start(tag)
			e.itemBody(cell)
			e.End(tag)
		}
		e.End("tr")
		e.Newline()
	}
}

type tableRow struct {
	header bool
	cells  []value.Content
}

// tableRows cuts a table's cells into rows of Columns cells. A table.header
// always starts a row, so its cells never share one with body cells; a run that
// doesn't fill its last row leaves that row short.
func tableRows(t *value.Table) []tableRow {
	var rows []tableRow
	cut := func(cells []value.Content, header bool) {
		for len(cells) > 0 {
			n := min(max(t.Columns, 1), len(cells))
			rows = append(rows, tableRow{header: header, cells: cells[:n]})
			cells = cells[n:]
		}
	}

	var body []value.Content
	for _, child := range t.Children {
		if h, ok := child.(*value.TableHeader); ok {
			cut(body, false)
			body = nil
			cut(h.Children, true)
			continue
		}
		body = append(body, child)
	}
	cut(body, false)
	return rows
}

// url applies one of the rewriting hooks, or returns what the document wrote
// when there is none.
func (c *config) url(f func(string) (string, error), s string) (string, error) {
	if f == nil {
		return s, nil
	}
	return f(s)
}

// cssLength writes a markst length as a CSS one. The two components are
// independent, points being absolute and ems relative to the font size, so a
// length with both needs calc to add them.
func cssLength(l value.Length) string {
	switch {
	case l.Pt == 0 && l.Em == 0:
		return "0"
	case l.Pt == 0:
		return fmt.Sprintf("%.4gem", l.Em)
	case l.Em == 0:
		return fmt.Sprintf("%.4gpt", l.Pt)
	default:
		return fmt.Sprintf("calc(%.4gpt + %.4gem)", l.Pt, l.Em)
	}
}
