package clock

import "time"

var GlobalClock Clock

// Now returns the current time.
//
// It is shorthand for GlobalClock.Now().
func Now() time.Time {
	return GlobalClock.Now()
}

// Since returns the time elapsed since t.
//
// It is shorthand for GlobalClock.Since(t).
func Since(t time.Time) time.Duration {
	return GlobalClock.Since(t)
}

// MustIncrement returns the current time or the previous time plus one second if
// the current time is not later than the previous time.
func MustIncrement(prev time.Time) time.Time {
	return GlobalClock.MustIncrement(prev)
}
