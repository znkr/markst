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

package html_test

import (
	"reflect"
	"strings"
	"testing"

	"znkr.io/markst/html"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// TestEveryElementRenders renders one of every value.Content implementation,
// built by hand rather than compiled, and checks what each comes out as.
//
// It exists for completeness rather than for the expectations: value.Content is
// a closed interface, the renderer's type switch covers all of it, and an
// element added to the model without a case would land in the switch's default
// — which routes to the math renderer, which would route it back. This test is
// what turns that into a failure here instead of a stack overflow in a
// document. Keep it exhaustive.
func TestEveryElementRenders(t *testing.T) {
	body := func() value.Content { return &value.Text{Text: "x"} }
	label := &value.Label{Name: name.Make("l")}

	tests := []struct {
		name string
		in   value.Content
		want string
	}{
		// Structure and text.
		{"sequence", &value.Sequence{Children: []value.Content{body(), body()}}, "xx"},
		{"par", &value.Par{Body: body()}, "<p>x</p>\n"},
		{"text", &value.Text{Text: "a < b"}, "a &lt; b"},
		{"linebreak", &value.Linebreak{}, "<br>"},
		{"parbreak", &value.Parbreak{}, ""},
		{"hspace", &value.HSpace{Amount: value.Length{Em: 1}}, `<span style="display:inline-block;width:1em"></span>`},
		{"smartquote", &value.SmartQuote{Double: true}, "“"},
		{"raw-inline", &value.Raw{Text: "a"}, "<code>a</code>"},
		{"raw-block", &value.Raw{Block: true, Lang: "go", Text: "a"}, "<pre><code class=\"language-go\">a</code></pre>\n"},

		// Markup wrappers.
		{"heading", &value.Heading{Depth: 2, Body: body(), Label: label}, "<h2 id=\"l\">x</h2>\n"},
		{"strong", &value.Strong{Body: body()}, "<strong>x</strong>"},
		{"emph", &value.Emph{Body: body()}, "<em>x</em>"},
		{"underline", &value.Underline{Body: body()}, "<u>x</u>"},
		{"link", &value.Link{Dest: "u", Body: body()}, `<a href="u">x</a>`},
		{"ref", &value.Ref{Target: name.Make("t")}, `<a href="#t">t</a>`},
		{"ref-with-supplement", &value.Ref{Target: name.Make("t"), Supplement: body()}, `<a href="#t">x</a>`},
		{"image", &value.Image{Path: "p.png", Alt: "a"}, `<img src="p.png" alt="a">`},

		// Lists and tables.
		{"list", &value.List{Children: []*value.ListItem{{Body: body()}}}, "<ul>\n<li>x</li>\n</ul>\n"},
		{"list-item", &value.ListItem{Body: body()}, "<li>x</li>\n"},
		{"enum", &value.Enum{Children: []*value.EnumItem{{Number: -1, Body: body()}}}, "<ol>\n<li>x</li>\n</ol>\n"},
		{"enum-item", &value.EnumItem{Number: 3, Body: body()}, "<li value=\"3\">x</li>\n"},
		{"terms", &value.Terms{Children: []*value.TermItem{{Term: body(), Description: body()}}}, "<dl>\n<dt>x</dt>\n<dd>x</dd>\n</dl>\n"},
		{"term-item", &value.TermItem{Term: body(), Description: body()}, "<dt>x</dt>\n<dd>x</dd>\n"},
		{"table", &value.Table{Columns: 1, Children: []value.Content{body()}}, "<table>\n<tbody>\n<tr><td>x</td></tr>\n</tbody>\n</table>\n"},
		// A header outside a table means nothing there; its cells are all that
		// is left of it.
		{"table-header", &value.TableHeader{Children: []value.Content{body()}}, "x"},

		// Escape hatches, invisible elements, and the root.
		{"html-elem", &value.HTMLElem{Tag: "span", Body: body()}, "<span>x</span>"},
		{"html-elem-void", &value.HTMLElem{Tag: "br", Block: false}, "<br>"},
		{"metadata", &value.Metadata{Value: value.Int(1)}, ""},
		{"state-update", &value.StateUpdate{Key: "k", Update: value.Int(1)}, ""},
		{"style-update", &value.StyleUpdate{}, ""},
		{"styled", &value.Styled{Body: body()}, "x"},
		{"document", &value.Document{Body: &value.Par{Body: body()}}, "<p>x</p>\n"},

		// Math. Each is also rendered on its own, outside any equation, which
		// is content markst allows to be built even though a document cannot
		// write it.
		{"equation-inline", &value.Equation{Body: &value.MathText{Text: "x"}}, "<math><mi>x</mi></math>"},
		{"equation-block", &value.Equation{Block: true, Body: &value.MathText{Text: "x"}}, "<math display=\"block\"><mi>x</mi></math>\n"},
		{"math-text", &value.MathText{Text: "x"}, "<mi>x</mi>"},
		{"math-op", &value.MathOp{Text: &value.MathText{Text: "sin"}}, `<mo lspace="0.1667em" rspace="0.1667em">sin</mo>`},
		{"math-attach", &value.MathAttach{Base: &value.MathText{Text: "a"}, Top: &value.MathText{Text: "2"}}, "<msup><mi>a</mi><mn>2</mn></msup>"},
		{"math-frac", &value.MathFrac{Num: &value.MathText{Text: "a"}, Denom: &value.MathText{Text: "b"}}, "<mfrac><mi>a</mi><mi>b</mi></mfrac>"},
		{"math-root", &value.MathRoot{Radicand: &value.MathText{Text: "a"}}, "<msqrt><mi>a</mi></msqrt>"},
		{"math-primes", &value.MathPrimes{Base: &value.MathText{Text: "f"}, Count: 1}, "<msup><mi>f</mi><mo>′</mo></msup>"},
		{"math-align-point", &value.MathAlignPoint{}, "<mspace></mspace>"},
		{"math-lr", &value.MathLr{Body: &value.MathText{Text: "a"}}, "<mrow><mi>a</mi></mrow>"},
		// A mid follows its enclosing group; standing alone there is none, so it
		// does not stretch. Inside one, see delimited-with-a-mid in math.test.
		{"math-mid", &value.MathMid{Body: &value.MathText{Text: "|"}}, `<mo stretchy="false">|</mo>`},
		{"math-underline", &value.MathUnderline{Body: &value.MathText{Text: "a"}}, "<munder accentunder=\"true\"><mi>a</mi><mo stretchy=\"true\">̲</mo></munder>"},
		{"math-accent", &value.MathAccent{Base: &value.MathText{Text: "a"}, Accent: "̂"}, "<mover accent=\"true\"><mi>a</mi><mo>̂</mo></mover>"},
		{"math-cancel", &value.MathCancel{Body: &value.MathText{Text: "a"}}, `<mrow style="text-decoration:line-through"><mi>a</mi></mrow>`},
		{"math-vec", &value.MathVec{Children: []value.Content{&value.MathText{Text: "a"}}}, `<mrow><mo stretchy="true">(</mo><mtable columnalign="center"><mtr><mtd><mi>a</mi></mtd></mtr></mtable><mo stretchy="true">)</mo></mrow>`},
		{"math-cases", &value.MathCases{Children: []value.Content{&value.MathText{Text: "a"}}}, `<mrow><mo stretchy="true">{</mo><mtable columnalign="left"><mtr><mtd><mi>a</mi></mtd></mtr></mtable></mrow>`},
		{"math-mat", &value.MathMat{Rows: [][]value.Content{{&value.MathText{Text: "a"}}}}, `<mrow><mo stretchy="true">(</mo><mtable columnalign="center"><mtr><mtd><mi>a</mi></mtd></mtr></mtable><mo stretchy="true">)</mo></mrow>`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			if err := html.Render(&b, tc.in); err != nil {
				t.Fatalf("Render(%s) error = %v", tc.in.Name(), err)
			}
			if got := b.String(); got != tc.want {
				t.Errorf("Render(%s) = %q, want %q", tc.in.Name(), got, tc.want)
			}
		})
	}

	// Footnote and Custom are left out of the table because neither renders on
	// its own: a footnote is a citation of an endnote, and a custom element
	// needs a hook. They are covered by footnotes.test and TestCustomWithHook.
	kinds := map[reflect.Type]bool{
		reflect.TypeFor[*value.Footnote](): true,
		reflect.TypeFor[*value.Custom]():   true,
	}
	for _, tc := range tests {
		kinds[reflect.TypeOf(tc.in)] = true
	}
	const contentKinds = 47
	if len(kinds) != contentKinds {
		t.Errorf("covered %d of value.Content's %d implementations: the model gained or lost one, "+
			"and the renderer's type switch has to gain or lose a case with it", len(kinds), contentKinds)
	}
}
