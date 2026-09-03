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

package parser

import (
	"fmt"

	"znkr.io/markst/syntax"
)

// asErrorNode converts a node into an error node with the given diagnostic
// message, preserving the original node's span and text. If the node is
// already an error, it is returned as-is.
func (p *parser) asErrorNode(node syntax.Node, format string, args ...any) *syntax.Error {
	if node.Kind() == syntax.KindError {
		return node.(*syntax.Error)
	}
	return p.a.Error(
		node.Span(),
		fmt.Sprintf(format, args...),
	)
}
