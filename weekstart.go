package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const (
	settingWeekStart = "week_starts_on"
	weekStartMonday  = "monday"
	weekStartSunday  = "sunday"
)

var weekStartContextKey = contextKey{"weekStart"}

// parseWeekStart maps the stored setting to a Go weekday. Anything blank or
// unrecognised keeps Monday, which is how the planner has always laid out.
func parseWeekStart(value string) time.Weekday {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case weekStartSunday, "sun", "0":
		return time.Sunday
	default:
		return time.Monday
	}
}

func weekStartValue(day time.Weekday) string {
	if day == time.Sunday {
		return weekStartSunday
	}
	return weekStartMonday
}

func withWeekStartDay(ctx context.Context, day time.Weekday) context.Context {
	return context.WithValue(ctx, weekStartContextKey, day)
}

func weekStartDay(ctx context.Context) time.Weekday {
	if day, ok := ctx.Value(weekStartContextKey).(time.Weekday); ok {
		return day
	}
	return time.Monday
}

func requestWeekStartDay(r *http.Request) time.Weekday {
	if r == nil {
		return time.Monday
	}
	return weekStartDay(r.Context())
}

func (a *App) familyWeekStart() time.Weekday {
	if a == nil || a.store == nil {
		return time.Monday
	}
	value, err := a.store.Setting(settingWeekStart)
	if err != nil {
		return time.Monday
	}
	return parseWeekStart(value)
}

// weekStartOn snaps t to the first day of its week for the given start day.
func weekStartOn(t time.Time, start time.Weekday) time.Time {
	offset := (int(t.Weekday()) - int(start) + 7) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, t.Location())
}

// weekStart snaps to Monday. Prefer requestWeekStart when a request is in play
// so the family's configured start day is honoured.
func weekStart(t time.Time) time.Time {
	return weekStartOn(t, time.Monday)
}

func requestWeekStart(r *http.Request, t time.Time) time.Time {
	return weekStartOn(t, requestWeekStartDay(r))
}

func weekDatesOn(date string, start time.Weekday) []string {
	begin := weekStartOn(parseDate(date), start).Format(dateLayout)
	days := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		days = append(days, addDays(begin, i))
	}
	return days
}

func weekDates(date string) []string {
	return weekDatesOn(date, time.Monday)
}

func requestWeekDates(r *http.Request, date string) []string {
	return weekDatesOn(date, requestWeekStartDay(r))
}

// weekdayHeaders are the short column labels for month calendars, in the order
// the grid fills left to right.
func weekdayHeaders(start time.Weekday) []string {
	if start == time.Sunday {
		return []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	}
	return []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
}
