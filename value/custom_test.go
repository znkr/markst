package value

import (
	"testing"

	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

func TestCustomFormat(t *testing.T) {
	tests := []struct {
		name string
		in   *Custom
		want string
	}{
		{
			name: "plain",
			in:   &Custom{Elem: "include-diff", Block: true, Value: 42},
			want: "#custom(\"include-diff\")\n",
		},
		{
			name: "labelled",
			in:   &Custom{Elem: "include-snippet", Label: &Label{Name: name.Make("snip")}},
			want: "#custom(\"include-snippet\") <snip>\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatContent(tt.in); got != tt.want {
				t.Errorf("FormatContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCustomEqual(t *testing.T) {
	// The payload is opaque, so equality is identity: two elements built from
	// the same payload are still two elements.
	payload := "shared"
	a := &Custom{Elem: "x", Value: payload}
	b := &Custom{Elem: "x", Value: payload}
	if !a.Equal(a) {
		t.Errorf("a.Equal(a) = false, want true")
	}
	if a.Equal(b) {
		t.Errorf("a.Equal(b) = true, want false")
	}
	if a.Equal(&Text{Text: "x"}) {
		t.Errorf("a.Equal(text) = true, want false")
	}
}

func TestCustomContent(t *testing.T) {
	c := &Custom{Elem: "include-diff", Block: true, Value: 42}
	if got := c.Type(); got != types.Content {
		t.Errorf("Type() = %v, want %v", got, types.Content)
	}
	if got := c.Name(); got != "include-diff" {
		t.Errorf("Name() = %q, want %q", got, "include-diff")
	}
	if !c.IsBlock() {
		t.Errorf("IsBlock() = false, want true")
	}
	if got := c.HasField(name.Make("value")); got {
		t.Errorf("HasField(value) = true, want false")
	}
	if got := c.Fields(); got.Elems.Len() != 0 {
		t.Errorf("Fields() = %v, want empty", got)
	}

	old := c.SetLabel(&Label{Name: name.Make("l")})
	if old != nil {
		t.Errorf("SetLabel() = %v, want nil", old)
	}
	if got := c.GetLabel(); got == nil || got.Name != name.Make("l") {
		t.Errorf("GetLabel() = %v, want <l>", got)
	}
}

func TestCustomWalk(t *testing.T) {
	// A Custom is a leaf: the walk reaches it, and does not descend into the
	// payload even when the payload is itself content.
	c := &Custom{Elem: "x", Value: &Text{Text: "hidden"}}
	body := &Sequence{Children: []Content{&Text{Text: "a"}, c}}
	var got []string
	for e := range All(body) {
		got = append(got, e.Name())
	}
	want := []string{"sequence", "text", "x"}
	if len(got) != len(want) {
		t.Fatalf("All() visited %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("All() visited %v, want %v", got, want)
		}
	}
}
