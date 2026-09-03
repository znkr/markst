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

package value

import (
	"iter"
	"slices"
)

// Preorder iterates c and everything below it in document order: c first, then
// its children depth-first, left to right. It yields a [Cursor] for each
// element whose kind is in kinds. Elements outside kinds are still descended
// into; kinds selects what the caller is given, not what is traversed.
//
// Every element of a realized document is visited, including the ones that
// produce no output ([Metadata], [StateUpdate]). [Metadata.Value] is not
// descended into: it is a value the document carries, not part of the document,
// so metadata inside another metadata's value is not visited.
//
// Breaking out of the loop stops the walk. To skip one subtree and continue,
// call [Cursor.SkipChildren]:
//
//	for c := range value.Preorder(body, kinds) {
//		if c.Kind() == value.KindFootnote {
//			c.SkipChildren() // a footnote's text is not part of the title
//			continue
//		}
//		…
//	}
//
// The walk allocates nothing per element.
func Preorder(c Content, kinds KindSet) iter.Seq[Cursor] {
	return func(yield func(Cursor) bool) {
		if c == nil {
			return
		}
		w := &walker{yield: yield, kinds: kinds}
		w.stack = w.inline[:0]
		c.inspect(w)
	}
}

// walker holds the state of one Preorder walk: the caller's yield function, the
// kinds to select, and the stack of open elements that a cursor reports as its
// ancestors.
type walker struct {
	yield func(Cursor) bool
	kinds KindSet
	stack []Content
	// inline is the stack's initial storage. A walk over a document less than
	// 8 levels deep allocates only the walker.
	inline  [8]Content
	stopped bool
	skip    bool
}

// push opens an element, making it the current one and an ancestor of
// everything visited before it is popped. Every element is pushed, including
// those kinds does not select: kinds controls what the caller is given, not
// what an element's ancestors are.
func (w *walker) push(n Content) { w.stack = append(w.stack, n) }

// pop closes the element on top of the stack, once its children are done.
func (w *walker) pop() { w.stack = w.stack[:len(w.stack)-1] }

// visit yields the element on top of the stack and reports whether to descend
// into it. False covers two cases, stopping the walk and pruning this subtree;
// [walker.live] distinguishes them.
//
// The kind test is in the generated descent rather than here, where the kind is
// a constant and the test compiles to one bit test.
func (w *walker) visit() bool {
	if !w.yield(Cursor{w, len(w.stack) - 1}) {
		w.stopped = true
	}
	if w.stopped || w.skip {
		w.skip = false
		w.stack = w.stack[:len(w.stack)-1]
		return false
	}
	return true
}

// live reports whether the walk is still running. An element whose subtree was
// pruned returns this to its parent.
func (w *walker) live() bool { return !w.stopped }

// Cursor is one visited element together with its ancestors. [Preorder] yields
// one per visited element.
//
// A cursor is valid only inside the loop body it was yielded to: its ancestors
// are read from the walk's stack, which unwinds as the walk continues. Use
// [Cursor.Path] to copy the ancestors out. The element from [Cursor.Node] is
// safe to retain.
type Cursor struct {
	w *walker
	i int
}

// Node returns the element the cursor is on.
func (c Cursor) Node() Content { return c.at(c.i) }

// Kind returns the element's kind, avoiding the type switch [Cursor.Node] would
// require.
func (c Cursor) Kind() ElemKind { return c.at(c.i).Kind() }

// Depth returns the number of enclosing elements: 0 for the element the walk
// started at, 1 for its children, and so on. All enclosing elements count,
// including those the walk's kinds did not select.
func (c Cursor) Depth() int { return c.i }

// Parent returns the enclosing element, and false at the element the walk
// started at. The parent need not be one of the kinds the walk selected, so it
// may be an element the loop was never given.
func (c Cursor) Parent() (Cursor, bool) {
	if c.i == 0 {
		return Cursor{}, false
	}
	return Cursor{c.w, c.i - 1}, true
}

// Enclosing iterates the cursor's element and its ancestors, innermost first,
// keeping those whose kind is in kinds. Use it for questions about context that
// document order cannot answer: which heading some content is under, or whether
// it is inside a footnote.
func (c Cursor) Enclosing(kinds KindSet) iter.Seq[Cursor] {
	return func(yield func(Cursor) bool) {
		for i := c.i; i >= 0; i-- {
			if !kinds.Contains(c.w.stack[i].Kind()) {
				continue
			}
			if !yield(Cursor{c.w, i}) {
				return
			}
		}
	}
}

// Path returns the elements from the one the walk started at down to the
// cursor's own, as a slice the caller owns. Unlike the cursor, it stays valid
// after the walk continues.
func (c Cursor) Path() []Content {
	return slices.Clone(c.w.stack[:c.i+1])
}

// SkipChildren stops the walk descending into the cursor's element; it
// continues with what follows. It applies only to the element being visited, so
// it panics on a cursor from [Cursor.Parent] or one kept past its loop body.
func (c Cursor) SkipChildren() {
	if c.i != len(c.w.stack)-1 {
		panic("value: SkipChildren called on a cursor other than the one being visited")
	}
	c.w.skip = true
}

func (c Cursor) at(i int) Content {
	if i >= len(c.w.stack) {
		panic("value: cursor used after its walk moved on")
	}
	return c.w.stack[i]
}

// Preorder walks the document element and then its body, in document order. It
// is [Preorder] over n, and it is what makes a [Document] a [Tree].
func (n *Document) Preorder(kinds KindSet) iter.Seq[Cursor] {
	if n == nil {
		return func(func(Cursor) bool) {}
	}
	return Preorder(n, kinds)
}

// The hand-written content nodes ([StateUpdate], [StyleUpdate], [Styled],
// [Custom]) are not produced by the generator, so their kind and descent are
// written here. All but Styled are leaves; Styled has the content it applies
// to as its child.

func (n *StateUpdate) Kind() ElemKind { return KindStateUpdate }
func (n *StyleUpdate) Kind() ElemKind { return KindStyleUpdate }
func (n *Custom) Kind() ElemKind      { return KindCustom }
func (n *Styled) Kind() ElemKind      { return KindStyled }

func (n *StateUpdate) inspect(w *walker) bool { return leaf(w, n, KindStateUpdate) }
func (n *StyleUpdate) inspect(w *walker) bool { return leaf(w, n, KindStyleUpdate) }
func (n *Custom) inspect(w *walker) bool      { return leaf(w, n, KindCustom) }

func (n *Styled) inspect(w *walker) bool {
	w.push(n)
	if w.kinds.Contains(KindStyled) && !w.visit() {
		return w.live()
	}
	if n.Body != nil && !n.Body.inspect(w) {
		return false
	}
	w.pop()
	return true
}

func leaf(w *walker, n Content, k ElemKind) bool {
	w.push(n)
	if w.kinds.Contains(k) && !w.visit() {
		return w.live()
	}
	w.pop()
	return true
}
