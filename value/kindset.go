package value

// KindSet is a set of element kinds, held as one bit per kind. It is what a
// walk filters on: [Preorder] yields the elements whose kind the set contains
// and passes silently through the rest.
type KindSet uint64

// AnyKind is the set of every element kind, the mask for an unfiltered walk.
const AnyKind KindSet = 1<<numElemKinds - 1

// SetOf returns the set holding exactly kinds.
func SetOf(kinds ...ElemKind) KindSet {
	var s KindSet
	for _, k := range kinds {
		s |= 1 << k
	}
	return s
}

// Contains reports whether k is in s.
func (s KindSet) Contains(k ElemKind) bool { return s&(1<<k) != 0 }

// Add returns s with kinds added.
func (s KindSet) Add(kinds ...ElemKind) KindSet { return s | SetOf(kinds...) }

// Remove returns s without kinds — the way to say "everything except", starting
// from [AnyKind].
func (s KindSet) Remove(kinds ...ElemKind) KindSet { return s &^ SetOf(kinds...) }
