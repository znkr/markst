// Command symbolsgen turns a symbol data file (copied from
// github.com/typst/codex) into a Go source file declaring a symbols.Module.
//
// Usage:
//
//	symbolsgen <Name> <input>
//
// It reads <input> and writes <lowername>_gen.go declaring a top-level
// var <Name> = Module{...} in package symbols.
//
// The data format is line-oriented. After stripping "//" comments and
// surrounding whitespace, each non-blank line is one of:
//
//	<name> {              module open
//	}                     module close
//	.<mod>[.<mod>…] <val>  variant of the preceding entry
//	<name> [<val>]        entry (value optional)
//
// Braces are reserved; literal braces in values are always escaped (\u{7B},
// \u{7D}), so the open/close checks are unambiguous.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: symbolsgen <Name> <input>")
		os.Exit(2)
	}
	name, input := os.Args[1], os.Args[2]

	if err := run(name, input); err != nil {
		fmt.Fprintf(os.Stderr, "symbolsgen: %s\n", err)
		os.Exit(1)
	}
}

func run(name, input string) error {
	in, err := os.Open(input)
	if err != nil {
		return err
	}
	defer in.Close()

	root, err := parse(in)
	if err != nil {
		return err
	}

	src, err := generate(name, root)
	if err != nil {
		return err
	}

	filename := strings.ToLower(name) + "_gen.go"
	return os.WriteFile(filename, src, 0o644)
}

type symbol struct {
	mods  []string
	value string // decoded Unicode
}

type binding struct {
	name     string
	variants []symbol // set => Variants binding
	sub      *module  // set => submodule binding
}

type module struct {
	bindings []*binding // source order; sorted by name at generation time
}

// lineScanner yields the significant lines of the input, dropping comments and
// blank lines while tracking the source line number for error messages.
type lineScanner struct {
	sc  *bufio.Scanner
	num int
}

// next advances to the next significant line, returning it and whether one was
// found.
func (s *lineScanner) next() (string, bool) {
	for s.sc.Scan() {
		s.num++
		line := s.sc.Text()
		line, _, _ = strings.Cut(line, "//")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line, true
	}
	return "", false
}

func parse(r io.Reader) (*module, error) {
	s := &lineScanner{sc: bufio.NewScanner(r)}
	root, err := parseModule(s, true)
	if err != nil {
		return nil, err
	}
	if err := s.sc.Err(); err != nil {
		return nil, err
	}
	return root, nil
}

// parseModule reads bindings until end of input (top) or a closing brace.
func parseModule(s *lineScanner, top bool) (*module, error) {
	m := &module{}
	for {
		line, ok := s.next()
		if !ok {
			if !top {
				return nil, fmt.Errorf("line %d: unclosed module", s.num)
			}
			return m, nil
		}

		if line == "}" {
			if top {
				return nil, fmt.Errorf("line %d: unexpected '}'", s.num)
			}
			return m, nil
		}

		key, value, _ := strings.Cut(line, " ")
		value = strings.TrimSpace(value)

		switch {
		case value == "{":
			sub, err := parseModule(s, false)
			if err != nil {
				return nil, err
			}
			m.bindings = append(m.bindings, &binding{name: key, sub: sub})

		case strings.HasPrefix(key, "."):
			if len(m.bindings) == 0 {
				return nil, fmt.Errorf("line %d: variant %q without preceding entry", s.num, key)
			}
			last := m.bindings[len(m.bindings)-1]
			if last.sub != nil {
				return nil, fmt.Errorf("line %d: variant %q cannot follow a module", s.num, key)
			}
			v, err := decode(value)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", s.num, err)
			}
			last.variants = append(last.variants, symbol{
				mods:  strings.Split(key[1:], "."),
				value: v,
			})

		default:
			b := &binding{name: key}
			if value != "" {
				v, err := decode(value)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", s.num, err)
				}
				b.variants = append(b.variants, symbol{value: v})
			}
			m.bindings = append(m.bindings, b)
		}
	}
}

// decode expands the escape sequences in a value, leaving all other text (which
// is already real Unicode) untouched:
//
//	\u{hex}       the code point hex
//	\vs{N}        variation selector N (1..16)
//	\vs{text}     text-presentation selector (VS15)
//	\vs{emoji}    emoji-presentation selector (VS16)
func decode(text string) (string, error) {
	var b strings.Builder
	for {
		switch {
		case strings.HasPrefix(text, `\u{`):
			rest := text[len(`\u{`):]
			code, tail, ok := strings.Cut(rest, "}")
			if !ok {
				return "", fmt.Errorf(`unclosed Unicode escape: \u{%s`, rest)
			}
			n, err := strconv.ParseUint(code, 16, 32)
			if err != nil || !utf8.ValidRune(rune(n)) {
				return "", fmt.Errorf(`invalid Unicode escape: \u{%s}`, code)
			}
			b.WriteRune(rune(n))
			text = tail

		case strings.HasPrefix(text, `\vs{`):
			rest := text[len(`\vs{`):]
			value, tail, ok := strings.Cut(rest, "}")
			if !ok {
				return "", fmt.Errorf(`unclosed VS escape: \vs{%s`, rest)
			}
			vs, ok := variationSelector(value)
			if !ok {
				return "", fmt.Errorf(`invalid VS escape: \vs{%s}`, value)
			}
			b.WriteRune(vs)
			text = tail

		default:
			i := strings.IndexByte(text, '\\')
			if i < 0 {
				b.WriteString(text)
				return b.String(), nil
			}
			if i == 0 {
				return "", fmt.Errorf("invalid escape sequence: %s", text)
			}
			b.WriteString(text[:i])
			text = text[i:]
		}
	}
}

// variationSelector maps a \vs{...} argument to its variation selector rune.
// "1".."16" select VS1..VS16 (U+FE00..U+FE0F); "text" and "emoji" alias VS15
// and VS16.
func variationSelector(s string) (rune, bool) {
	switch s {
	case "text":
		return 0xFE0E, true
	case "emoji":
		return 0xFE0F, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 16 {
		return 0, false
	}
	return rune(0xFE00 + n - 1), true
}

func generate(name string, root *module) ([]byte, error) {
	g := generator{prefix: "n_" + name + "_", ids: map[string]string{}}
	g.module(root)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by symbolsgen; DO NOT EDIT.\n\n")
	fmt.Fprintf(&buf, "package symbols\n\n")
	fmt.Fprintf(&buf, "import \"znkr.io/writst/name\"\n\n")

	// Intern every distinct name exactly once; the body references these.
	slices.Sort(g.names)
	fmt.Fprintf(&buf, "var (\n")
	for _, s := range g.names {
		fmt.Fprintf(&buf, "%s%s = name.Make(%q)\n", g.prefix, s, s)
	}
	fmt.Fprintf(&buf, ")\n\n")

	fmt.Fprintf(&buf, "var %s = ", name)
	buf.Write(g.body.Bytes())
	fmt.Fprintf(&buf, "\n")

	return format.Source(buf.Bytes())
}

// generator emits a Module literal into body while collecting the distinct
// names it references, so each is interned once in a shared var block.
type generator struct {
	body   bytes.Buffer
	prefix string
	ids    map[string]string // name -> Go identifier
	names  []string          // distinct names, first-seen order
}

// id returns the identifier of the var interning s, registering s on first use.
func (g *generator) id(s string) string {
	id, ok := g.ids[s]
	if !ok {
		id = g.prefix + s // s is ASCII letters; the prefix avoids keywords
		g.ids[s] = id
		g.names = append(g.names, s)
	}
	return id
}

func (g *generator) module(m *module) {
	fmt.Fprintf(&g.body, "Module{\nbindings: map[name.Name]Binding{\n")
	slices.SortFunc(m.bindings, func(a, b *binding) int {
		return strings.Compare(a.name, b.name)
	})
	for _, b := range m.bindings {
		fmt.Fprintf(&g.body, "%s: ", g.id(b.name))
		if b.sub != nil {
			g.module(b.sub)
			fmt.Fprintf(&g.body, ",\n")
			continue
		}
		fmt.Fprintf(&g.body, "Variants{\n")
		for _, s := range b.variants {
			mods := make([]string, len(s.mods))
			for i, mod := range s.mods {
				mods[i] = g.id(mod)
			}
			fmt.Fprintf(&g.body, "Symbol{Mods: []name.Name{%s}, Value: %q},\n", strings.Join(mods, ", "), s.value)
		}
		fmt.Fprintf(&g.body, "},\n")
	}
	fmt.Fprintf(&g.body, "},\n}")
}
