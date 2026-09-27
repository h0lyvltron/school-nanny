package main

import (
	"strings"
	"time"
)

// ExpandAdultEvents turns stored masters into the concrete days a month view
// needs. A yearly birthday becomes one row per anniversary in the range; other
// events pass through unchanged.
func ExpandAdultEvents(masters []AdultEvent, from, to string) []AdultEvent {
	windowStart, errFrom := time.Parse(dateLayout, from)
	windowEnd, errTo := time.Parse(dateLayout, to)
	if errFrom != nil || errTo != nil {
		return masters
	}
	var out []AdultEvent
	for _, e := range masters {
		hits, yearly := adultEventYearlyHits(e, windowStart, windowEnd)
		if !yearly {
			if e.StartsOn <= to && e.EndsOn >= from {
				out = append(out, e)
			}
			continue
		}
		span := daysBetween(parseDate(e.StartsOn), parseDate(e.EndsOn)) + 1
		if span < 1 {
			span = 1
		}
		for _, day := range hits {
			occ := e
			occ.StartsOn = day.Format(dateLayout)
			occ.EndsOn = day.AddDate(0, 0, span-1).Format(dateLayout)
			occ.RRule = "" // occurrence drawn for the grid, not a master
			out = append(out, occ)
		}
	}
	return out
}

func adultEventYearlyHits(e AdultEvent, windowStart, windowEnd time.Time) ([]time.Time, bool) {
	rule, ok := parseYearlyRule(e.RRule)
	if !ok {
		return nil, false
	}
	start, err := time.Parse(dateLayout, e.StartsOn)
	if err != nil {
		return nil, true
	}
	ex := map[string]bool{}
	for _, part := range strings.Split(e.ExDates, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			ex[part] = true
		}
	}
	var hits []time.Time
	generated := 0
	for year := start.Year(); ; year += rule.interval {
		if year > windowEnd.Year()+1 {
			break
		}
		days := yearlyDays(year, start, rule)
		stop := false
		for _, day := range days {
			if day.Before(start) {
				continue
			}
			if rule.hasUntil && day.After(rule.until) {
				stop = true
				break
			}
			if rule.count > 0 && generated >= rule.count {
				stop = true
				break
			}
			generated++
			if ex[day.Format(dateLayout)] {
				continue
			}
			if day.Before(windowStart) {
				continue
			}
			if day.After(windowEnd) {
				stop = true
				break
			}
			hits = append(hits, day)
		}
		if stop {
			break
		}
	}
	return hits, true
}
