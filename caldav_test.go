package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCalDAVDiscoveryAndRoundTrip(t *testing.T) {
	ta := newTestApp(t)
	parent := ta.parent()

	code, _ := ta.dav("OPTIONS", "/dav/", "", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated OPTIONS: %d", code)
	}

	pass := "calendar-secret"
	hash, err := hashPassword(pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := ta.store.SetSetting(settingCalendarPassword, hash); err != nil {
		t.Fatal(err)
	}
	if err := ta.store.SetSetting(settingCalendarUser, "calendar"); err != nil {
		t.Fatal(err)
	}

	code, body := ta.dav("OPTIONS", "/dav/", pass, nil)
	if code != 200 {
		t.Fatalf("OPTIONS: %d", code)
	}
	if !strings.Contains(body, "") && ta.server.Client() == nil {
		t.Fatal("options empty")
	}

	code, body = ta.dav("PROPFIND", "/dav/", pass, []byte(`<?xml version="1.0"?>
<D:propfind xmlns:D="DAV:"><D:prop><D:current-user-principal/></D:prop></D:propfind>`))
	if code != 207 {
		t.Fatalf("PROPFIND root: %d %s", code, body)
	}
	mustContain(t, body, calDAVPrincipal, "principal href")

	code, body = ta.dav("PROPFIND", calDAVHome, pass, []byte(`<?xml version="1.0"?>
<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop><D:displayname/><C:calendar-home-set/></D:prop>
</D:propfind>`))
	if code != 207 {
		t.Fatalf("PROPFIND home depth0: %d", code)
	}

	req, err := http.NewRequest("PROPFIND", ta.server.URL+calDAVHome, strings.NewReader(`<?xml version="1.0"?>
<D:propfind xmlns:D="DAV:"><D:prop><D:displayname/><D:resourcetype/></D:prop></D:propfind>`))
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("calendar", pass)
	req.Header.Set("Depth", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 207 {
		t.Fatalf("PROPFIND home depth1: %d %s", resp.StatusCode, raw)
	}
	mustContain(t, string(raw), parent.Name, "adult calendar name")
	mustContain(t, string(raw), fmt.Sprintf("%s%d/", calDAVHome, parent.ID), "adult href")

	vevent := []byte(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:birthday-caldav@test
DTSTART;VALUE=DATE:20151010
DTEND;VALUE=DATE:20151011
RRULE:FREQ=YEARLY
SUMMARY:CalDAV Birthday
END:VEVENT
END:VCALENDAR
`)
	href := calDAVEventHref(parent.ID, "birthday-caldav@test")
	code, _ = ta.dav("PUT", href, pass, vevent)
	if code != http.StatusCreated {
		t.Fatalf("PUT create: %d", code)
	}

	code, got := ta.dav("GET", href, pass, nil)
	if code != 200 {
		t.Fatalf("GET: %d", code)
	}
	mustContain(t, got, "RRULE:FREQ=YEARLY", "yearly rule round-trips")
	mustContain(t, got, "CalDAV Birthday", "title")

	nextYear := addMonths(today(), 12)
	day := nextYear[:5] + "10-10"
	if len(day) != 10 {
		day = fmt.Sprintf("%s-10-10", today()[:4])
		if day <= today() {
			day = fmt.Sprintf("%d-10-10", parseDate(today()).Year()+1)
		}
	}
	// Prefer the next Oct 10 on or after today.
	y := parseDate(today()).Year()
	day = fmt.Sprintf("%d-10-10", y)
	if day < today() {
		day = fmt.Sprintf("%d-10-10", y+1)
	}
	events, err := ta.store.AdultEventsOverlapping(parent.ID, day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Title != "CalDAV Birthday" {
		t.Fatalf("expanded birthday: %+v want %s", events, day)
	}

	code, report := ta.dav("REPORT", fmt.Sprintf("%s%d/", calDAVHome, parent.ID), pass, []byte(`<?xml version="1.0"?>
<D:sync-collection xmlns:D="DAV:">
  <D:sync-token/>
  <D:prop><D:getetag/></D:prop>
</D:sync-collection>`))
	if code != 207 {
		t.Fatalf("sync REPORT: %d %s", code, report)
	}
	mustContain(t, report, href, "sync lists event")
	mustContain(t, report, "sync-token", "sync token")

	// iOS follows sync with calendar-multiget using absolute hrefs.
	abs := ta.server.URL + href
	code, multi := ta.dav("REPORT", fmt.Sprintf("%s%d/", calDAVHome, parent.ID), pass, []byte(fmt.Sprintf(`<?xml version="1.0"?>
<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop><D:getetag/><C:calendar-data/></D:prop>
  <D:href>%s</D:href>
</C:calendar-multiget>`, abs)))
	if code != 207 {
		t.Fatalf("multiget absolute: %d %s", code, multi)
	}
	mustContain(t, multi, "CalDAV Birthday", "absolute multiget returns event body")
	mustContain(t, multi, "BEGIN:VCALENDAR", "calendar-data payload")

	code, calProp := ta.dav("PROPFIND", fmt.Sprintf("%s%d/", calDAVHome, parent.ID), pass, []byte(`<?xml version="1.0"?>
<D:propfind xmlns:D="DAV:" xmlns:CS="http://calendarserver.org/ns/">
  <D:prop><CS:getctag/><D:sync-token/></D:prop>
</D:propfind>`))
	if code != 207 {
		t.Fatalf("calendar propfind: %d", code)
	}
	mustContain(t, calProp, "getctag", "Apple getctag")

	code, _ = ta.dav("DELETE", href, pass, nil)
	if code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", code)
	}
	events, err = ta.store.AdultEventsOverlapping(parent.ID, day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("deleted event still present: %+v", events)
	}
}

func TestCalDAVPullsWebCreatedEvent(t *testing.T) {
	ta := newTestApp(t)
	parent := ta.parent()
	pass := "pull-secret"
	hash, err := hashPassword(pass)
	if err != nil {
		t.Fatal(err)
	}
	mustOK(t, ta.store.SetSetting(settingCalendarPassword, hash))
	mustOK(t, ta.store.SetSetting(settingCalendarUser, "calendar"))

	id, err := ta.store.CreateAdultEvent(AdultEvent{
		AdultID: parent.ID, AllDay: true,
		StartsOn: today(), EndsOn: today(),
		Title: "From the app",
	})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := ta.store.AdultEvent(id)
	if err != nil {
		t.Fatal(err)
	}

	code, query := ta.dav("REPORT", fmt.Sprintf("%s%d/", calDAVHome, parent.ID), pass, []byte(`<?xml version="1.0"?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop><D:getetag/><C:calendar-data/></D:prop>
  <C:filter><C:comp-filter name="VCALENDAR"/></C:filter>
</C:calendar-query>`))
	if code != 207 {
		t.Fatalf("calendar-query: %d %s", code, query)
	}
	mustContain(t, query, "From the app", "web event appears in query")
	mustContain(t, query, ev.UID, "uid in href or body")
}

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (ta *testApp) dav(method, path, password string, body []byte) (int, string) {
	ta.t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, ta.server.URL+path, rdr)
	if err != nil {
		ta.t.Fatalf("request: %v", err)
	}
	if password != "" {
		req.SetBasicAuth("calendar", password)
	}
	if method == "PROPFIND" || method == "REPORT" {
		req.Header.Set("Depth", "0")
		req.Header.Set("Content-Type", "application/xml")
	}
	if method == "PUT" {
		req.Header.Set("Content-Type", "text/calendar")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ta.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}
