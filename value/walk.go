package value

import "iter"

// All iterates c and everything below it in document order: c first, then its
// children depth-first, left to right. It is the walk introspection is built
// on — see znkr.io/writst.Query — and it reaches every element of a realized
// document, invisible ones ([Metadata], [StateUpdate]) included.
//
// The traversal allocates nothing per element: each node's generated walk hands
// the same yield function to its children rather than collecting them.
//
// What it does not descend into is a value that merely happens to be content:
// [Metadata.Value] is data the document carries, not part of the document, so
// metadata nested in another metadata's value stays out of reach.
func All(c Content) iter.Seq[Content] {
	return func(yield func(Content) bool) {
		if c != nil {
			c.walk(yield)
		}
	}
}

// The hand-written content nodes ([StateUpdate], [StyleUpdate], [Styled]) opt
// out of the generator, so they carry their own walk. The two updates are
// leaves; a Styled wraps the content it scopes over.

func (n *StateUpdate) walk(yield func(Content) bool) bool { return yield(n) }
func (n *StyleUpdate) walk(yield func(Content) bool) bool { return yield(n) }

func (n *Styled) walk(yield func(Content) bool) bool {
	if !yield(n) {
		return false
	}
	if n.Body == nil {
		return true
	}
	return n.Body.walk(yield)
}
