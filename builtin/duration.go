package builtin

import (
	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	Duration = &value.Function{
		Name: "duration",
		Named: value.NamedParams{
			names.Seconds: value.Param{Name: "seconds", Type: types.SetOf(types.Int), Default: value.Int(0)},
			names.Minutes: value.Param{Name: "minutes", Type: types.SetOf(types.Int), Default: value.Int(0)},
			names.Hours:   value.Param{Name: "hours", Type: types.SetOf(types.Int), Default: value.Int(0)},
			names.Days:    value.Param{Name: "days", Type: types.SetOf(types.Int), Default: value.Int(0)},
			names.Weeks:   value.Param{Name: "weeks", Type: types.SetOf(types.Int), Default: value.Int(0)},
		},
		F: durationImpl,
	}

	DurationSeconds = durationUnitFunc("duration.seconds", 1)
	DurationMinutes = durationUnitFunc("duration.minutes", 60)
	DurationHours   = durationUnitFunc("duration.hours", 3600)
	DurationDays    = durationUnitFunc("duration.days", 86400)
	DurationWeeks   = durationUnitFunc("duration.weeks", 604800)
)

// durationUnitFunc builds a method returning the whole duration expressed in
// the given unit, computed from seconds. The result is usually fractional: a
// 90-minute duration is 1.5 hours.
func durationUnitFunc(fname string, unitSeconds float64) *value.Function {
	return &value.Function{
		Name: fname,
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Duration)},
		},
		F: func(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
			d := args[0].(value.Duration)
			return value.Float(d.Seconds() / unitSeconds), nil
		},
	}
}

func durationImpl(_ *value.FunctionCallContext, _ []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	unit := func(n name.Name) int64 {
		return int64(named.Get(n).(value.Int))
	}
	d, ok := value.NewDuration(
		unit(names.Weeks),
		unit(names.Days),
		unit(names.Hours),
		unit(names.Minutes),
		unit(names.Seconds),
	)
	if !ok {
		return nil, value.ErrValueTooLarge
	}
	return d, nil
}
