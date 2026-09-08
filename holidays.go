package main

import (
	"sort"
	"time"
)

// The classic US holidays are worked out for whichever year is on screen
// rather than stored as rows. Nobody has to seed them, they never run out at
// the end of some pre-filled range, and a family who scrolls to 2043 still
// sees Thanksgiving on the right Thursday.
//
// These are the days a household actually marks - the federal holidays plus
// the traditional ones people cook and decorate for. They are read-only on the
// calendar: anything she wants to change is her own event instead.
func usHolidays(year int) []Holiday {
	days := []Holiday{
		{onDate(year, time.January, 1), "New Year's Day"},
		{nthWeekdayOf(year, time.January, time.Monday, 3), "Martin Luther King Jr. Day"},
		{onDate(year, time.February, 2), "Groundhog Day"},
		{onDate(year, time.February, 14), "Valentine's Day"},
		{nthWeekdayOf(year, time.February, time.Monday, 3), "Presidents' Day"},
		{onDate(year, time.March, 17), "St. Patrick's Day"},
		{goodFriday(year), "Good Friday"},
		{easterSunday(year), "Easter Sunday"},
		{nthWeekdayOf(year, time.May, time.Sunday, 2), "Mother's Day"},
		{lastWeekdayOf(year, time.May, time.Monday), "Memorial Day"},
		{onDate(year, time.June, 14), "Flag Day"},
		{nthWeekdayOf(year, time.June, time.Sunday, 3), "Father's Day"},
		{onDate(year, time.June, 19), "Juneteenth"},
		{onDate(year, time.July, 4), "Independence Day"},
		{nthWeekdayOf(year, time.September, time.Monday, 1), "Labor Day"},
		{nthWeekdayOf(year, time.October, time.Monday, 2), "Columbus Day"},
		{onDate(year, time.October, 31), "Halloween"},
		{onDate(year, time.November, 11), "Veterans Day"},
		{nthWeekdayOf(year, time.November, time.Thursday, 4), "Thanksgiving"},
		{onDate(year, time.December, 24), "Christmas Eve"},
		{onDate(year, time.December, 25), "Christmas Day"},
		{onDate(year, time.December, 31), "New Year's Eve"},
	}
	sort.SliceStable(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	return days
}

// holidaysBetween collects the holidays landing in a range, which can span the
// turn of a year because a month grid shows a few days either side of itself.
func holidaysBetween(from, to string) []Holiday {
	start, end := parseDate(from), parseDate(to)
	if end.Before(start) {
		return nil
	}
	var found []Holiday
	for year := start.Year(); year <= end.Year(); year++ {
		for _, h := range usHolidays(year) {
			if h.Date >= from && h.Date <= to {
				found = append(found, h)
			}
		}
	}
	return found
}

func onDate(year int, month time.Month, day int) string {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format(dateLayout)
}

// nthWeekdayOf finds days named the way the calendar names them: the third
// Monday in January, the fourth Thursday in November.
func nthWeekdayOf(year int, month time.Month, weekday time.Weekday, n int) string {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(weekday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, offset+(n-1)*7).Format(dateLayout)
}

// lastWeekdayOf walks back from the end of the month, which is how Memorial
// Day is described.
func lastWeekdayOf(year int, month time.Month, weekday time.Weekday) string {
	last := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1)
	back := (int(last.Weekday()) - int(weekday) + 7) % 7
	return last.AddDate(0, 0, -back).Format(dateLayout)
}

// easterSunday uses the anonymous Gregorian computus. Easter is the one day
// here with no simple rule, and Good Friday hangs off it.
func easterSunday(year int) string {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return onDate(year, time.Month(month), day)
}

func goodFriday(year int) string {
	return addDays(easterSunday(year), -2)
}
