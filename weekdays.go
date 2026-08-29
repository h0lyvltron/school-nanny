package main

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

const maxOccurrences = 400

var (
	errNoWeekdays  = errors.New("pick at least one day of the week")
	errNoSeriesEnd = errors.New("a repeating lesson needs an end date or a number of times")
)

var weekdayLabels = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

type WeekdayChoice struct {
	Value int
	Label string
	On    bool
}

// isoWeekday numbers Monday as 1 through Sunday as 7.
func isoWeekday(t time.Time) int {
	w := int(t.Weekday())
	if w == 0 {
		return 7
	}
	return w
}

func isWeekdayMonFri(t time.Time) bool {
	w := t.Weekday()
	return w >= time.Monday && w <= time.Friday
}

func parseWeekdays(raw string) []int {
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 || n > 7 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func formatWeekdays(days []int) string {
	seen := map[int]bool{}
	var parts []string
	for d := 1; d <= 7; d++ {
		for _, n := range days {
			if n == d && !seen[d] {
				seen[d] = true
				parts = append(parts, strconv.Itoa(d))
			}
		}
	}
	return strings.Join(parts, ",")
}

func weekdaySet(raw string) map[int]bool {
	out := map[int]bool{}
	for _, n := range parseWeekdays(raw) {
		out[n] = true
	}
	return out
}

func weekdayChoices(mask string) []WeekdayChoice {
	set := weekdaySet(mask)
	out := make([]WeekdayChoice, 7)
	for i := 1; i <= 7; i++ {
		out[i-1] = WeekdayChoice{Value: i, Label: weekdayLabels[i-1], On: set[i]}
	}
	return out
}

func defaultWeekdays() string {
	return "1,2,3,4,5"
}

func formWeekdays(values []string) string {
	var days []int
	for _, v := range values {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			continue
		}
		days = append(days, n)
	}
	return formatWeekdays(days)
}

// occurrenceDates walks from start, keeping days whose ISO weekday is in the
// mask, until it hits the end date, the requested count, or the cap.
func occurrenceDates(start, end string, count int, weekdays []int) ([]string, error) {
	set := map[int]bool{}
	for _, n := range weekdays {
		if n >= 1 && n <= 7 {
			set[n] = true
		}
	}
	if len(set) == 0 {
		return nil, errNoWeekdays
	}
	startT, err := time.Parse(dateLayout, start)
	if err != nil {
		return nil, err
	}
	var endT time.Time
	hasEnd := end != ""
	if hasEnd {
		endT, err = time.Parse(dateLayout, end)
		if err != nil {
			return nil, err
		}
		if endT.Before(startT) {
			return nil, errNoSeriesEnd
		}
	}
	if count < 0 {
		count = 0
	}
	if count > maxOccurrences {
		count = maxOccurrences
	}
	if !hasEnd && count == 0 {
		return nil, errNoSeriesEnd
	}
	limit := count
	if limit == 0 {
		limit = maxOccurrences
	}

	var dates []string
	for t := startT; len(dates) < limit; t = t.AddDate(0, 0, 1) {
		if hasEnd && t.After(endT) {
			break
		}
		if set[isoWeekday(t)] {
			dates = append(dates, t.Format(dateLayout))
		}
		if !hasEnd && t.After(startT.AddDate(3, 0, 0)) && count == 0 {
			break
		}
	}
	return dates, nil
}

func weekdayCountMonFri(from, to string) int {
	start, err := time.Parse(dateLayout, from)
	if err != nil {
		return 0
	}
	end, err := time.Parse(dateLayout, to)
	if err != nil || end.Before(start) {
		return 0
	}
	n := 0
	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		if isWeekdayMonFri(t) {
			n++
		}
	}
	return n
}

func monthFirst(value string) string {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		t = time.Now()
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format(dateLayout)
}

func monthLast(value string) string {
	t, err := time.Parse(dateLayout, monthFirst(value))
	if err != nil {
		return value
	}
	return t.AddDate(0, 1, -1).Format(dateLayout)
}

func addMonths(value string, months int) string {
	t, err := time.Parse(dateLayout, monthFirst(value))
	if err != nil {
		return value
	}
	return t.AddDate(0, months, 0).Format(dateLayout)
}

func maxDate(a, b string) string {
	if a > b {
		return a
	}
	return b
}

func minDate(a, b string) string {
	if a != "" && (b == "" || a < b) {
		return a
	}
	return b
}
