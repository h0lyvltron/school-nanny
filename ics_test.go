package main

import (
	"strings"
	"testing"
)

func TestICSRoundTripAllDay(t *testing.T) {
	events := []AdultEvent{
		{ID: 1, Title: "Piano, recital", Body: "Line one\nLine two", StartsOn: "2026-09-13", EndsOn: "2026-09-13"},
		{ID: 2, Title: "Trip", StartsOn: "2026-09-20", EndsOn: "2026-09-22"},
	}
	raw := EmitAdultEventsICS("Test calendar", events)
	body := string(raw)
	if !strings.Contains(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "SUMMARY:Piano\\, recital") {
		t.Fatalf("unexpected ics: %s", body)
	}
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20260913") {
		t.Fatal("missing DTSTART")
	}
	// Exclusive end: single day -> next day; span Sep 20-22 -> Sep 23
	if !strings.Contains(body, "DTEND;VALUE=DATE:20260914") {
		t.Fatal("single-day DTEND should be exclusive next day")
	}
	if !strings.Contains(body, "DTEND;VALUE=DATE:20260923") {
		t.Fatal("multi-day DTEND should be day after inclusive end")
	}

	parsed, err := ParseICSEvents(raw)
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
		AdultID: parent.ID, StartsOn: today(), EndsOn: today(),
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
	mustContain(t, body, "not part of school hours", "day glance note")
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
