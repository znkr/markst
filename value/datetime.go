package value

import (
	"cmp"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"znkr.io/writst/types"
)

// DatetimeKind distinguishes the three shapes a [Datetime] can take: a date
// without a time of day, a time of day without a date, or both together.
type DatetimeKind uint8

const (
	DateOnly DatetimeKind = iota
	TimeOnly
	DateAndTime
)

// Datetime is a date, a time of day, or both. Which of the two halves carry
// meaning is recorded in Kind; reading a component that the value doesn't have
// yields [None].
type Datetime struct {
	// T holds the components. It is always in UTC, and the half not covered by
	// Kind is zeroed: a date-only value sits at midnight, a time-only value on
	// January 1st of year 0.
	T time.Time

	// Kind records which halves of T are meaningful.
	Kind DatetimeKind
}

// NewDate returns a date without a time of day. The second result is false if
// the components don't describe a real date (e.g. February 30th).
func NewDate(year, month, day int) (Datetime, bool) {
	t, ok := makeTime(year, month, day, 0, 0, 0)
	return Datetime{T: t, Kind: DateOnly}, ok
}

// NewTime returns a time of day without a date. The second result is false if
// the components don't describe a real time.
func NewTime(hour, minute, second int) (Datetime, bool) {
	t, ok := makeTime(0, 1, 1, hour, minute, second)
	return Datetime{T: t, Kind: TimeOnly}, ok
}

// NewDatetime returns a date with a time of day. The second result is false if
// the components don't describe a real point in time.
func NewDatetime(year, month, day, hour, minute, second int) (Datetime, bool) {
	t, ok := makeTime(year, month, day, hour, minute, second)
	return Datetime{T: t, Kind: DateAndTime}, ok
}

// DatetimeFromTime returns t (converted to UTC) as a date-and-time value.
func DatetimeFromTime(t time.Time) Datetime {
	t = t.UTC()
	return Datetime{
		T:    time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC),
		Kind: DateAndTime,
	}
}

// makeTime builds a UTC time from calendar components, reporting whether the
// components describe a real point in time. [time.Date] normalizes out-of-range
// values (February 30th becomes March 1st or 2nd), so the only way to reject
// them is to check that the result still reads back the way it went in.
func makeTime(year, month, day, hour, minute, second int) (time.Time, bool) {
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	ok := t.Year() == year && int(t.Month()) == month && t.Day() == day &&
		t.Hour() == hour && t.Minute() == minute && t.Second() == second
	return t, ok
}

// HasDate reports whether the value carries a date.
func (d Datetime) HasDate() bool { return d.Kind != TimeOnly }

// HasTime reports whether the value carries a time of day.
func (d Datetime) HasTime() bool { return d.Kind != DateOnly }

func (d Datetime) String() string {
	var args []string
	if d.HasDate() {
		args = append(args,
			fmt.Sprintf("year: %d", d.T.Year()),
			fmt.Sprintf("month: %d", int(d.T.Month())),
			fmt.Sprintf("day: %d", d.T.Day()),
		)
	}
	if d.HasTime() {
		args = append(args,
			fmt.Sprintf("hour: %d", d.T.Hour()),
			fmt.Sprintf("minute: %d", d.T.Minute()),
			fmt.Sprintf("second: %d", d.T.Second()),
		)
	}
	return "datetime(" + strings.Join(args, ", ") + ")"
}

// Duration is a span of time, stored as a whole number of days plus a
// sub-day remainder rather than as one flat nanosecond count. A day is
// exactly 24 hours here — writst models no time zones, so there is no DST to
// make one shorter — which keeps the split exact while lifting the ±292-year
// ceiling a bare [time.Duration] would impose on a span.
//
// The representation is canonical: Time is always under 24 hours in magnitude
// and never disagrees in sign with Days, so two spans of equal length are
// equal field by field. Every operation that can leave that range reports
// [ErrValueTooLarge] rather than wrapping.
//
// The units are the fixed-length ones. Months and years are deliberately
// absent: their length depends on where in the calendar they land, so they
// cannot be a span on their own.
type Duration struct {
	// Days is the whole-day part of the span.
	Days int64

	// Time is what is left over, always less than 24 hours in magnitude.
	Time time.Duration
}

// day is the length of a day, which is what Days counts.
const day = 24 * time.Hour

// NewDuration returns the duration covering the given number of weeks, days,
// hours, minutes, and seconds, any of which may be negative. The second result
// is false if the total is wider than a duration can represent.
func NewDuration(weeks, days, hours, minutes, seconds int64) (Duration, bool) {
	whole := int64(0)
	var rest time.Duration
	addDays := func(n int64) bool {
		var ok bool
		whole, ok = addNoOverflow(whole, n)
		return ok
	}
	// A sub-day unit is split into whole days and a remainder before it is
	// converted to nanoseconds, so a large count of them can't overflow.
	addUnits := func(n, perDay int64, unit time.Duration) bool {
		rest += time.Duration(n%perDay) * unit
		return addDays(n / perDay)
	}
	w, ok := mulNoOverflow(weeks, 7)
	if !ok || !addDays(w) || !addDays(days) ||
		!addUnits(hours, 24, time.Hour) ||
		!addUnits(minutes, 24*60, time.Minute) ||
		!addUnits(seconds, 24*60*60, time.Second) {
		return Duration{}, false
	}
	return normalizeDuration(whole, rest)
}

// normalizeDuration puts a day count and a remainder into canonical form:
// whole days carried out of the remainder, and the two halves agreeing in
// sign. It reports false if the day count overflows.
func normalizeDuration(days int64, rest time.Duration) (Duration, bool) {
	carry := int64(rest / day)
	rest -= time.Duration(carry) * day
	days, ok := addNoOverflow(days, carry)
	if !ok {
		return Duration{}, false
	}
	// Carrying alone can still leave the halves pulling in opposite
	// directions — one day minus one hour — so borrow a day back.
	switch {
	case rest < 0 && days > 0:
		days, rest = days-1, rest+day
	case rest > 0 && days < 0:
		days, rest = days+1, rest-day
	}
	return Duration{Days: days, Time: rest}, true
}

// Neg returns the duration with its sign flipped, reporting false when the
// result doesn't fit: the day count reaches one step further below zero than
// above it, so the most negative duration has no positive counterpart.
func (d Duration) Neg() (Duration, bool) {
	if d.Days == math.MinInt64 {
		return Duration{}, false
	}
	return Duration{Days: -d.Days, Time: -d.Time}, true
}

// Compare orders two durations by length. The canonical form makes this a
// plain lexicographic comparison of the two halves.
func (d Duration) Compare(o Duration) int {
	if c := cmp.Compare(d.Days, o.Days); c != 0 {
		return c
	}
	return cmp.Compare(d.Time, o.Time)
}

// Seconds returns the whole span as a number of seconds. Spans far wider than
// a lifetime lose sub-second precision to the float, which is the price of
// answering at all.
func (d Duration) Seconds() float64 {
	return float64(d.Days)*86400 + d.Time.Seconds()
}

func mulNoOverflow(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	if c/b != a {
		return 0, false
	}
	return c, true
}

func addNoOverflow(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}

// subNoOverflow subtracts b from a, reporting whether the result fits.
func subNoOverflow(a, b int64) (int64, bool) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, false
	}
	return a - b, true
}

func (d Duration) String() string {
	var args []string
	unit := func(name string, n int64) {
		if n != 0 {
			args = append(args, fmt.Sprintf("%s: %d", name, n))
		}
	}
	unit("weeks", d.Days/7)
	unit("days", d.Days%7)
	rest := d.Time
	sub := func(name string, size time.Duration) {
		if n := rest / size; n != 0 {
			args = append(args, fmt.Sprintf("%s: %d", name, n))
			rest -= n * size
		}
	}
	sub("hours", time.Hour)
	sub("minutes", time.Minute)
	// Seconds carry whatever is left, sub-second remainder included, so that a
	// duration that isn't a whole number of seconds doesn't print as one that
	// is.
	if rest != 0 {
		if rest%time.Second == 0 {
			args = append(args, fmt.Sprintf("seconds: %d", rest/time.Second))
		} else {
			args = append(args, "seconds: "+strconv.FormatFloat(rest.Seconds(), 'g', -1, 64))
		}
	}
	return "duration(" + strings.Join(args, ", ") + ")"
}

func (Datetime) aValue() {}
func (Duration) aValue() {}

func (Datetime) Type() types.Type { return types.Datetime }
func (Duration) Type() types.Type { return types.Duration }
