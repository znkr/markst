// Package name provides interned string identifiers.
package name

import "unique"

// Name is an interned string handle.
//
// Two Names are equal if and only if they were created from the same string,
// and comparison is O(1) pointer equality.
type Name unique.Handle[string]

var Invalid = Name{}

// Make interns s and returns its Name handle.
//
// Calling Make with the same string always returns an equal Name.
func Make(s string) Name {
	return Name(unique.Make(s))
}

// String returns the underlying string value of the Name.
func (n Name) String() string {
	return unique.Handle[string](n).Value()
}
