package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestResolveLocationPrefersFamilyThenDevice(t *testing.T) {
	family := resolveLocation("America/New_York", "America/Los_Angeles")
	if family.String() != "America/New_York" {
		t.Fatalf("family override was %s", family)
	}
	device := resolveLocation("", "America/Los_Angeles")
	if device.String() != "America/Los_Angeles" {
		t.Fatalf("device zone was %s", device)
	}
	if resolveLocation("not/a-zone", "also/bad") != time.Local {
		t.Fatal("junk names should fall through to the process zone")
	}
}

func TestEncodedTimezoneCookieStillResolves(t *testing.T) {
	if todayIn(time.UTC) == todayIn(time.Local) {
		t.Skip("this instant is the same calendar day in UTC and locally")
	}

	ta := newTestApp(t)
	ta.addKid("Mia")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: tzCookie, Value: "America%2FLos_Angeles"})
	rec := httptest.NewRecorder()
	ta.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("home returned %d", rec.Code)
	}
	want := formatDate(todayIn(loadLocation("America/Los_Angeles")), "Jan 2")
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("encoded cookie did not resolve to %s\n%s", want, rec.Body.String())
	}
}

func TestTodayFollowsTheBrowserCookie(t *testing.T) {
	if todayIn(time.UTC) == todayIn(time.Local) {
		t.Skip("this instant is the same calendar day in UTC and locally")
	}

	ta := newTestApp(t)
	ta.addKid("Mia")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: tzCookie, Value: "UTC"})
	rec := httptest.NewRecorder()
	ta.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("home returned %d", rec.Code)
	}
	want := formatDate(todayIn(time.UTC), "Jan 2")
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("home did not use the cookie's UTC today %s\n%s", want, rec.Body.String())
	}
}

func TestFamilyTimezoneWinsOverTheCookie(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")
	if err := ta.store.SetSetting(settingTimezone, "Pacific/Honolulu"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: tzCookie, Value: "UTC"})
	rec := httptest.NewRecorder()
	ta.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("home returned %d", rec.Code)
	}
	want := formatDate(todayIn(loadLocation("Pacific/Honolulu")), "Jan 2")
	utcDay := formatDate(todayIn(time.UTC), "Jan 2")
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("home did not use the family zone's today %s\n%s", want, rec.Body.String())
	}
	if want != utcDay && strings.Contains(rec.Body.String(), utcDay) && !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("home still showed UTC %s", utcDay)
	}
}

func TestSettingsSavesAndClearsFamilyTimezone(t *testing.T) {
	ta := newTestApp(t)
	status, _ := ta.post("/settings/timezone", url.Values{
		"preset": {"America/Chicago"},
	})
	if status != http.StatusSeeOther && status != http.StatusOK {
		t.Fatalf("save returned %d", status)
	}
	got, err := ta.store.Setting(settingTimezone)
	if err != nil || got != "America/Chicago" {
		t.Fatalf("saved %q (%v)", got, err)
	}

	status, _ = ta.post("/settings/timezone", url.Values{"preset": {""}})
	if status != http.StatusSeeOther && status != http.StatusOK {
		t.Fatalf("clear returned %d", status)
	}
	got, err = ta.store.Setting(settingTimezone)
	if err != nil || got != "" {
		t.Fatalf("cleared setting is %q (%v)", got, err)
	}
}
