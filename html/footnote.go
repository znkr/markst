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
	"strconv"

	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// Footnotes returns the footnotes in c, in document order; the first is
// numbered 1.
//
// One footnote reached from two places — through a binding used twice — is a
// single footnote and appears here once, so every citation of it shows the same
// number.
//
// Pass the result to [WithFootnotes] to render a piece of a document whose
// citations keep the numbers they have in the whole.
func Footnotes(c value.Content) []*value.Footnote {
	return collect(nil, c)
}

// collect returns the distinct footnotes in c, in document order. It reads them
// off x when the caller supplied one, which for a document that has few
// footnotes is most of what an index is for: the subtrees without any are
// stepped over rather than searched.
func collect(x *value.Index, c value.Content) []*value.Footnote {
	if c == nil {
		return nil
	}
	found := value.Preorder(c, footnoteKind)
	if x != nil {
		found = x.Preorder(footnoteKind)
	}
	var list []*value.Footnote
	seen := make(map[*value.Footnote]bool)
	for v := range found {
		fn := v.Node().(*value.Footnote)
		if seen[fn] {
			continue
		}
		seen[fn] = true
		list = append(list, fn)
	}
	return list
}

var footnoteKind = value.SetOf(value.KindFootnote)

// notes is a numbered run of footnotes, looked up by the footnote itself and by
// the label an @ref cites it with.
type notes struct {
	list    []*value.Footnote
	byNote  map[*value.Footnote]int
	byLabel map[name.Name]int
}

func index(list []*value.Footnote) *notes {
	ns := &notes{
		list:    list,
		byNote:  make(map[*value.Footnote]int, len(list)),
		byLabel: make(map[name.Name]int, len(list)),
	}
	for i, fn := range list {
		ns.byNote[fn] = i + 1
		if l := fn.GetLabel(); l != nil {
			ns.byLabel[l.Name] = i + 1
		}
	}
	return ns
}

// renderFootnote writes a footnote where it is cited: the mark on the line,
// linking to the endnote that carries the text.
func (e *Encoder) renderFootnote(c *value.Footnote) {
	n, ok := e.notes.byNote[c]
	if !ok {
		// A footnote that was not in the content the render started from, such
		// as one a hook built. There is no endnote to link to, so write the
		// body where it stands.
		e.itemBody(c.Body)
		return
	}
	e.citation("fnref:"+strconv.Itoa(n), n)
}

// citation writes the superscript mark linking to endnote n. id is the anchor
// the endnote links back to, empty for a repeat citation made with @ref — the
// return trip goes to where the footnote was written, not to every mention.
func (e *Encoder) citation(id string, n int) {
	num := strconv.Itoa(n)
	anchor := Attr{}
	if id != "" {
		anchor = Attr{"id", id}
	}
	e.Start("sup", anchor)
	e.Start("a", Attr{"href", "#fn:" + num})
	e.Text(num)
	e.End("a")
	e.End("sup")
}

// footnoteList writes the endnotes, in citation order, each linking back to
// where it was cited. Nothing is written when the document has no footnotes.
func (e *Encoder) footnoteList() {
	if len(e.notes.list) == 0 {
		return
	}
	e.Start("div", Attr{"class", "footnotes"}, Attr{"role", "doc-endnotes"})
	e.Newline()
	e.HTML("<hr>")
	e.Newline()
	e.Start("ol")
	e.Newline()
	for i, fn := range e.notes.list {
		num := strconv.Itoa(i + 1)
		e.Start("li", Attr{"id", "fn:" + num})
		e.itemBody(fn.Body)
		if !e.atLineStart() {
			// The endnote is a line of text and the mark belongs at the end of
			// it. After a block it is already on a line of its own.
			e.HTML(" ")
		}
		e.Start("a", Attr{"href", "#fnref:" + num}, Attr{"role", "doc-backlink"})
		e.HTML("&#8617;")
		e.End("a")
		e.End("li")
		e.Newline()
	}
	e.End("ol")
	e.Newline()
	e.End("div")
	e.Newline()
}
