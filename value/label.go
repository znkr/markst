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
	"znkr.io/markst/name"
	"znkr.io/markst/types"
)

// Label is a named marker that can be attached to content elements for
// cross-referencing (e.g. <intro> in Markst markup).
type Label struct {
	Name name.Name

	// Auto reports that the label was derived from the element's text during
	// realization rather than written in the source. Realization labels every
	// heading that the author left unlabeled, so that a presenter always has
	// an anchor to link a section by; such a label is only as stable as the
	// text it was derived from, and changes when the heading is reworded.
	//
	// A generated label is not part of the document's namespace: `@ref` and
	// `#show <x>:` see the labels the source wrote, and nothing else.
	Auto bool
}

func (Label) aValue()           {}
func (*Label) Type() types.Type { return types.Label }
