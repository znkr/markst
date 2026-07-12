package value

import (
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

// Content is the interface for values that represent document content. It
// extends [Value] with label support and field access (for show/set rules).
// All Content values have [types.Content] as their type.
//
//go:generate go tool znkr.io/writst/internal/fieldaccessgen
type Content interface {
	Value

	SetLabel(*Label) *Label

	Field(name.Name) Value
	HasField(name.Name) bool
	Fields() *Dict

	// Name returns the element name of the content value (e.g. "text",
	// "heading", "sequence"), matching the function name used to construct it.
	// Used in diagnostics such as "element <name> has no method `x`".
	Name() string

	aContent()
}

// Sequence is a flat list of content elements, produced by concatenating
// content with the + operator or from markup blocks.
type Sequence struct {
	Children []Content `writst:"required"`
	Label    *Label
}

type Heading struct {
	Depth int     `writst:"required"`
	Body  Content `writst:"required"`
	Label *Label
}

type Strong struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Emph struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Text struct {
	Text  string `writst:"required"`
	Label *Label
}

type Raw struct {
	Block bool `writst:"required"`
	Lang  string
	Text  string `writst:"required"`
}

type Linebreak struct{}

type Parbreak struct{}

type Link struct {
	Dest  string  `writst:"required"`
	Body  Content `writst:"required"`
	Label *Label
}

type Ref struct {
	Target     name.Name `writst:"required"`
	Supplement Content
	Label      *Label
}

type List struct {
	Children []*ListItem `writst:"required"`
	Label    *Label
}

type ListItem struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Enum struct {
	Children []*EnumItem `writst:"required"`
	Label    *Label
}

type EnumItem struct {
	Number int
	Body   Content `writst:"required"`
	Label  *Label
}

type Terms struct {
	Children []*TermItem `writst:"required"`
	Label    *Label
}

type TermItem struct {
	Term        Content `writst:"required"`
	Description Content `writst:"required"`
	Label       *Label
}

type Table struct {
	Children []Content `writst:"required"`
	Label    *Label
}

func (*Sequence) aValue()  {}
func (*Heading) aValue()   {}
func (*Text) aValue()      {}
func (*Raw) aValue()       {}
func (*Strong) aValue()    {}
func (*Emph) aValue()      {}
func (*Linebreak) aValue() {}
func (*Parbreak) aValue()  {}
func (*Link) aValue()      {}
func (*Ref) aValue()       {}
func (*List) aValue()      {}
func (*ListItem) aValue()  {}
func (*Enum) aValue()      {}
func (*EnumItem) aValue()  {}
func (*Terms) aValue()     {}
func (*TermItem) aValue()  {}
func (*Table) aValue()     {}

func (Sequence) aContent()   {}
func (*Heading) aContent()   {}
func (*Text) aContent()      {}
func (*Raw) aContent()       {}
func (*Strong) aContent()    {}
func (*Emph) aContent()      {}
func (*Linebreak) aContent() {}
func (*Parbreak) aContent()  {}
func (*Link) aContent()      {}
func (*Ref) aContent()       {}
func (*List) aContent()      {}
func (*ListItem) aContent()  {}
func (*Enum) aContent()      {}
func (*EnumItem) aContent()  {}
func (*Terms) aContent()     {}
func (*TermItem) aContent()  {}
func (*Table) aContent()     {}

// Name returns the element name, matching the constructor function used to
// build it. Table has no dedicated diagnostic name and reports the generic
// "content".
func (Sequence) Name() string   { return "sequence" }
func (*Heading) Name() string   { return "heading" }
func (*Strong) Name() string    { return "strong" }
func (*Emph) Name() string      { return "emph" }
func (*Text) Name() string      { return "text" }
func (*Raw) Name() string       { return "raw" }
func (*Linebreak) Name() string { return "linebreak" }
func (*Parbreak) Name() string  { return "parbreak" }
func (*Link) Name() string      { return "link" }
func (*Ref) Name() string       { return "ref" }
func (*List) Name() string      { return "list" }
func (*ListItem) Name() string  { return "list.item" }
func (*Enum) Name() string      { return "enum" }
func (*EnumItem) Name() string  { return "enum.item" }
func (*Terms) Name() string     { return "terms" }
func (*TermItem) Name() string  { return "terms.item" }
func (*Table) Name() string     { return "table" }

func (Sequence) Type() types.Type   { return types.Content }
func (*Heading) Type() types.Type   { return types.Content }
func (*Text) Type() types.Type      { return types.Content }
func (*Raw) Type() types.Type       { return types.Content }
func (*Strong) Type() types.Type    { return types.Content }
func (*Emph) Type() types.Type      { return types.Content }
func (*Linebreak) Type() types.Type { return types.Content }
func (*Parbreak) Type() types.Type  { return types.Content }
func (*Link) Type() types.Type      { return types.Content }
func (*Ref) Type() types.Type       { return types.Content }
func (*List) Type() types.Type      { return types.Content }
func (*ListItem) Type() types.Type  { return types.Content }
func (*Enum) Type() types.Type      { return types.Content }
func (*EnumItem) Type() types.Type  { return types.Content }
func (*Terms) Type() types.Type     { return types.Content }
func (*TermItem) Type() types.Type  { return types.Content }
func (*Table) Type() types.Type     { return types.Content }
