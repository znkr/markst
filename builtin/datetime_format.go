package builtin

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"znkr.io/markst/value"
)

// This file implements the format descriptions `datetime.display` takes, the
// same ones Typst inherits from the Rust `time` crate: literal text with
// components in square brackets, each optionally carrying `key:value`
// modifiers — `"[year]-[month repr:long]"`. A literal `[` is written `[[`.
//
// Two kinds of failure come out of here, and they are reported at different
// places. A malformed description is the caller's argument being wrong, so it
// is an [value.ArgErrorPosf] against the pattern (positional index 1, since
// `self` is pre-bound on a method call). Asking for a component the value
// doesn't have — the hour of a date — is a property of the call as a whole and
// reported as a plain error against the whole expression.

// errInsufficient is returned when the format description asks for a component
// the datetime does not carry.
var errInsufficient = fmt.Errorf("failed to format datetime (insufficient information)")

// patternErrorf builds a diagnostic pointing at the format description itself.
func patternErrorf(format string, args ...any) error {
	return value.ArgErrorPosf(1, format, args...)
}

// formatDatetime renders d according to the given format description.
func formatDatetime(d value.Datetime, pattern string) (string, error) {
	var sb strings.Builder
	for i := 0; i < len(pattern); {
		if pattern[i] != '[' {
			sb.WriteByte(pattern[i])
			i++
			continue
		}
		if i+1 < len(pattern) && pattern[i+1] == '[' {
			sb.WriteByte('[')
			i += 2
			continue
		}
		end := strings.IndexByte(pattern[i:], ']')
		if end < 0 {
			return "", patternErrorf("missing closing bracket for bracket at index %d", i)
		}
		end += i
		s, err := formatComponent(d, pattern[i+1:end], i)
		if err != nil {
			return "", err
		}
		sb.WriteString(s)
		i = end + 1
	}
	return sb.String(), nil
}

// modifier is one `key:value` pair inside a component, with the source indices
// of both halves so a complaint about either can point at the right character.
type modifier struct {
	key, val       string
	keyIdx, valIdx int
	used           bool
}

// formatComponent renders the component described by body, which is the text
// between the brackets that start at open in the enclosing format description.
func formatComponent(d value.Datetime, body string, open int) (string, error) {
	name, mods, err := splitComponent(body, open)
	if err != nil {
		return "", err
	}

	var out string
	switch name {
	case "year":
		out, err = formatYear(d, mods)
	case "month":
		out, err = formatMonth(d, mods)
	case "day":
		out, err = formatDatePart(d, mods, 2, func(t time.Time) int { return t.Day() })
	case "ordinal":
		out, err = formatDatePart(d, mods, 3, func(t time.Time) int { return t.YearDay() })
	case "weekday":
		out, err = formatWeekday(d, mods)
	case "week_number":
		out, err = formatWeekNumber(d, mods)
	case "hour":
		out, err = formatHour(d, mods)
	case "minute":
		out, err = formatTimePart(d, mods, func(t time.Time) int { return t.Minute() })
	case "second":
		out, err = formatTimePart(d, mods, func(t time.Time) int { return t.Second() })
	case "period":
		out, err = formatPeriod(d, mods)
	default:
		nameIdx := open + 1 + strings.Index(body, name)
		return "", patternErrorf("invalid component name '%s' at index %d", name, nameIdx)
	}
	if err != nil {
		return "", err
	}

	// Every modifier the component didn't ask about is one it doesn't know.
	for _, m := range mods {
		if !m.used {
			return "", patternErrorf("invalid modifier '%s' at index %d", m.key, m.keyIdx)
		}
	}
	return out, nil
}

// splitComponent breaks a component body into its name and its modifiers. base
// is the index of the opening bracket, used to report positions in terms of the
// whole format description.
func splitComponent(body string, base int) (string, []*modifier, error) {
	fields, offsets := splitFields(body)
	if len(fields) == 0 {
		return "", nil, patternErrorf("expected component name at index %d", base)
	}
	mods := make([]*modifier, 0, len(fields)-1)
	for i, f := range fields[1:] {
		idx := base + 1 + offsets[i+1]
		key, val, ok := strings.Cut(f, ":")
		if !ok || key == "" {
			return "", nil, patternErrorf("invalid modifier '%s' at index %d", f, idx)
		}
		mods = append(mods, &modifier{key: key, val: val, keyIdx: idx, valIdx: idx + len(key) + 1})
	}
	return fields[0], mods, nil
}

// splitFields splits s on whitespace, returning each field along with its byte
// offset in s.
func splitFields(s string) (fields []string, offsets []int) {
	for i := 0; i < len(s); {
		if s[i] == ' ' || s[i] == '\t' {
			i++
			continue
		}
		start := i
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
		fields = append(fields, s[start:i])
		offsets = append(offsets, start)
	}
	return fields, offsets
}

// lookup returns the value of the named modifier, marking it as understood by
// the component. def is returned when the modifier isn't present; an unknown
// value is an error pointing at the value itself.
func lookup(mods []*modifier, key, def string, allowed ...string) (string, error) {
	for _, m := range mods {
		if m.key != key {
			continue
		}
		m.used = true
		for _, a := range allowed {
			if m.val == a {
				return m.val, nil
			}
		}
		return "", patternErrorf("invalid modifier '%s' at index %d", m.val, m.valIdx)
	}
	return def, nil
}

// padding reads the `padding` modifier, which all numeric components share.
func padding(mods []*modifier) (string, error) {
	return lookup(mods, "padding", "zero", "zero", "space", "none")
}

// pad renders n to at least width digits in the requested padding style,
// keeping a minus sign in front of the padding.
func pad(n, width int, style string) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	digits := strconv.Itoa(n)
	if style != "none" {
		fill := byte('0')
		if style == "space" {
			fill = ' '
		}
		if d := width - len(digits); d > 0 {
			digits = strings.Repeat(string(fill), d) + digits
		}
	}
	return sign + digits
}

func formatYear(d value.Datetime, mods []*modifier) (string, error) {
	repr, err := lookup(mods, "repr", "full", "full", "last_two")
	if err != nil {
		return "", err
	}
	base, err := lookup(mods, "base", "calendar", "calendar", "iso_week")
	if err != nil {
		return "", err
	}
	sign, err := lookup(mods, "sign", "automatic", "automatic", "mandatory")
	if err != nil {
		return "", err
	}
	style, err := padding(mods)
	if err != nil {
		return "", err
	}
	if !d.HasDate() {
		return "", errInsufficient
	}

	year := d.T.Year()
	if base == "iso_week" {
		year, _ = d.T.ISOWeek()
	}
	if repr == "last_two" {
		// The last two digits stand on their own: the century they came from
		// is gone, so a sign in front of them would name nothing. The `time`
		// crate this format language follows suppresses it here too — printing
		// one would turn 44 BC into "+44".
		return pad(abs(year)%100, 2, style), nil
	}
	s := pad(year, 4, style)
	if sign == "mandatory" && year >= 0 {
		s = "+" + s
	}
	return s, nil
}

func formatMonth(d value.Datetime, mods []*modifier) (string, error) {
	repr, err := lookup(mods, "repr", "numerical", "numerical", "long", "short")
	if err != nil {
		return "", err
	}
	style, err := padding(mods)
	if err != nil {
		return "", err
	}
	if !d.HasDate() {
		return "", errInsufficient
	}
	switch repr {
	case "long":
		return d.T.Month().String(), nil
	case "short":
		return d.T.Month().String()[:3], nil
	default:
		return pad(int(d.T.Month()), 2, style), nil
	}
}

// formatDatePart renders a plain zero-padded number read off the date.
func formatDatePart(d value.Datetime, mods []*modifier, width int, get func(time.Time) int) (string, error) {
	style, err := padding(mods)
	if err != nil {
		return "", err
	}
	if !d.HasDate() {
		return "", errInsufficient
	}
	return pad(get(d.T), width, style), nil
}

func formatWeekday(d value.Datetime, mods []*modifier) (string, error) {
	repr, err := lookup(mods, "repr", "long", "long", "short", "sunday", "monday")
	if err != nil {
		return "", err
	}
	oneIndexed, err := lookup(mods, "one_indexed", "true", "true", "false")
	if err != nil {
		return "", err
	}
	if !d.HasDate() {
		return "", errInsufficient
	}
	switch repr {
	case "long":
		return d.T.Weekday().String(), nil
	case "short":
		return d.T.Weekday().String()[:3], nil
	}
	// The numeric representations name the day the week starts on, so the
	// count runs from there.
	n := int(d.T.Weekday())
	if repr == "monday" {
		n = (n + 6) % 7
	}
	if oneIndexed == "true" {
		n++
	}
	return strconv.Itoa(n), nil
}

func formatWeekNumber(d value.Datetime, mods []*modifier) (string, error) {
	repr, err := lookup(mods, "repr", "iso", "iso", "sunday", "monday")
	if err != nil {
		return "", err
	}
	style, err := padding(mods)
	if err != nil {
		return "", err
	}
	if !d.HasDate() {
		return "", errInsufficient
	}
	var week int
	switch repr {
	case "iso":
		_, week = d.T.ISOWeek()
	case "sunday":
		week = (d.T.YearDay() + 6 - int(d.T.Weekday())) / 7
	case "monday":
		week = (d.T.YearDay() + 6 - (int(d.T.Weekday())+6)%7) / 7
	}
	return pad(week, 2, style), nil
}

func formatHour(d value.Datetime, mods []*modifier) (string, error) {
	repr, err := lookup(mods, "repr", "24", "24", "12")
	if err != nil {
		return "", err
	}
	style, err := padding(mods)
	if err != nil {
		return "", err
	}
	if !d.HasTime() {
		return "", errInsufficient
	}
	hour := d.T.Hour()
	if repr == "12" {
		if hour %= 12; hour == 0 {
			hour = 12
		}
	}
	return pad(hour, 2, style), nil
}

// formatTimePart renders a plain zero-padded number read off the time.
func formatTimePart(d value.Datetime, mods []*modifier, get func(time.Time) int) (string, error) {
	style, err := padding(mods)
	if err != nil {
		return "", err
	}
	if !d.HasTime() {
		return "", errInsufficient
	}
	return pad(get(d.T), 2, style), nil
}

func formatPeriod(d value.Datetime, mods []*modifier) (string, error) {
	c, err := lookup(mods, "case", "upper", "upper", "lower")
	if err != nil {
		return "", err
	}
	if !d.HasTime() {
		return "", errInsufficient
	}
	s := "AM"
	if d.T.Hour() >= 12 {
		s = "PM"
	}
	if c == "lower" {
		s = strings.ToLower(s)
	}
	return s, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
