package main

import (
	"sort"
	"strings"
	"time"
)

// The classic US holidays are worked out for whichever year is on screen
// rather than stored as rows. Nobody has to seed them, they never run out at
// the end of some pre-filled range, and a family who scrolls to 2043 still
// sees Thanksgiving on the right Thursday.
//
// These are the days a household actually marks - the federal holidays plus
// the traditional ones people cook and decorate for. Each carries a default
// emoji; an adult can swap it (or add a note) without changing the holiday
// itself for anyone else.
func usHolidays(year int) []Holiday {
	days := []Holiday{
		{Date: onDate(year, time.January, 1), Name: "New Year's Day", Emoji: "🎉"},
		{Date: nthWeekdayOf(year, time.January, time.Monday, 3), Name: "Martin Luther King Jr. Day", Emoji: "✊"},
		{Date: onDate(year, time.February, 2), Name: "Groundhog Day", Emoji: "🦫"},
		{Date: onDate(year, time.February, 14), Name: "Valentine's Day", Emoji: "💝"},
		{Date: nthWeekdayOf(year, time.February, time.Monday, 3), Name: "Presidents' Day", Emoji: "🇺🇸"},
		{Date: onDate(year, time.March, 17), Name: "St. Patrick's Day", Emoji: "☘️"},
		{Date: goodFriday(year), Name: "Good Friday", Emoji: "✝️"},
		{Date: easterSunday(year), Name: "Easter Sunday", Emoji: "🐣"},
		{Date: nthWeekdayOf(year, time.May, time.Sunday, 2), Name: "Mother's Day", Emoji: "💐"},
		{Date: lastWeekdayOf(year, time.May, time.Monday), Name: "Memorial Day", Emoji: "🫡"},
		{Date: onDate(year, time.June, 14), Name: "Flag Day", Emoji: "🚩"},
		{Date: nthWeekdayOf(year, time.June, time.Sunday, 3), Name: "Father's Day", Emoji: "👔"},
		{Date: onDate(year, time.July, 4), Name: "Independence Day", Emoji: "🇺🇸"},
		{Date: nthWeekdayOf(year, time.September, time.Monday, 1), Name: "Labor Day", Emoji: "🛠️"},
		{Date: nthWeekdayOf(year, time.October, time.Monday, 2), Name: "Columbus Day", Emoji: "⛵"},
		{Date: onDate(year, time.October, 31), Name: "Halloween", Emoji: "🎃"},
		{Date: onDate(year, time.November, 11), Name: "Veterans Day", Emoji: "🎖️"},
		{Date: nthWeekdayOf(year, time.November, time.Thursday, 4), Name: "Thanksgiving", Emoji: "🦃"},
		{Date: onDate(year, time.December, 24), Name: "Christmas Eve", Emoji: "🎄"},
		{Date: onDate(year, time.December, 25), Name: "Christmas Day", Emoji: "🎄"},
		{Date: onDate(year, time.December, 31), Name: "New Year's Eve", Emoji: "🥂"},
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

// easterSunday is the anonymous Gregorian computus: the same arithmetic the
// Western churches have used for centuries to put Easter on the right Sunday.
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

// calendarEmojis is the shared palette for holiday personalization and label
// icons. It starts with every holiday's default, then adds common family and
// school marks so Labor Day's wrench is here alongside birthday cakes.
func calendarEmojis() []string {
	seen := map[string]bool{}
	var out []string
	add := func(emoji string) {
		emoji = strings.TrimSpace(emoji)
		if emoji == "" || seen[emoji] {
			return
		}
		seen[emoji] = true
		out = append(out, emoji)
	}
	for _, h := range usHolidays(2026) {
		add(h.Emoji)
	}
	for _, emoji := range []string{
		"🎂", "🎁", "🕯️", "🎆", "🎇", "✨", "❄️", "☃️", "🌸", "🌻",
		"🏥", "🦷", "✈️", "🏫", "💼", "⚽", "🎵", "📚", "👨‍👩‍👧‍👦", "🏠",
		"🚗", "🗓️", "📝", "💡", "❤️", "⭐", "🌈", "🐶", "🐱", "☕",
		"🧹", "🛒", "💊", "🧘", "🎨", "🎤", "🎬", "🏕️", "🏖️", "🧳",
	} {
		add(emoji)
	}
	return out
}
