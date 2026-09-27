package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestICSRoundTripAllDay(t *testing.T) {
	events := []AdultEvent{
		{ID: 1, UID: "a@test", Title: "Piano, recital", Body: "Line one\nLine two", StartsOn: "2026-09-13", EndsOn: "2026-09-13", AllDay: true},
		{ID: 2, UID: "b@test", Title: "Trip", StartsOn: "2026-09-20", EndsOn: "2026-09-22", AllDay: true},
	}
	raw := EmitAdultEventsICS("Test calendar", events)
	body := string(raw)
	if !strings.Contains(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "SUMMARY:Piano\\, recital") {
		t.Fatalf("unexpected ics: %s", body)
	}
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20260913") {
		t.Fatal("missing DTSTART")
	}
	if !strings.Contains(body, "DTEND;VALUE=DATE:20260914") {
		t.Fatal("single-day DTEND should be exclusive next day")
	}
	if !strings.Contains(body, "DTEND;VALUE=DATE:20260923") {
		t.Fatal("multi-day DTEND should be day after inclusive end")
	}

	parsed, err := ParseICSEvents(raw, time.UTC, time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("got %d events", len(parsed))
	}
	if parsed[0].Title != "Piano, recital" || parsed[0].StartsOn != "2026-09-13" || parsed[0].EndsOn != "2026-09-13" {
		t.Fatalf("event 0: %+v", parsed[0])
	}
	if parsed[1].StartsOn != "2026-09-20" || parsed[1].EndsOn != "2026-09-22" {
		t.Fatalf("event 1 dates: %+v", parsed[1])
	}
	if !strings.Contains(parsed[0].Body, "Line one") {
		t.Fatalf("body lost newlines: %q", parsed[0].Body)
	}
}

func TestTodayDayGlanceAndICSExportImport(t *testing.T) {
	ta := newTestApp(t)
	parent := ta.parent()
	ta.addKid("Mia")
	_, err := ta.store.CreateAdultEvent(AdultEvent{
		AdultID: parent.ID, StartsOn: today(), EndsOn: today(), AllDay: true,
		Title: "Library run", Body: "Return books",
	})
	if err != nil {
		t.Fatal(err)
	}

	code, body := ta.get("/")
	if code != 200 {
		t.Fatalf("today: %d", code)
	}
	mustContain(t, body, "Also today", "day glance heading")
	mustContain(t, body, "Household calendar", "day glance note")
	mustContain(t, body, "Library run", "event title")
	mustContain(t, body, `href="/adults/`+itoa64(parent.ID)+`"`, "event links to adult")

	code, ics := ta.get("/adults/" + itoa64(parent.ID) + "/calendar.ics")
	if code != 200 {
		t.Fatalf("ics export: %d", code)
	}
	mustContain(t, ics, "BEGIN:VCALENDAR", "ics")
	mustContain(t, ics, "Library run", "ics title")

	importBody := []byte(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:x@test
DTSTART;VALUE=DATE:20260915
DTEND;VALUE=DATE:20260916
SUMMARY:Imported appointment
DESCRIPTION:From ICS
END:VEVENT
END:VCALENDAR
`)
	code, _ = ta.postFile("/adults/"+itoa64(parent.ID)+"/calendar/import", "file", "trip.ics", importBody)
	if code != 200 {
		t.Fatalf("ics import: %d", code)
	}
	events, err := ta.store.AdultEventsOverlapping(parent.ID, "2026-09-15", "2026-09-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Title != "Imported appointment" {
		t.Fatalf("imported events: %+v", events)
	}

	code, body = ta.get("/static/manifest.webmanifest")
	if code != 200 || !strings.Contains(body, `"name": "School Nanny"`) {
		t.Fatalf("manifest: %d %q", code, body)
	}
	code, _ = ta.get("/static/sw.js")
	if code != 200 {
		t.Fatalf("service worker: %d", code)
	}
}

func TestICSYearlyBirthdayKeepsMasterRule(t *testing.T) {
	loc := loadLocation("America/Los_Angeles")
	now := time.Date(2026, 9, 21, 22, 0, 0, 0, loc)
	body := []byte(`BEGIN:VCALENDAR
BEGIN:VEVENT
UID:mom@test
DTSTART;VALUE=DATE:20231010
DTEND;VALUE=DATE:20231011
RRULE:FREQ=YEARLY
SUMMARY:Mom's birthday
END:VEVENT
BEGIN:VEVENT
UID:crystal@test
DTSTART;VALUE=DATE:20170202
DTEND;VALUE=DATE:20170203
EXDATE;VALUE=DATE:20260202
RRULE:FREQ=YEARLY
SUMMARY:Crystal birthday
END:VEVENT
BEGIN:VEVENT
UID:ended@test
DTSTART;VALUE=DATE:20170828
DTEND;VALUE=DATE:20170830
RRULE:FREQ=YEARLY;UNTIL=20220828
SUMMARY:Old birthday
END:VEVENT
BEGIN:VEVENT
UID:wedding@test
DTSTART;VALUE=DATE:20170610
DTEND;VALUE=DATE:20170612
SUMMARY:Wedding
END:VEVENT
BEGIN:VTIMEZONE
TZID:America/Los_Angeles
BEGIN:DAYLIGHT
DTSTART:20070311T020000
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU
SUMMARY:Not an event
END:DAYLIGHT
END:VTIMEZONE
END:VCALENDAR
`)
	events, err := ParseICSEvents(body, loc, now)
	if err != nil {
		t.Fatal(err)
	}
	byUID := map[string]AdultEvent{}
	for _, e := range events {
		byUID[e.UID] = e
	}
	mom := byUID["mom@test"]
	if mom.RRule == "" || mom.StartsOn != "2023-10-10" {
		t.Fatalf("mom master: %+v", mom)
	}
	crystal := byUID["crystal@test"]
	if !strings.Contains(crystal.ExDates, "2026-02-02") {
		t.Fatalf("crystal exdates: %+v", crystal)
	}
	old := byUID["ended@test"]
	if old.RRule == "" || old.StartsOn != "2017-08-28" {
		t.Fatalf("old birthday: %+v", old)
	}
	wedding := byUID["wedding@test"]
	if wedding.RRule != "" || wedding.StartsOn != "2017-06-10" || wedding.EndsOn != "2017-06-11" {
		t.Fatalf("wedding: %+v", wedding)
	}

	expanded := ExpandAdultEvents(events, "2026-09-01", "2028-08-31")
	got := map[string]bool{}
	for _, e := range expanded {
		got[e.Title+"|"+e.StartsOn] = true
	}
	for _, key := range []string{
		"Mom's birthday|2026-10-10",
		"Mom's birthday|2027-10-10",
		"Crystal birthday|2027-02-02",
		"Crystal birthday|2028-02-02",
	} {
		if !got[key] {
			t.Errorf("missing expanded %s", key)
		}
	}
	if got["Crystal birthday|2026-02-02"] {
		t.Error("excluded crystal date expanded")
	}
	if got["Not an event|2007-03-11"] {
		t.Error("timezone rule imported")
	}
}

func TestICSImportOpensOnNextYearlyDate(t *testing.T) {
	ta := newTestApp(t)
	parent := ta.parent()
	nextMonth := addMonths(today(), 1)
	day := nextMonth[:8] + "10"
	start := "2010-" + day[5:]
	end := addDays(start, 1)
	body := []byte(fmt.Sprintf(`BEGIN:VCALENDAR
BEGIN:VEVENT
UID:bday@test
DTSTART;VALUE=DATE:%s
DTEND;VALUE=DATE:%s
RRULE:FREQ=YEARLY
SUMMARY:Imported birthday
END:VEVENT
END:VCALENDAR
`, strings.ReplaceAll(start, "-", ""), strings.ReplaceAll(end, "-", "")))

	code, page := ta.postFile("/adults/"+itoa64(parent.ID)+"/calendar/import", "file", "birthdays.ics", body)
	if code != 200 {
		t.Fatalf("import: %d", code)
	}
	mustContain(t, page, "Imported birthday", "birthday title")
	mustContain(t, page, formatDate(day, "January 2006"), "opens on the anniversary month")

	events, err := ta.store.AdultEventsOverlapping(parent.ID, day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].StartsOn != day {
		t.Fatalf("anniversary: %+v", events)
	}
	masters, err := ta.store.AdultEventMasters(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	var master AdultEvent
	for _, e := range masters {
		if e.Title == "Imported birthday" {
			master = e
			break
		}
	}
	if master.RRule == "" || master.StartsOn != start {
		t.Fatalf("master should keep original DTSTART and RRULE: %+v", master)
	}

	code, page = ta.postFile("/adults/"+itoa64(parent.ID)+"/calendar/import", "file", "birthdays.ics", body)
	if code != 200 {
		t.Fatalf("second import: %d", code)
	}
	mustContain(t, page, "Nothing new to add", "second import")
}

func TestReferenceHomeCalendarYearly(t *testing.T) {
	path := os.Getenv("ICS_REFERENCE")
	if path == "" {
		t.Skip("set ICS_REFERENCE to check a real export")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loc := loadLocation("America/Los_Angeles")
	now := time.Date(2026, 9, 21, 22, 0, 0, 0, loc)
	events, err := ParseICSEvents(raw, loc, now)
	if err != nil {
		t.Fatal(err)
	}
	expanded := ExpandAdultEvents(events, "2026-09-01", "2028-08-31")
	has := func(title, date string) bool {
		for _, e := range expanded {
			if strings.TrimSpace(e.Title) == title && e.StartsOn == date {
				return true
			}
		}
		return false
	}
	for _, pair := range [][2]string{
		{"Mom's birthday", "2026-10-10"},
		{"Mom's birthday", "2027-10-10"},
		{"Lux’s Birthday", "2026-11-22"},
		{"Lux’s Birthday", "2027-11-22"},
		{"Crystals birthday", "2027-02-02"},
		{"Alichia's birthday", "2027-08-03"},
		{"Sayaka's birthday", "2027-08-01"},
	} {
		if !has(pair[0], pair[1]) {
			t.Errorf("missing %s on %s", pair[0], pair[1])
		}
	}
	if has("Crystals birthday", "2026-02-02") {
		t.Error("excluded 2026 crystal birthday was imported")
	}
	for _, e := range events {
		if strings.HasPrefix(e.StartsOn, "1883") || strings.HasPrefix(e.StartsOn, "1918") {
			t.Fatalf("timezone rule imported as an event: %+v", e)
		}
	}
}
