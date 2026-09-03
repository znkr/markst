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

// htmlTag records what an HTML tag's name says about the element written with
// it. Two questions are asked of every tag, and they are not the same question:
//
//   - block is about the element's own place in the flow — whether it occupies
//     a line of its own, which is what [Content.IsBlock] reports.
//   - flow is about what goes inside it. An element whose content model is
//     flow content holds paragraphs; one whose content model is phrasing
//     content holds a single line's worth of text. `p` is block and phrasing
//     at once: it starts a line, and realizing its body as flow content would
//     put a paragraph inside a paragraph.
//
// void marks the elements that have no body at all and no closing tag.
type htmlTag struct {
	block bool
	flow  bool
	void  bool
}

// htmlTags is what markst knows about HTML: the elements whose defaults differ
// from an unknown element's. Everything absent from it — a custom element such
// as `my-callout`, or a tag added to HTML after this table was written — is
// treated the way a browser treats an element it does not recognize: inline,
// with a phrasing body.
var htmlTags = map[string]htmlTag{
	// Flow containers: a body of paragraphs, on a line of their own.
	"address":    {block: true, flow: true},
	"article":    {block: true, flow: true},
	"aside":      {block: true, flow: true},
	"blockquote": {block: true, flow: true},
	"dd":         {block: true, flow: true},
	"details":    {block: true, flow: true},
	"div":        {block: true, flow: true},
	"fieldset":   {block: true, flow: true},
	"figure":     {block: true, flow: true},
	"footer":     {block: true, flow: true},
	"form":       {block: true, flow: true},
	"header":     {block: true, flow: true},
	"li":         {block: true, flow: true},
	"main":       {block: true, flow: true},
	"nav":        {block: true, flow: true},
	"section":    {block: true, flow: true},
	"td":         {block: true, flow: true},
	"th":         {block: true, flow: true},

	// Block elements whose body is a single line of phrasing content.
	"caption":    {block: true},
	"dt":         {block: true},
	"figcaption": {block: true},
	"h1":         {block: true},
	"h2":         {block: true},
	"h3":         {block: true},
	"h4":         {block: true},
	"h5":         {block: true},
	"h6":         {block: true},
	"legend":     {block: true},
	"p":          {block: true},
	"pre":        {block: true},
	"summary":    {block: true},

	// Containers that hold only their own item elements. The body is written
	// as those items, so it is phrasing: nothing here takes a paragraph.
	"dl":    {block: true},
	"ol":    {block: true},
	"table": {block: true},
	"tbody": {block: true},
	"tfoot": {block: true},
	"thead": {block: true},
	"tr":    {block: true},
	"ul":    {block: true},

	// Void elements: no body, no closing tag.
	"area":   {void: true},
	"base":   {void: true},
	"br":     {void: true},
	"col":    {void: true},
	"embed":  {void: true},
	"hr":     {block: true, void: true},
	"img":    {void: true},
	"input":  {void: true},
	"link":   {void: true},
	"meta":   {void: true},
	"source": {void: true},
	"track":  {void: true},
	"wbr":    {void: true},
}

// HtmlTagBlock reports whether an element written with this tag occupies a line
// of its own. It is what `block: auto` resolves to; a document that disagrees
// passes `block:` explicitly.
func HtmlTagBlock(tag string) bool { return htmlTags[tag].block }

// HtmlTagFlow reports whether the tag's content model is flow content, so that
// paragraphs form inside its body. Unlike block-ness this is not overridable:
// it follows from the tag, because it is the tag that decides what the markup
// around the body is allowed to be.
func HtmlTagFlow(tag string) bool { return htmlTags[tag].flow }

// HtmlTagVoid reports whether the tag is one of HTML's void elements, which
// have no body and no closing tag. A presenter writes `<br>` and stops.
func HtmlTagVoid(tag string) bool { return htmlTags[tag].void }
