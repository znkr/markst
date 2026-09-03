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
	"slices"

	"znkr.io/markst/syntax"
)

// maxDepth is the deepest the parser nests before it gives up on the construct
// it is in. Without a limit, input that nests deeply enough — a few hundred
// thousand unclosed `{`, say — exhausts the goroutine stack, which kills the
// process rather than panicking. The limit is Typst's.
const maxDepth = 256

// depthExceededMsg is the diagnostic a parse that hits [maxDepth] reports.
const depthExceededMsg = "maximum parsing depth exceeded"

// noStops is the empty stop set. [parser.enterDepth] and [parser.checkDepth]
// take it to mean "fold a single token into the error" rather than "skip to
// the next stop token".
var noStops syntax.Set

var (
	openDelims  = syntax.SetOf(syntax.KindLeftParen, syntax.KindLeftBracket, syntax.KindLeftBrace)
	closeDelims = syntax.SetOf(syntax.KindRightParen, syntax.KindRightBracket, syntax.KindRightBrace)
)

// enterDepth descends one level of nesting, reporting whether the parser may
// go on. A true result must be paired with [parser.leaveDepth]. A false result
// means the limit is reached: the input the caller would have parsed has been
// replaced by an error node, and the caller must return without parsing
// anything itself.
func (p *parser) enterDepth(stops syntax.Set) bool {
	if !p.checkDepth(stops) {
		return false
	}
	p.depth++
	return true
}

// leaveDepth undoes one [parser.enterDepth]. The expression parsers call it at
// the end of the function instead of deferring it, which is worth a few percent
// of a parse.
func (p *parser) leaveDepth() { p.depth-- }

// checkDepth reports whether the parser is within the nesting limit, without
// descending itself; it reports the same error as [parser.enterDepth] when it
// is not. A parser of a sequence uses it to give up on the whole sequence,
// which turns one error per element into one error for the sequence.
func (p *parser) checkDepth(stops syntax.Set) bool {
	if p.depth < maxDepth {
		return true
	}
	p.depthExceeded(stops)
	return false
}

// depthExceeded replaces the input the caller would have parsed with a single
// error node. It consumes up to the next token in stops that is not inside a
// delimited group. When stops is empty it consumes one token, plus the group
// that token opens if it opens one. Unless the input is exhausted it always
// consumes at least one token, so the caller's loop makes progress.
func (p *parser) depthExceeded(stops syntax.Set) {
	from := len(p.nodes) - p.cur.trivia
	balance := 0
	p.withNewlineMode(nlContinue, func() {
		for !p.atEnd() {
			switch {
			case openDelims.Contains(p.cur.kind):
				balance++
			case closeDelims.Contains(p.cur.kind):
				balance = max(balance-1, 0)
			}
			p.consume()
			if balance == 0 && (stops == noStops || p.atSet(stops)) {
				break
			}
		}
	})

	to := len(p.nodes) - p.cur.trivia
	if from == to {
		// The input ran out before anything could be consumed, so there is
		// nothing to fold into the error. Report it where the construct would
		// have continued, unless an error is already there.
		var prev syntax.Span
		if from > 0 {
			if p.nodes[from-1].Kind() == syntax.KindError {
				p.errAnchor = noAnchor
				return
			}
			prev = p.nodes[from-1].Span()
		}
		n := p.a.Error(syntax.Span{Start: prev.End, End: prev.End}, depthExceededMsg)
		p.nodes = slices.Insert(p.nodes, from, syntax.Node(n))
		p.errAnchor = from
		return
	}

	span := syntax.Span{Start: p.nodes[from].Span().Start, End: p.nodes[to-1].Span().End}
	p.nodes[from] = p.a.Error(span, depthExceededMsg)
	p.nodes = slices.Delete(p.nodes, from+1, to)
	p.errAnchor = from
}
