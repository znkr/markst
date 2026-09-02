package value

import (
	"iter"
	"slices"
)

// Preorder iterates c and everything below it in document order — c first, then
// its children depth-first, left to right — yielding a [Cursor] for each element
// whose kind is in kinds. Elements outside kinds are still descended into; the
// mask decides what is handed to the caller, not what is walked.
//
// It is the walk introspection is built on, and it reaches every element of a
// realized document, invisible ones ([Metadata], [StateUpdate]) included. What
// it does not descend into is a value that merely happens to be content:
// [Metadata.Value] is data the document carries, not part of the document, so
// metadata nested in another metadata's value stays out of reach.
//
// Breaking out of the loop stops the walk. To skip one subtree and carry on,
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
// The walk allocates nothing per element — each element's generated descent
// hands the same walker down rather than collecting anything along the way — so
// its cost grows with the document only in the elements it touches.
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

// walker carries one Preorder walk: the caller's yield function, the kind mask
// it filters on, and the chain of elements currently open, which is what gives a
// cursor its ancestors.
type walker struct {
	yield func(Cursor) bool
	kinds KindSet
	stack []Content
	// inline is the stack's initial storage, so a walk over a document of
	// ordinary depth allocates only the walker itself.
	inline  [8]Content
	stopped bool
	skip    bool
}

// push opens an element: it becomes the one a cursor is on, and an ancestor of
// everything visited until it is popped. Every element is pushed, whether or not
// the mask yields it, because the mask decides what the caller sees and not what
// the chain it sits in is.
func (w *walker) push(n Content) { w.stack = append(w.stack, n) }

// pop closes the element on top of the stack, once its children are done.
func (w *walker) pop() { w.stack = w.stack[:len(w.stack)-1] }

// visit offers the element on top of the stack to the caller and reports whether
// to descend into it. It is false both when the caller stopped the walk and when
// it pruned this subtree; [walker.live] tells the two apart. The generated
// descent calls it only when the mask matches, which is why the mask test is not
// here: the kind is a constant there, and the test folds into a single bit test.
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

// live reports whether the walk is still running, which is what an element
// whose subtree was pruned returns to its parent.
func (w *walker) live() bool { return !w.stopped }

// Cursor is one element of a walk, together with the chain of elements it sits
// under. [Preorder] yields one per visited element.
//
// A cursor is valid only inside the loop body it was yielded to: the ancestors
// it reports live on the walk's own stack, and that stack unwinds as the walk
// moves on. [Cursor.Path] is the escape for a caller that needs to keep the
// chain; [Cursor.Node] is safe to keep on its own, being just the element.
type Cursor struct {
	w *walker
	i int
}

// Node returns the element the cursor is on.
func (c Cursor) Node() Content { return c.at(c.i) }

// Kind returns the element's kind, without the type switch [Cursor.Node] would
// need.
func (c Cursor) Kind() ElemKind { return c.at(c.i).Kind() }

// Depth returns how many elements the cursor sits under: 0 for the element the
// walk started at, 1 for its children, and so on. It counts every enclosing
// element, whether or not the walk's mask yielded it.
func (c Cursor) Depth() int { return c.i }

// Parent returns the element the cursor's element sits directly in, and false
// at the element the walk started at. The parent is reported whether or not the
// walk's mask matches it, so it may be an element the loop never saw.
func (c Cursor) Parent() (Cursor, bool) {
	if c.i == 0 {
		return Cursor{}, false
	}
	return Cursor{c.w, c.i - 1}, true
}

// Enclosing iterates the cursor's element and the elements it sits in, from the
// element itself outwards, keeping those whose kind is in kinds. It answers
// questions about context — which heading a piece of content belongs to, whether
// anything above it is a footnote — that document order alone cannot.
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

// Path returns the chain of elements from the one the walk started at down to
// and including the cursor's own, as a slice the caller owns. Unlike the cursor
// it is taken from, it stays valid after the walk has moved on.
func (c Cursor) Path() []Content {
	return slices.Clone(c.w.stack[:c.i+1])
}

// SkipChildren prunes the cursor's element: the walk does not descend into it
// and carries on with what follows. It applies to the element being visited, so
// it panics on a cursor obtained from [Cursor.Parent] or kept past its loop body.
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

// The hand-written content nodes ([StateUpdate], [StyleUpdate], [Styled],
// [Custom]) opt out of the generator, so they carry their own kind and descent.
// All but Styled are leaves; a Styled wraps the content it scopes over.

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
