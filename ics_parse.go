package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// recurHorizonYears kept for tests that still reason about windows; import no
// longer copies years ahead because masters store the RRULE.
const recurHorizonYears = 2

// ParseICSEvents reads VEVENTs into adult events. A FREQ=YEARLY rule is kept on
// the master row; other frequencies become the original single day. Unsupported
// recurrence is accepted on import by dropping the rule. CalDAV PUT uses
// ParseICSEventStrict instead. loc is the clock used for UTC timestamps.
func ParseICSEvents(body []byte, loc *time.Location, now time.Time) ([]AdultEvent, error) {
	if loc == nil {
		loc = time.UTC
	}
	raw, err := readICSEvents(body, loc)
	if err != nil {
		return nil, err
	}
	out := materializeICSMasters(raw)
	if len(out) == 0 {
		return nil, fmt.Errorf("that calendar has no events we can read")
	}
	return out, nil
}

// ParseICSEventStrict reads exactly one VEVENT for a CalDAV PUT. Yearly rules
// are kept; any other recurrence is rejected.
func ParseICSEventStrict(body []byte, loc *time.Location) (AdultEvent, error) {
	if loc == nil {
		loc = time.UTC
	}
	raw, err := readICSEvents(body, loc)
	if err != nil {
		return AdultEvent{}, err
	}
	for _, ev := range raw {
		if ev.hasRecur {
			continue
		}
		if ev.rrule != "" {
			if _, ok := parseYearlyRule(ev.rrule); !ok {
				return AdultEvent{}, fmt.Errorf("only yearly repeating events are supported")
			}
		}
	}
	out := materializeICSMasters(raw)
	if len(out) != 1 {
		return AdultEvent{}, fmt.Errorf("calendar object must contain exactly one event")
	}
	return out[0], nil
}

// icsImportWindow is the first of this month through the same month two years on.
func icsImportWindow(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(recurHorizonYears, 0, -1)
	return start, end
}

type icsEvent struct {
	uid, title, body, location, rrule, status string
	start, startAt, endAt                     time.Time
	span                                      int
	allDay                                    bool
	exdates                                   []time.Time
	recurOn                                   time.Time
	hasRecur                                  bool
}

func (ev *icsEvent) cancelled() bool {
	return strings.EqualFold(strings.TrimSpace(ev.status), "CANCELLED")
}

func (ev *icsEvent) at(day time.Time) AdultEvent {
	span := ev.span
	if span < 1 {
		span = 1
	}
	ex := make([]string, 0, len(ev.exdates))
	for _, d := range ev.exdates {
		ex = append(ex, d.Format(dateLayout))
	}
	allDay := true
	startAt, endAt := "", ""
	if !ev.allDay {
		allDay = false
		startAt = ev.startAt.UTC().Format(time.RFC3339)
		endAt = ev.endAt.UTC().Format(time.RFC3339)
	}
	return AdultEvent{
		UID:      ev.uid,
		Title:    ev.title,
		Body:     ev.body,
		Location: ev.location,
		StartsOn: day.Format(dateLayout),
		EndsOn:   day.AddDate(0, 0, span-1).Format(dateLayout),
		AllDay:   allDay,
		StartAt:  startAt,
		EndAt:    endAt,
		RRule:    ev.rrule,
		ExDates:  strings.Join(ex, ","),
	}
}

func materializeICSMasters(raw []icsEvent) []AdultEvent {
	type group struct {
		master *icsEvent
		overs  map[string]*icsEvent
	}
	order := []string{}
	groups := map[string]*group{}
	anon := 0
	for i := range raw {
		ev := &raw[i]
		key := ev.uid
		if key == "" {
			anon++
			key = fmt.Sprintf("\x00%d", anon)
		}
		g := groups[key]
		if g == nil {
			g = &group{overs: map[string]*icsEvent{}}
			groups[key] = g
			order = append(order, key)
		}
		if ev.hasRecur {
			g.overs[ev.recurOn.Format(dateLayout)] = ev
			continue
		}
		if g.master == nil || (g.master.rrule == "" && ev.rrule != "") {
			g.master = ev
		}
	}

	var out []AdultEvent
	for _, key := range order {
		g := groups[key]
		if g.master == nil {
			for _, ov := range g.overs {
				if !ov.cancelled() {
					e := ov.at(ov.start)
					e.RRule = ""
					out = append(out, e)
				}
			}
			continue
		}
		if g.master.cancelled() {
			continue
		}
		if g.master.rrule != "" {
			if _, yearly := parseYearlyRule(g.master.rrule); !yearly {
				g.master.rrule = ""
			}
		}
		e := g.master.at(g.master.start)
		if e.RRule != "" {
			if _, yearly := parseYearlyRule(e.RRule); !yearly {
				e.RRule = ""
				e.ExDates = ""
			}
		}
		out = append(out, e)
		if e.RRule == "" {
			for _, ov := range g.overs {
				if !ov.cancelled() {
					oe := ov.at(ov.start)
					oe.RRule = ""
					out = append(out, oe)
				}
			}
		}
	}
	return out
}

type yearlyRule struct {
	interval int
	count    int
	until    time.Time
	hasUntil bool
	months   []time.Month
	mdays    []int
}

func parseYearlyRule(raw string) (yearlyRule, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return yearlyRule{}, false
	}
	rule := yearlyRule{interval: 1}
	yearly := false
	for _, part := range strings.Split(raw, ";") {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "FREQ":
			yearly = strings.EqualFold(val, "YEARLY")
		case "INTERVAL":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				rule.interval = n
			}
		case "COUNT":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				rule.count = n
			}
		case "UNTIL":
			if t, ok := parseICSUntil(val); ok {
				rule.until = t
				rule.hasUntil = true
			}
		case "BYMONTH":
			rule.months = parseICSMonths(val)
		case "BYMONTHDAY":
			rule.mdays = parseICSInts(val)
		}
	}
	if !yearly {
		return yearlyRule{}, false
	}
	return rule, true
}

func yearlyDays(year int, start time.Time, rule yearlyRule) []time.Time {
	months := rule.months
	if len(months) == 0 {
		months = []time.Month{start.Month()}
	}
	days := rule.mdays
	if len(days) == 0 {
		days = []int{start.Day()}
	}
	var out []time.Time
	for _, month := range months {
		for _, day := range days {
			if t, ok := dateInMonth(year, month, day); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

func dateInMonth(year int, month time.Month, day int) (time.Time, bool) {
	if day < 1 || day > 31 {
		return time.Time{}, false
	}
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || t.Month() != month || t.Day() != day {
		return time.Time{}, false
	}
	return t, true
}

func parseICSMonths(val string) []time.Month {
	var out []time.Month
	for _, part := range strings.Split(val, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 || n > 12 {
			continue
		}
		out = append(out, time.Month(n))
	}
	return out
}

func parseICSInts(val string) []int {
	var out []int
	for _, part := range strings.Split(val, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func parseICSUntil(val string) (time.Time, bool) {
	t, _, ok := parseICSStamp("", val, time.UTC)
	if !ok {
		return time.Time{}, false
	}
	return utcDate(t), true
}

type icsDraft struct {
	uid, title, body, location, rrule, status string
	start, end                                time.Time
	startOK, endOK                            bool
	startAllDay, endAllDay                    bool
	ex                                        []time.Time
	recur                                     time.Time
	recurOK                                   bool
}

func (d *icsDraft) event() (icsEvent, bool) {
	title := strings.TrimSpace(d.title)
	if !d.startOK || title == "" {
		return icsEvent{}, false
	}
	start := utcDate(d.start)
	allDay := d.startAllDay
	span := 1
	startAt, endAt := d.start, d.end
	if !d.endOK {
		endAt = d.start
	}
	if allDay || d.endAllDay {
		allDay = true
		if d.endOK {
			end := utcDate(d.end)
			n := daysBetween(start, end)
			if n < 1 {
				n = 1
			}
			span = n
		}
	} else {
		last := endAt.Add(-time.Second)
		n := daysBetween(start, utcDate(last)) + 1
		if n < 1 {
			n = 1
		}
		span = n
	}
	ev := icsEvent{
		uid:      strings.TrimSpace(d.uid),
		title:    title,
		body:     strings.TrimSpace(d.body),
		location: strings.TrimSpace(d.location),
		rrule:    strings.TrimSpace(d.rrule),
		status:   strings.TrimSpace(d.status),
		start:    start,
		startAt:  startAt,
		endAt:    endAt,
		allDay:   allDay,
		span:     span,
		exdates:  d.ex,
	}
	if d.recurOK {
		ev.hasRecur = true
		ev.recurOn = utcDate(d.recur)
	}
	return ev, true
}

func readICSEvents(body []byte, loc *time.Location) ([]icsEvent, error) {
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\r"), []byte("\n"))
	body = unfoldICS(body)

	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// depth 1 is the VEVENT itself. Nested pieces (an alarm) and the timezone
	// block that follows a calendar are not events, even when they carry a
	// DTSTART and an RRULE.
	var depth int
	var cur *icsDraft
	var out []icsEvent
	flush := func() {
		if cur == nil {
			return
		}
		if ev, ok := cur.event(); ok {
			out = append(out, ev)
		}
		cur = nil
	}

	for sc.Scan() {
		line := sc.Text()
		name, params, value, ok := splitICSProp(line)
		if !ok {
			continue
		}
		switch name {
		case "BEGIN":
			if strings.EqualFold(value, "VEVENT") && depth == 0 {
				flush()
				cur = &icsDraft{}
				depth = 1
				continue
			}
			if depth > 0 {
				depth++
			}
			continue
		case "END":
			if depth == 0 {
				continue
			}
			if depth == 1 && strings.EqualFold(value, "VEVENT") {
				flush()
				depth = 0
				continue
			}
			depth--
			continue
		}
		if depth != 1 || cur == nil {
			continue
		}
		switch name {
		case "SUMMARY":
			cur.title = icsUnescape(value)
		case "DESCRIPTION":
			cur.body = icsUnescape(value)
		case "LOCATION":
			cur.location = icsUnescape(value)
		case "UID":
			cur.uid = strings.TrimSpace(value)
		case "RRULE":
			cur.rrule = strings.TrimSpace(value)
		case "STATUS":
			cur.status = strings.TrimSpace(value)
		case "DTSTART":
			if t, allDay, ok := parseICSStamp(params, value, loc); ok {
				cur.start, cur.startOK, cur.startAllDay = t, true, allDay
			}
		case "DTEND":
			if t, allDay, ok := parseICSStamp(params, value, loc); ok {
				cur.end, cur.endOK, cur.endAllDay = t, true, allDay
			}
		case "EXDATE":
			for _, part := range strings.Split(value, ",") {
				if t, _, ok := parseICSStamp(params, part, loc); ok {
					cur.ex = append(cur.ex, utcDate(t))
				}
			}
		case "RECURRENCE-ID":
			if t, _, ok := parseICSStamp(params, value, loc); ok {
				cur.recur, cur.recurOK = t, true
			}
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func splitICSProp(line string) (name, params, value string, ok bool) {
	colon := indexUnquoted(line, ':')
	if colon < 0 {
		return "", "", "", false
	}
	head := line[:colon]
	value = line[colon+1:]
	name = head
	if semi := indexUnquoted(head, ';'); semi >= 0 {
		name = head[:semi]
		params = head[semi+1:]
	}
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return "", "", "", false
	}
	return name, params, value, true
}

func indexUnquoted(s string, sep byte) int {
	quoted := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if quoted && i+1 < len(s) {
				i++
			}
		default:
			if !quoted && s[i] == sep {
				return i
			}
		}
	}
	return -1
}

func icsParam(params, key string) string {
	key = strings.ToUpper(key)
	rest := params
	for rest != "" {
		semi := indexUnquoted(rest, ';')
		part := rest
		if semi >= 0 {
			part = rest[:semi]
			rest = rest[semi+1:]
		} else {
			rest = ""
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), key) {
			continue
		}
		return strings.Trim(strings.TrimSpace(v), `"`)
	}
	return ""
}

func parseICSStamp(params, value string, fallback *time.Location) (time.Time, bool, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false, false
	}
	if fallback == nil {
		fallback = time.UTC
	}
	kind := strings.ToUpper(icsParam(params, "VALUE"))
	allDay := kind == "DATE" || (kind != "DATE-TIME" && len(value) == 8 && digitsOnly(value))
	if allDay {
		if len(value) < 8 {
			return time.Time{}, false, false
		}
		t, err := time.Parse("20060102", value[:8])
		if err != nil {
			return time.Time{}, false, false
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true, true
	}
	loc := fallback
	if tz := icsParam(params, "TZID"); tz != "" {
		if loaded := loadLocation(tz); loaded != nil {
			loc = loaded
		}
	}
	if strings.HasSuffix(strings.ToUpper(value), "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		if err != nil {
			return time.Time{}, false, false
		}
		return t.In(loc), false, true
	}
	if t, err := time.Parse("20060102T150405-0700", value); err == nil {
		return t.In(loc), false, true
	}
	if t, err := time.Parse("20060102T150405-07:00", value); err == nil {
		return t.In(loc), false, true
	}
	if t, err := time.ParseInLocation("20060102T150405", value, loc); err == nil {
		return t, false, true
	}
	return time.Time{}, false, false
}

func digitsOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func utcDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}
