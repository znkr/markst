package value

import (
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

// Label is a named marker that can be attached to content elements for
// cross-referencing (e.g. <intro> in Writst markup).
type Label struct {
	Name name.Name

	// Auto reports that the label was derived from the element's text during
	// realization rather than written in the source. Realization labels every
	// heading that the author left unlabelled, so that a presenter always has
	// an anchor to link a section by; such a label is only as stable as the
	// text it was derived from, and changes when the heading is reworded.
	//
	// A generated label is not part of the document's namespace: `@ref` and
	// `#show <x>:` see the labels the source wrote, and nothing else.
	Auto bool
}

func (Label) aValue()           {}
func (*Label) Type() types.Type { return types.Label }
