package eval

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/value"
)

// realizeDocument turns the recorded content tree into a realized
// [value.Document]: paragraphs are formed, list/enum/term items grouped,
// `*value.Styled` wrappers resolved (show recipes applied, `set document`
// properties hoisted), and every unlabelled heading given a label to be linked
// by. It runs inside [Eval] while the session — and any closures captured by
// show transforms — are still live.
func (s *session) realizeDocument(c value.Content) *value.Document {
	// Realize first: the body is where the `set document` rules live, so
	// s.doc isn't populated until it has been walked.
	body := s.realizeBody(topChildren(c), nil)
	// Heading labels come after, not during: a show rule runs as part of the
	// pass above and can both reword a heading and attach labels of its own,
	// so neither the text a label is derived from nor the set of names already
	// taken is settled until the pass has finished.
	s.assignHeadingLabels(body)
	doc := s.doc
	doc.Body = body
	return &doc
}

// topChildren returns c's children when it is an (unlabeled) sequence, else c
// itself, so the top-level body is grouped as a single run.
func topChildren(c value.Content) []value.Content {
	if seq, ok := c.(*value.Sequence); ok && seq.Label == nil {
		return seq.Children
	}
	return []value.Content{c}
}

// realizeBody flattens and realizes a run of sibling content, then groups it
// into paragraphs and item containers. This is *block context*: bare inline runs
// become paragraphs.
func (s *session) realizeBody(children []value.Content, label *value.Label) value.Content {
	r := seqOf(groupContent(trimRun(mergeText(s.flattenAndRealize(children)), true, true)))
	if label != nil {
		r.SetLabel(label)
	}
	return r
}

// flattenAndRealize realizes each child and splices unlabeled sequences (and
// resolved style scopes) into a single flat run, so paragraph/item grouping sees
// across nested-sequence boundaries.
func (s *session) flattenAndRealize(children []value.Content) []value.Content {
	var out []value.Content
	for _, ch := range children {
		switch c := ch.(type) {
		case *value.Styled:
			appendFlat(&out, s.resolveStyled(c, s.realize))
		case *value.Sequence:
			if c.Label == nil {
				out = append(out, s.flattenAndRealize(c.Children)...)
			} else {
				out = append(out, s.realize(c))
			}
		default:
			out = append(out, s.realize(ch))
		}
	}
	return out
}

func appendFlat(out *[]value.Content, c value.Content) {
	if seq, ok := c.(*value.Sequence); ok && seq.Label == nil {
		*out = append(*out, seq.Children...)
		return
	}
	*out = append(*out, c)
}

// realize realizes one content node in block context: style scopes resolve,
// sequences group into paragraphs, and block bodies (list/enum/term items) are
// recursed into. Inline bodies (heading/strong/emph/link/par) use
// [session.realizeInline] so their content isn't wrapped in paragraphs.
func (s *session) realize(c value.Content) value.Content {
	switch c := c.(type) {
	case *value.Styled:
		return s.resolveStyled(c, s.realize)
	case *value.Sequence:
		return s.realizeBody(c.Children, c.Label)
	case *value.Heading:
		return &value.Heading{Depth: c.Depth, Body: s.realizeInlineBlock(c.Body), Label: c.Label}
	case *value.Strong:
		return &value.Strong{Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.Emph:
		return &value.Emph{Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.Link:
		return &value.Link{Dest: c.Dest, Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.Par:
		return &value.Par{Body: s.realizeInlineBlock(c.Body), Label: c.Label}
	case *value.ListItem:
		return &value.ListItem{Body: s.realize(c.Body), Label: c.Label}
	case *value.EnumItem:
		return &value.EnumItem{Number: c.Number, Body: s.realize(c.Body), Label: c.Label}
	case *value.TermItem:
		return &value.TermItem{Term: s.realize(c.Term), Description: s.realize(c.Description), Label: c.Label}
	case *value.HTMLElem:
		return &value.HTMLElem{Tag: c.Tag, Attrs: c.Attrs, Body: s.realizeHtmlBody(c), Block: c.Block, Label: c.Label}
	case *value.Equation:
		// A block equation's body ends where the block ends; an inline one sits
		// in the middle of a line, so its edges still separate words.
		return &value.Equation{Block: c.Block, Body: s.realizeInlineRun(c.Body, c.Block), Label: c.Label}
	case *value.List:
		items := make([]*value.ListItem, len(c.Children))
		for i, it := range c.Children {
			items[i] = s.realize(it).(*value.ListItem)
		}
		return &value.List{Children: items, Label: c.Label}
	case *value.Enum:
		items := make([]*value.EnumItem, len(c.Children))
		for i, it := range c.Children {
			items[i] = s.realize(it).(*value.EnumItem)
		}
		return &value.Enum{Children: items, Label: c.Label}
	case *value.Terms:
		items := make([]*value.TermItem, len(c.Children))
		for i, it := range c.Children {
			items[i] = s.realize(it).(*value.TermItem)
		}
		return &value.Terms{Children: items, Label: c.Label}
	case *value.Table:
		children := make([]value.Content, len(c.Children))
		for i, ch := range c.Children {
			children[i] = s.realize(ch)
		}
		return &value.Table{Columns: c.Columns, Children: children, Label: c.Label}
	case *value.TableHeader:
		children := make([]value.Content, len(c.Children))
		for i, ch := range c.Children {
			children[i] = s.realize(ch)
		}
		return &value.TableHeader{Children: children, Label: c.Label}
	case *value.Footnote:
		return s.realizeFootnote(c)
	case *value.Ref:
		return &value.Ref{Target: c.Target, Supplement: mapOpt(c.Supplement, s.realizeInline), Label: c.Label}
	default:
		// Leaves: Text, Raw, Linebreak, Parbreak, Ref, …
		return c
	}
}

// realizeInline realizes inline content whose run sits inside a line: a
// strong/emph/link body, or a sequence about to be spliced into a surrounding
// run. Its edge whitespace is kept, because it still separates words —
// `#emph[Hello ]world` reads "Hello world".
func (s *session) realizeInline(c value.Content) value.Content {
	return s.realizeInlineRun(c, false)
}

// realizeInlineBlock is [session.realizeInline] for a body whose ends are block
// boundaries — a heading or paragraph body — where edge whitespace has nothing
// left to separate.
func (s *session) realizeInlineBlock(c value.Content) value.Content {
	return s.realizeInlineRun(c, true)
}

// realizeInlineRun realizes inline content without forming paragraphs: it
// resolves style scopes and recurses into wrapper bodies but leaves sequences
// flat. edges says whether the run this builds ends on block boundaries; it
// reaches the run's own ends through [trimRun] and [trimEdges].
func (s *session) realizeInlineRun(c value.Content, edges bool) value.Content {
	switch c := c.(type) {
	case *value.Styled:
		return s.resolveStyled(c, func(body value.Content) value.Content {
			return s.realizeInlineRun(body, edges)
		})
	case *value.Sequence:
		var children []value.Content
		for _, ch := range c.Children {
			// A nested sequence is spliced into this run, so its own ends are
			// not edges: realizeInline, never realizeInlineBlock.
			appendFlat(&children, s.realizeInline(ch))
		}
		return &value.Sequence{Children: trimRun(mergeText(children), edges, edges), Label: c.Label}
	case *value.Footnote:
		// Opaque to the edges either way: a footnote is a mark on the line, and
		// the body behind it is a run of its own.
		return s.realizeFootnote(c)
	default:
		// A run of one: this element is its own first and last item, so a block
		// edge reaches straight into it.
		r, keep := trimEdges(mapChildren(c, s.realizeInline), edges, edges)
		if !keep {
			return &value.Sequence{}
		}
		return r
	}
}

// realizeHtmlBody realizes an HTML element's body in the context its tag calls
// for. A tag whose content model is flow content holds paragraphs, so its body
// is realized the way the document body is; everything else holds a single
// line's worth of content, and realizing it as flow would nest a paragraph
// inside markup that cannot have one — `p` being the case that gives the rule
// away. The tag is what decides this, not `block:`, which cannot turn a
// phrasing body into a flow one; see [value.HtmlTagFlow]. It can rule one out,
// though: an element forced inline sits inside a paragraph, and a paragraph
// nested in that one would be markup no browser accepts.
func (s *session) realizeHtmlBody(c *value.HTMLElem) value.Content {
	if c.Body == nil {
		return nil
	}
	if c.Block && value.HtmlTagFlow(c.Tag) {
		return s.realizeBody(topChildren(c.Body), nil)
	}
	// The element's own edges are where its body ends, so edge whitespace has
	// nothing left to separate — realizeInlineBlock, as for a heading.
	return s.realizeInlineBlock(c.Body)
}

// realizeFootnote realizes a footnote's body as a block body. An endnote is a
// document of its own — it is printed away from the line that cites it — so
// paragraphs form in it no matter where the citation sits.
//
// The same footnote reached twice realizes to the same element, not to two
// equal ones: `#let n = footnote[…]` used in two places is one footnote, and a
// presenter tells that from identity.
func (s *session) realizeFootnote(c *value.Footnote) value.Content {
	if r, ok := s.footnotes[c]; ok {
		return r
	}
	r := &value.Footnote{Body: s.realizeBody(topChildren(c.Body), nil), Label: c.Label}
	if s.footnotes == nil {
		s.footnotes = make(map[*value.Footnote]value.Content)
	}
	s.footnotes[c] = r
	return r
}

// groupContent groups a flat run of realized siblings into paragraphs and item
// containers: maximal runs of item nodes become a List/Enum/Terms, runs of
// inline content delimited by parbreaks or block content become a Par, and
// parbreak separators are dropped.
func groupContent(items []value.Content) []value.Content {
	var out []value.Content
	var para []value.Content
	flush := func() {
		if len(para) == 0 {
			return
		}
		// A run of nothing but invisible elements has no paragraph to make:
		// `#metadata(…) <x>` alone on a line would otherwise leave an empty
		// paragraph behind for the presenter to render. They stay where they
		// are, as siblings of the blocks around them, and the whitespace
		// between them goes too — with nothing visible on either side, it has
		// nothing left to separate.
		if !slices.ContainsFunc(para, isVisible) {
			for _, c := range para {
				if !spaceOnly(c) {
					out = append(out, c)
				}
			}
			para = nil
			return
		}
		out = append(out, &value.Par{Body: seqOf(para)})
		para = nil
	}
	for i := 0; i < len(items); {
		it := items[i]
		switch {
		case isParbreak(it):
			flush()
			i++
		case isItem(it):
			flush()
			i = appendItemRun(&out, items, i)
		case it.IsBlock():
			flush()
			out = append(out, it)
			i++
		default:
			para = append(para, it)
			i++
		}
	}
	flush()
	return out
}

// appendItemRun consumes the maximal run of consecutive same-kind item nodes
// starting at i, appends the grouped container to out, and returns the next
// index.
func appendItemRun(out *[]value.Content, items []value.Content, i int) int {
	switch items[i].(type) {
	case *value.ListItem:
		var run []*value.ListItem
		for ; i < len(items); i++ {
			it, ok := items[i].(*value.ListItem)
			if !ok {
				break
			}
			run = append(run, it)
		}
		*out = append(*out, &value.List{Children: run})
	case *value.EnumItem:
		var run []*value.EnumItem
		for ; i < len(items); i++ {
			it, ok := items[i].(*value.EnumItem)
			if !ok {
				break
			}
			run = append(run, it)
		}
		*out = append(*out, &value.Enum{Children: run})
	case *value.TermItem:
		var run []*value.TermItem
		for ; i < len(items); i++ {
			it, ok := items[i].(*value.TermItem)
			if !ok {
				break
			}
			run = append(run, it)
		}
		*out = append(*out, &value.Terms{Children: run})
	}
	return i
}

// mergeText concatenates adjacent unlabeled Text items and collapses the
// whitespace that markup composition duplicates at their seam. A markup space
// is an ordinary Text holding " " (see the analyzer's lowering of
// syntax.KindSpace), so this is where `[*Hello* ] + [world!]` gets its single
// separating space. Dropping the whitespace that has nothing to separate is
// [trimRun]'s job; run the two in that order.
//
// Whitespace inside a single Text value is never touched: an explicit
// #text("a    b") keeps its spacing.
func mergeText(items []value.Content) []value.Content {
	out := make([]value.Content, 0, len(items))
	for _, it := range items {
		if text, ok := mergeable(it); ok && len(out) > 0 {
			if prev, ok := mergeable(out[len(out)-1]); ok {
				// Both sides contribute whitespace at the seam: keep one.
				if endsWithSpace(prev.Text) {
					text = &value.Text{Text: trimLeftSpace(text.Text)}
				}
				out[len(out)-1] = &value.Text{Text: prev.Text + text.Text}
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

// trimRun drops the whitespace in a merged run that has nothing to separate:
// a space beside content that breaks the line anyway, and — where left or right
// is set — a space at that end of the run. Both are true for a run whose ends
// really are block boundaries (the document body, a list item, a heading or
// paragraph body) and false for one that sits inside a line, where the trailing
// space of `#emph[Hello ]world` still separates two words. They differ only for
// a wrapper body that sits on one edge but not the other.
//
// A block edge trims whatever lands on it — see [trimEdges]; only the
// whitespace *inside* a Text is off limits.
//
// trimRun filters in place, which is why it takes the run rather than being
// folded into [mergeText]: every caller hands it a freshly built slice.
func trimRun(items []value.Content, left, right bool) []value.Content {
	kept := items[:0]
	l := left
	for i, it := range items {
		// edgeRight looks ahead into slots this loop has not written yet, so it
		// reads the run as it came in — which is what it is asking about.
		it, keep := trimEdges(it, l, edgeRight(items, i, right))
		if !keep {
			// A space that trimmed away leaves the edge where it was, so the
			// item behind it lands on the edge in its turn.
			continue
		}
		l = breaksLine(it)
		kept = append(kept, it)
	}
	return kept
}

// edgeRight reports whether items[i] ends on an edge: the run's own end when
// right is set, a neighbor that breaks the line, or nothing but space that is
// about to trim away on the same edge. [mergeText] leaves at most one Text
// between two other items, so the scan takes a step or two.
func edgeRight(items []value.Content, i int, right bool) bool {
	for j := i + 1; j < len(items); j++ {
		if breaksLine(items[j]) || isFootnote(items[j]) {
			return true
		}
		if !spaceOnly(items[j]) {
			return false
		}
	}
	return right
}

// spaceOnly reports whether c is an unlabeled Text of nothing but whitespace —
// one that leaves the run entirely once an edge trims it.
func spaceOnly(c value.Content) bool {
	text, ok := mergeable(c)
	return ok && strings.TrimFunc(text.Text, unicode.IsSpace) == ""
}

// trimEdges trims the whitespace on whichever sides of c are marked by left and
// right, descending through the inline wrappers a block edge sees straight
// through: the space in `#strong[ x ]` alone in a heading has as little left to
// separate as the space in a bare ` x `. keep is false when c was a Text that
// trimmed away to nothing and leaves the run.
func trimEdges(c value.Content, left, right bool) (_ value.Content, keep bool) {
	if !left && !right {
		return c, true
	}
	switch c := c.(type) {
	case *value.Text:
		trimmed := c.Text
		if left {
			trimmed = trimLeftSpace(trimmed)
		}
		if right {
			trimmed = trimRightSpace(trimmed)
		}
		if trimmed == "" && c.Label == nil {
			return nil, false
		}
		if trimmed == c.Text {
			return c, true
		}
		return &value.Text{Text: trimmed, Label: c.Label}, true
	case *value.Strong:
		return &value.Strong{Body: trimBody(c.Body, left, right), Label: c.Label}, true
	case *value.Emph:
		return &value.Emph{Body: trimBody(c.Body, left, right), Label: c.Label}, true
	case *value.Link:
		return &value.Link{Dest: c.Dest, Body: trimBody(c.Body, left, right), Label: c.Label}, true
	}
	// Everything else is opaque to the edge: an inline equation keeps its own
	// spacing, and a leaf has nothing to trim.
	return c, true
}

// trimBody trims the edges of a wrapper's realized body, which is a run in its
// own right: a sequence hands its children back to [trimRun], anything else is
// a run of one. A body that trims away to nothing leaves the wrapper empty
// rather than dropping it — `#strong[ ]` on an edge is still a strong.
func trimBody(c value.Content, left, right bool) value.Content {
	if seq, ok := c.(*value.Sequence); ok {
		return &value.Sequence{Children: trimRun(seq.Children, left, right), Label: seq.Label}
	}
	if trimmed, keep := trimEdges(c, left, right); keep {
		return trimmed
	}
	return &value.Sequence{}
}

// mergeable reports whether c is a Text that may be concatenated with an
// adjacent one. A labeled Text stands on its own: the label names it.
func mergeable(c value.Content) (*value.Text, bool) {
	text, ok := c.(*value.Text)
	if !ok || text.Label != nil {
		return nil, false
	}
	return text, true
}

// breaksLine reports whether c ends the line it sits on, making adjacent
// whitespace pointless. That is every block element, plus the two breaks, which
// end a line without being content on it.
func breaksLine(c value.Content) bool {
	return breakKinds.Contains(c.Kind()) || c.IsBlock()
}

// breakKinds are the two elements that end a line without being content on it.
var breakKinds = value.SetOf(value.KindParbreak, value.KindLinebreak)

func isFootnote(c value.Content) bool { return c.Kind() == value.KindFootnote }

func endsWithSpace(s string) bool {
	r, size := utf8.DecodeLastRuneInString(s)
	return size > 0 && unicode.IsSpace(r)
}

func trimLeftSpace(s string) string  { return strings.TrimLeftFunc(s, unicode.IsSpace) }
func trimRightSpace(s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) }

func isParbreak(c value.Content) bool { return c.Kind() == value.KindParbreak }

// isVisible reports whether c is an element a reader can see. The introspection
// elements are not: they ride the document to be found again — by
// znkr.io/markst.Query, or by whatever comes to resolve state — and produce no
// output where they sit.
func isVisible(c value.Content) bool {
	if introspectionKinds.Contains(c.Kind()) {
		return false
	}
	// Whitespace is not visible on its own account: it is only ever there to
	// separate what is around it. Two metadata on consecutive lines have a
	// markup space between them that separates nothing.
	return !spaceOnly(c)
}

// introspectionKinds are the elements that ride the document to be found
// again rather than to be read.
var introspectionKinds = value.SetOf(value.KindMetadata, value.KindStateUpdate)

// isItem reports whether c is one of the item elements. Items are block content
// like any other ([value.Content.IsBlock]); this narrower question is only
// [groupContent]'s, which must gather a run of them into a container instead of
// emitting them one by one, and so asks it first.
func isItem(c value.Content) bool { return itemKinds.Contains(c.Kind()) }

// itemKinds are the elements a run of which groups into a container.
var itemKinds = value.SetOf(value.KindListItem, value.KindEnumItem, value.KindTermItem)

// resolveStyled realizes a style scope: it realizes the body, applies the
// recorded set rules and show recipes to it, and returns the result with the
// wrapper gone. realize is the caller's own realization step, so that a scope
// reached from inline content (a math style function, say) doesn't have its
// body grouped into paragraphs.
func (s *session) resolveStyled(t *value.Styled, realize func(value.Content) value.Content) value.Content {
	body := realize(t.Body)
	for _, set := range t.Sets {
		body = s.applySet(body, set)
	}
	for _, r := range t.Recipes {
		if r.Selector == nil {
			body = s.applyTransform(body, r.Transform)
		} else {
			body = s.applyRecipe(body, r)
		}
	}
	return body
}

// applySet applies a set rule to realized content. Two kinds take effect: a
// `document` set, whose properties are hoisted into the session, and the math
// font styles, which are folded into the math leaves they reach (see
// applyMathStyle). Other element sets (e.g. `set heading(level: …)`) are
// realized away with no effect: our content model tracks no "unset" state for
// their properties and styles don't propagate top-down, so applying a default
// would wrongly overwrite explicit values (and nest in the wrong order). See
// IDEAS.md.
//
// The root is a single element, so a `set document` is never scoped to the block
// it appears in. When two rules name the same property, the one in the outer
// scope wins: a set rule scopes over its following siblings, so a later rule
// nests inside an earlier one, and resolveStyled realizes a body before applying
// its own sets. That is the order `set document(title:)` has always had.
func (s *session) applySet(c value.Content, set *value.Set) value.Content {
	switch set.Element.Name {
	case "document":
		// The argument types were checked when the set rule was bound, so a
		// value of the wrong shape here is a bug rather than bad input, and
		// there is no span left to report it against: leave the field alone.
		if v, ok := set.Fields.Get(names.Title); ok {
			if title, ok := v.(value.Str); ok {
				s.doc.Title = string(title)
			}
		}
		if d, ok := documentDate(set, names.Date); ok {
			s.doc.Date = &d
		}
	case "math.equation":
		return applyMathStyle(c, set)
	}
	return c
}

// documentDate reads a date-valued `document` property, which is either a
// datetime or a `yyyy-mm-dd` string. The second result is false when the set
// rule doesn't name the property at all.
func documentDate(set *value.Set, n name.Name) (value.Datetime, bool) {
	v, ok := set.Fields.Get(n)
	if !ok {
		return value.Datetime{}, false
	}
	switch v := v.(type) {
	case value.Datetime:
		return v, true
	case value.Str:
		return value.ParseDate(string(v))
	}
	return value.Datetime{}, false
}

// applyMathStyle folds the font-style properties of a `math.equation` set into
// every [value.MathText] in c — the leaves that carry a math font style. This is
// what makes `$bold(x)$` observable; the equation's other properties (block) are
// left to the general set behaviour described on applySet.
//
// A property already set on a leaf is left alone, so the nearest style function
// wins: resolveStyled realizes a body before applying its own sets, so in
// `sans(frak(x))` the inner frak lands first.
func applyMathStyle(c value.Content, set *value.Set) value.Content {
	bold, hasBold := set.Fields.Get(names.Bold)
	italic, hasItalic := set.Fields.Get(names.Italic)
	variant, hasVariant := set.Fields.Get(names.Variant)
	if !hasBold && !hasItalic && !hasVariant {
		return c
	}
	var fold func(value.Content) value.Content
	fold = func(c value.Content) value.Content {
		t, ok := c.(*value.MathText)
		if !ok {
			return mapChildren(c, fold)
		}
		styled := *t
		if hasBold && styled.Bold == nil {
			styled.Bold = bold
		}
		if hasItalic && styled.Italic == nil {
			styled.Italic = italic
		}
		if hasVariant && styled.Variant == nil {
			styled.Variant = variant
		}
		return &styled
	}
	return fold(c)
}

// applyRecipe replaces every node matching r.Selector with r's transform; non-
// matching nodes recurse. Replacements are not re-matched.
func (s *session) applyRecipe(c value.Content, r *value.Recipe) value.Content {
	if r.Selector.Match(c) {
		return s.applyTransform(c, r.Transform)
	}
	return mapChildren(c, func(ch value.Content) value.Content {
		return s.applyRecipe(ch, r)
	})
}

// applyTransform produces the realized replacement for a matched node: a content
// transform replaces directly; a function/element transform is called with the
// node as its single argument.
func (s *session) applyTransform(node value.Content, transform value.Value) value.Content {
	switch t := transform.(type) {
	case *value.Function:
		return s.callTransform(t, node)
	case *value.Element:
		return s.callTransform(&t.Function, node)
	case value.Content:
		return s.realize(t)
	}
	return node
}

func (s *session) callTransform(fn *value.Function, node value.Content) value.Content {
	// Realization has no spans left to point at, so there is no call site to
	// push; Runtime still has to be set, or a show rule whose transform is a
	// user closure would have no session to run against.
	fcc := value.FunctionCallContext{Now: s.now, Runtime: s}
	res, err := fn.Apply(&fcc, &value.Arguments{Positional: []value.Value{node}})
	if err != nil {
		s.recordError(&value.Error{Span: syntax.NoSpan, Msg: err.Error()})
		return node
	}
	c := value.ToContent(res)
	if c == nil {
		return &value.Sequence{}
	}
	return s.realize(c)
}

// mapChildren returns a copy of c with f applied to each of its direct content
// children (bodies/items). Leaves are returned unchanged.
func mapChildren(c value.Content, f func(value.Content) value.Content) value.Content {
	switch c := c.(type) {
	case *value.Sequence:
		return &value.Sequence{Children: mapEach(c.Children, f), Label: c.Label}
	case *value.Heading:
		return &value.Heading{Depth: c.Depth, Body: f(c.Body), Label: c.Label}
	case *value.Strong:
		return &value.Strong{Body: f(c.Body), Label: c.Label}
	case *value.Emph:
		return &value.Emph{Body: f(c.Body), Label: c.Label}
	case *value.Underline:
		return &value.Underline{Body: f(c.Body), Label: c.Label}
	case *value.Par:
		return &value.Par{Body: f(c.Body), Label: c.Label}
	case *value.Link:
		return &value.Link{Dest: c.Dest, Body: f(c.Body), Label: c.Label}
	case *value.Equation:
		return &value.Equation{Block: c.Block, Body: f(c.Body), Label: c.Label}
	case *value.HTMLElem:
		if c.Body == nil {
			return c
		}
		return &value.HTMLElem{Tag: c.Tag, Attrs: c.Attrs, Body: f(c.Body), Block: c.Block, Label: c.Label}
	case *value.Document:
		// Copied wholesale rather than field by field: the root carries
		// document properties that have nothing to do with the rewrite, and a
		// property added later must not go missing here.
		d := *c
		d.Body = f(c.Body)
		return &d
	case *value.ListItem:
		return &value.ListItem{Body: f(c.Body), Label: c.Label}
	case *value.EnumItem:
		return &value.EnumItem{Number: c.Number, Body: f(c.Body), Label: c.Label}
	case *value.TermItem:
		return &value.TermItem{Term: f(c.Term), Description: f(c.Description), Label: c.Label}
	case *value.List:
		items := make([]*value.ListItem, len(c.Children))
		for i, it := range c.Children {
			items[i] = f(it).(*value.ListItem)
		}
		return &value.List{Children: items, Label: c.Label}
	case *value.Enum:
		items := make([]*value.EnumItem, len(c.Children))
		for i, it := range c.Children {
			items[i] = f(it).(*value.EnumItem)
		}
		return &value.Enum{Children: items, Label: c.Label}
	case *value.Terms:
		items := make([]*value.TermItem, len(c.Children))
		for i, it := range c.Children {
			items[i] = f(it).(*value.TermItem)
		}
		return &value.Terms{Children: items, Label: c.Label}
	case *value.Table:
		return &value.Table{Columns: c.Columns, Children: mapEach(c.Children, f), Label: c.Label}
	case *value.TableHeader:
		return &value.TableHeader{Children: mapEach(c.Children, f), Label: c.Label}
	case *value.Footnote:
		return &value.Footnote{Body: f(c.Body), Label: c.Label}
	case *value.Ref:
		return &value.Ref{Target: c.Target, Supplement: mapOpt(c.Supplement, f), Label: c.Label}
	case *value.MathOp:
		return &value.MathOp{Text: f(c.Text), Limits: c.Limits, Label: c.Label}
	case *value.MathAttach:
		return &value.MathAttach{Base: f(c.Base), Top: mapOpt(c.Top, f), Bottom: mapOpt(c.Bottom, f), Label: c.Label}
	case *value.MathFrac:
		return &value.MathFrac{Num: f(c.Num), Denom: f(c.Denom), Label: c.Label}
	case *value.MathRoot:
		return &value.MathRoot{Index: mapOpt(c.Index, f), Radicand: f(c.Radicand), Label: c.Label}
	case *value.MathPrimes:
		return &value.MathPrimes{Base: f(c.Base), Count: c.Count, Label: c.Label}
	case *value.MathLr:
		return &value.MathLr{Body: f(c.Body), Size: c.Size, Label: c.Label}
	case *value.MathMid:
		return &value.MathMid{Body: f(c.Body), Label: c.Label}
	case *value.MathUnderline:
		return &value.MathUnderline{Body: f(c.Body), Label: c.Label}
	case *value.MathAccent:
		return &value.MathAccent{Base: f(c.Base), Accent: c.Accent, Size: c.Size, Dotless: c.Dotless, Label: c.Label}
	case *value.MathCancel:
		return &value.MathCancel{Body: f(c.Body), Angle: c.Angle, Label: c.Label}
	case *value.MathVec:
		return &value.MathVec{Children: mapEach(c.Children, f), Label: c.Label}
	case *value.MathCases:
		return &value.MathCases{Children: mapEach(c.Children, f), Label: c.Label}
	case *value.MathMat:
		rows := make([][]value.Content, len(c.Rows))
		for i, r := range c.Rows {
			rows[i] = mapEach(r, f)
		}
		return &value.MathMat{Rows: rows, Label: c.Label}
	}
	return c
}

func mapEach(children []value.Content, f func(value.Content) value.Content) []value.Content {
	out := make([]value.Content, len(children))
	for i, ch := range children {
		out[i] = f(ch)
	}
	return out
}

// mapOpt applies f to c unless c is nil, in which case it returns nil. Used for
// optional content fields.
func mapOpt(c value.Content, f func(value.Content) value.Content) value.Content {
	if c == nil {
		return nil
	}
	return f(c)
}
