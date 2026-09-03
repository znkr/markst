package builtin

import (
	"fmt"
	"strings"
	"time"

	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	Datetime = &value.Function{
		Name: "datetime",
		Named: value.NamedParams{
			names.Year:   value.Param{Name: "year", Type: datetimeComponent, Default: value.None{}},
			names.Month:  value.Param{Name: "month", Type: datetimeComponent, Default: value.None{}},
			names.Day:    value.Param{Name: "day", Type: datetimeComponent, Default: value.None{}},
			names.Hour:   value.Param{Name: "hour", Type: datetimeComponent, Default: value.None{}},
			names.Minute: value.Param{Name: "minute", Type: datetimeComponent, Default: value.None{}},
			names.Second: value.Param{Name: "second", Type: datetimeComponent, Default: value.None{}},
		},
		F: datetimeImpl,
	}

	DatetimeToday = &value.Function{
		Name: "datetime.today",
		Named: value.NamedParams{
			names.Offset: value.Param{
				Name:    "offset",
				Type:    types.SetOf(types.Auto, types.Int, types.Duration),
				Default: value.Auto{},
			},
		},
		F: datetimeTodayImpl,
	}

	DatetimeParseDate = &value.Function{
		Name: "datetime.parse_date",
		Positional: []value.Param{
			{Name: "string", Type: types.SetOf(types.Str)},
		},
		F: datetimeParseDateImpl,
	}

	DatetimeDisplay = &value.Function{
		Name: "datetime.display",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Datetime)},
			{Name: "pattern", Type: types.SetOf(types.Str, types.None), Default: value.None{}},
		},
		F: datetimeDisplayImpl,
	}

	DatetimeYear    = datetimeComponentFunc("datetime.year", datetimeYear)
	DatetimeMonth   = datetimeComponentFunc("datetime.month", datetimeMonth)
	DatetimeDay     = datetimeComponentFunc("datetime.day", datetimeDay)
	DatetimeHour    = datetimeComponentFunc("datetime.hour", datetimeHour)
	DatetimeMinute  = datetimeComponentFunc("datetime.minute", datetimeMinute)
	DatetimeSecond  = datetimeComponentFunc("datetime.second", datetimeSecond)
	DatetimeWeekday = datetimeComponentFunc("datetime.weekday", datetimeWeekday)
	DatetimeOrdinal = datetimeComponentFunc("datetime.ordinal", datetimeOrdinal)
)

// datetimeComponent is the type accepted by the constructor's named arguments:
// an integer, or `none` for a component that is deliberately left out.
var datetimeComponent = types.SetOf(types.Int, types.None)

// datetimeComponentFunc builds an accessor method returning one component of a
// datetime, or `none` when the value doesn't carry that half of the calendar.
func datetimeComponentFunc(fname string, get func(value.Datetime) value.Value) *value.Function {
	return &value.Function{
		Name: fname,
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Datetime)},
		},
		F: func(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
			return get(args[0].(value.Datetime)), nil
		},
	}
}

func datetimeYear(d value.Datetime) value.Value {
	if !d.HasDate() {
		return value.None{}
	}
	return value.Int(d.T.Year())
}

func datetimeMonth(d value.Datetime) value.Value {
	if !d.HasDate() {
		return value.None{}
	}
	return value.Int(d.T.Month())
}

func datetimeDay(d value.Datetime) value.Value {
	if !d.HasDate() {
		return value.None{}
	}
	return value.Int(d.T.Day())
}

// datetimeWeekday numbers the days of the week starting at Monday = 1, the way
// ISO 8601 does.
func datetimeWeekday(d value.Datetime) value.Value {
	if !d.HasDate() {
		return value.None{}
	}
	return value.Int(isoWeekday(d.T))
}

// datetimeOrdinal is the 1-based day of the year.
func datetimeOrdinal(d value.Datetime) value.Value {
	if !d.HasDate() {
		return value.None{}
	}
	return value.Int(d.T.YearDay())
}

func datetimeHour(d value.Datetime) value.Value {
	if !d.HasTime() {
		return value.None{}
	}
	return value.Int(d.T.Hour())
}

func datetimeMinute(d value.Datetime) value.Value {
	if !d.HasTime() {
		return value.None{}
	}
	return value.Int(d.T.Minute())
}

func datetimeSecond(d value.Datetime) value.Value {
	if !d.HasTime() {
		return value.None{}
	}
	return value.Int(d.T.Second())
}

func datetimeImpl(_ *value.FunctionCallContext, _ []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	year, hasYear := datetimeArg(named, names.Year)
	month, hasMonth := datetimeArg(named, names.Month)
	day, hasDay := datetimeArg(named, names.Day)
	hour, hasHour := datetimeArg(named, names.Hour)
	minute, hasMinute := datetimeArg(named, names.Minute)
	second, hasSecond := datetimeArg(named, names.Second)

	// The two halves are validated separately, time first and date second, so
	// that a call leaving both half-specified is reported against its time
	// arguments, as in Typst.
	hasTime := hasHour || hasMinute || hasSecond
	if hasTime && !(hasHour && hasMinute && hasSecond) {
		return nil, incompleteErr("time", []string{"hour", "minute", "second"},
			[]bool{hasHour, hasMinute, hasSecond})
	}
	hasDate := hasYear || hasMonth || hasDay
	if hasDate && !(hasYear && hasMonth && hasDay) {
		return nil, incompleteErr("date", []string{"year", "month", "day"},
			[]bool{hasYear, hasMonth, hasDay})
	}

	switch {
	case hasDate && hasTime:
		d, ok := value.NewDatetime(year, month, day, hour, minute, second)
		if !ok {
			// A datetime is invalid for one of two reasons. Naming the half at fault
			// is more useful than a combined message.
			if _, ok := value.NewDate(year, month, day); !ok {
				return nil, value.ArgErrorPosf(0, "date is invalid")
			}
			return nil, value.ArgErrorPosf(0, "time is invalid")
		}
		return d, nil
	case hasDate:
		d, ok := value.NewDate(year, month, day)
		if !ok {
			return nil, value.ArgErrorPosf(0, "date is invalid")
		}
		return d, nil
	case hasTime:
		d, ok := value.NewTime(hour, minute, second)
		if !ok {
			return nil, value.ArgErrorPosf(0, "time is invalid")
		}
		return d, nil
	default:
		err := value.ArgErrorPosf(0, "at least one of date or time must be fully specified")
		err.Hint(missingHint("time", []string{"hour", "minute", "second"}))
		err.Hint(missingHint("date", []string{"year", "month", "day"}))
		return nil, err
	}
}

// datetimeArg reads one constructor argument, reporting whether it was given as
// an integer. An explicit `none` counts as absent, the same as not passing it.
func datetimeArg(named value.NamedArgsWithDefaults, n name.Name) (int, bool) {
	i, ok := named.Get(n).(value.Int)
	return int(i), ok
}

// incompleteErr reports a half-specified date or time, hinting at exactly the
// arguments that are missing.
func incompleteErr(what string, all []string, present []bool) error {
	var missing []string
	for i, name := range all {
		if !present[i] {
			missing = append(missing, name)
		}
	}
	err := value.ArgErrorPosf(0, "%s is incomplete", what)
	err.Hint(missingHint(what, missing))
	return err
}

// missingHint phrases the hint that names the arguments still needed to make a
// valid date or time.
func missingHint(what string, missing []string) string {
	quoted := make([]string, len(missing))
	for i, m := range missing {
		quoted[i] = "`" + m + "`"
	}
	noun := "arguments"
	if len(missing) == 1 {
		noun = "argument"
	}
	return fmt.Sprintf("add the %s %s to get a valid %s", joinAnd(quoted), noun, what)
}

// joinAnd joins items into an English list with an Oxford comma.
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

func datetimeTodayImpl(call *value.FunctionCallContext, _ []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	now := call.Now
	if now.IsZero() {
		now = time.Now()
	}

	// Without an offset the current date is read in the local time zone. With
	// one, it is read in the zone that far ahead of UTC.
	local := now
	switch off := named.Get(names.Offset).(type) {
	case value.Auto:
	case value.Int:
		// These are wall-clock offsets, so a full day or more is not a real time
		// zone.
		if off <= -24 || off >= 24 {
			return nil, fmt.Errorf("unable to get the current date")
		}
		local = now.UTC().Add(time.Duration(off) * time.Hour)
	case value.Duration:
		// A duration is canonical, so a non-zero day count means the offset is
		// already a day or more.
		if off.Days != 0 {
			return nil, fmt.Errorf("unable to get the current date")
		}
		local = now.UTC().Add(off.Time)
	default:
		panic("should not be reachable due to type checking")
	}

	d, _ := value.NewDate(local.Year(), int(local.Month()), local.Day())
	return d, nil
}

func datetimeParseDateImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	s := string(args[0].(value.Str))
	d, ok := value.ParseDate(s)
	if !ok {
		e := value.ArgErrorPosf(0, "invalid date: %q", s)
		e.Hint("dates must be written as `yyyy-mm-dd`, e.g. `2024-02-29`")
		return nil, e
	}
	return d, nil
}

func datetimeDisplayImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	d := args[0].(value.Datetime)
	pattern, ok := args[1].(value.Str)
	if !ok {
		pattern = value.Str(defaultPattern(d))
	}
	s, err := formatDatetime(d, string(pattern))
	if err != nil {
		return nil, err
	}
	return value.Str(s), nil
}

// defaultPattern is the format description `display` uses when called without
// one: whichever halves of the calendar the value actually carries.
func defaultPattern(d value.Datetime) string {
	switch d.Kind {
	case value.DateOnly:
		return "[year]-[month]-[day]"
	case value.TimeOnly:
		return "[hour]:[minute]:[second]"
	default:
		return "[year]-[month]-[day] [hour]:[minute]:[second]"
	}
}

// isoWeekday numbers t's day of the week with Monday = 1 and Sunday = 7.
func isoWeekday(t time.Time) int {
	if wd := int(t.Weekday()); wd != 0 {
		return wd
	}
	return 7
}
