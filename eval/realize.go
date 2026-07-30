package eval

import (
	"znkr.io/writst/internal/names"
	"znkr.io/writst/name"
	"znkr.io/writst/value"
)

// realizeDocument turns the recorded content tree into a realized
// [value.Document]: paragraphs are formed, list/enum/term items grouped,
// `*value.Templated` wrappers resolved (show recipes applied, `set document`
// title hoisted). It runs inside [Eval] while the session — and any closures
// captured by show transforms — are still live.
func (s *session) realizeDocument(c value.Content) value.Content {
	body := s.realizeBody(topChildren(c), nil)
	return &value.Document{Title: s.docTitle, Body: body}
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
	r := seqOf(groupContent(s.flattenAndRealize(children)))
	if label != nil {
		r.SetLabel(label)
	}
	return r
}

// flattenAndRealize realizes each child and splices unlabeled sequences (and
// resolved templates) into a single flat run, so paragraph/item grouping sees
// across nested-sequence boundaries.
func (s *session) flattenAndRealize(children []value.Content) []value.Content {
	var out []value.Content
	for _, ch := range children {
		switch c := ch.(type) {
		case *value.Templated:
			appendFlat(&out, s.resolveTemplated(c))
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

// realize realizes one content node in block context: templates resolve,
// sequences group into paragraphs, and block bodies (list/enum/term items) are
// recursed into. Inline bodies (heading/strong/emph/link/par) use
// [session.realizeInline] so their content isn't wrapped in paragraphs.
func (s *session) realize(c value.Content) value.Content {
	switch c := c.(type) {
	case *value.Templated:
		return s.resolveTemplated(c)
	case *value.Sequence:
		return s.realizeBody(c.Children, c.Label)
	case *value.Heading:
		return &value.Heading{Depth: c.Depth, Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.Strong:
		return &value.Strong{Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.Emph:
		return &value.Emph{Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.Link:
		return &value.Link{Dest: c.Dest, Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.Par:
		return &value.Par{Body: s.realizeInline(c.Body), Label: c.Label}
	case *value.ListItem:
		return &value.ListItem{Body: s.realize(c.Body), Label: c.Label}
	case *value.EnumItem:
		return &value.EnumItem{Number: c.Number, Body: s.realize(c.Body), Label: c.Label}
	case *value.TermItem:
		return &value.TermItem{Term: s.realize(c.Term), Description: s.realize(c.Description), Label: c.Label}
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
		return &value.Table{Children: children, Label: c.Label}
	default:
		// Leaves: Text, Raw, Linebreak, Parbreak, Ref, …
		return c
	}
}

// realizeInline realizes inline content without forming paragraphs: it resolves
// templates and recurses into wrapper bodies but leaves sequences flat.
func (s *session) realizeInline(c value.Content) value.Content {
	switch c := c.(type) {
	case *value.Templated:
		return s.resolveTemplated(c)
	case *value.Sequence:
		children := make([]value.Content, len(c.Children))
		for i, ch := range c.Children {
			children[i] = s.realizeInline(ch)
		}
		return &value.Sequence{Children: children, Label: c.Label}
	default:
		return mapChildren(c, s.realizeInline)
	}
}

// groupContent groups a flat run of realized siblings into paragraphs and item
// containers: maximal runs of item nodes become a List/Enum/Terms, runs of
// inline content delimited by parbreaks or block content become a Par, and
// parbreak separators are dropped.
func groupContent(items []value.Content) []value.Content {
	var out []value.Content
	var para []value.Content
	flush := func() {
		if len(para) > 0 {
			out = append(out, &value.Par{Body: seqOf(para)})
			para = nil
		}
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
		case isBlock(it):
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

func isParbreak(c value.Content) bool { _, ok := c.(*value.Parbreak); return ok }

func isItem(c value.Content) bool {
	switch c.(type) {
	case *value.ListItem, *value.EnumItem, *value.TermItem:
		return true
	}
	return false
}

// isBlock reports whether c is block-level content (breaks paragraph flow).
// Everything not block, not a parbreak, and not a loose item is inline.
func isBlock(c value.Content) bool {
	switch c := c.(type) {
	case *value.Heading, *value.List, *value.Enum, *value.Terms,
		*value.Par, *value.Table, *value.Document:
		return true
	case *value.Raw:
		return c.Block
	}
	return false
}

// resolveTemplated realizes a template scope: it realizes the body, applies the
// recorded set rules and show recipes to it, and returns the result with the
// wrapper gone.
func (s *session) resolveTemplated(t *value.Templated) value.Content {
	body := s.realize(t.Body)
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

// applySet applies a set rule to realized content. Only `document` sets take
// effect for now — the title is hoisted into the session. Element sets (e.g.
// `set heading(level: …)`) are realized away with no effect: our content model
// tracks no "unset" state and styles don't propagate top-down, so applying a
// default would wrongly overwrite explicit values (and nest in the wrong order).
// See the deferred notes.
func (s *session) applySet(c value.Content, set *value.Set) value.Content {
	if set.Element.Name == "document" {
		if v, ok := set.Fields.Get(names.Title); ok {
			if tc, err := value.ToContent(v); err == nil {
				s.docTitle = tc
			}
		}
	}
	return c
}

// applyRecipe replaces every node matching r.Selector with r's transform; non-
// matching nodes recurse. Replacements are not re-matched.
func (s *session) applyRecipe(c value.Content, r *value.Recipe) value.Content {
	if matchesSelector(c, r.Selector) {
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
		return s.callTransform((*value.Function)(t), node)
	case value.Content:
		return s.realize(t)
	}
	return node
}

func (s *session) callTransform(fn *value.Function, node value.Content) value.Content {
	res, err := fn.Apply(&value.FunctionCallContext{}, &value.Arguments{Positional: []value.Value{node}})
	if err != nil {
		s.recordError(&value.Error{Msg: err.Error()})
		return node
	}
	c, cerr := value.ToContent(res)
	if cerr != nil {
		s.recordError(&value.Error{Msg: cerr.Error()})
		return node
	}
	if c == nil {
		return &value.Sequence{}
	}
	return s.realize(c)
}

// matchesSelector reports whether c is selected by sel.
func matchesSelector(c value.Content, sel value.Selector) bool {
	switch sel := sel.(type) {
	case *value.ElementSelector:
		return c.Name() == sel.Element.Name
	case *value.LabelSelector:
		n, ok := contentLabel(c)
		return ok && n == sel.Label
	case *value.WhereSelector:
		if c.Name() != sel.Element.Name {
			return false
		}
		n, ok := contentLabel(c)
		return ok && n == sel.Label
	}
	return false
}

func contentLabel(c value.Content) (name.Name, bool) {
	if l, ok := c.Field(names.Label).(*value.Label); ok {
		return l.Name, true
	}
	return name.Name{}, false
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
	case *value.Par:
		return &value.Par{Body: f(c.Body), Label: c.Label}
	case *value.Link:
		return &value.Link{Dest: c.Dest, Body: f(c.Body), Label: c.Label}
	case *value.Document:
		return &value.Document{Title: c.Title, Body: f(c.Body)}
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
		return &value.Table{Children: mapEach(c.Children, f), Label: c.Label}
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
