package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	// The deployed image is distroless and carries no /usr/share/zoneinfo, so
	// without the embedded copy every LoadLocation would fail and the whole
	// family would quietly be put back on UTC.
	_ "time/tzdata"
)

const (
	// tzCookie holds the IANA zone the browser reports for itself.
	tzCookie = "sn_tz"
	// tzCookieMaxAge is a year: the zone only changes when someone travels or
	// the device is reconfigured, and the script rewrites it when that happens.
	tzCookieMaxAge = 365 * 24 * 60 * 60
)

// contextKey keeps our context value from colliding with anyone else's.
type contextKey struct{ name string }

var locationContextKey = contextKey{"location"}

// loadLocation turns an IANA name into a location, returning nil for anything
// blank or unrecognised so the caller can fall through to the next source.
// A stale cookie from a renamed zone should not be able to break a page.
func loadLocation(name string) *time.Location {
	name = strings.TrimSpace(name)
	// Real zone names are short; the cap keeps a junk cookie from turning into
	// a large lookup. LoadLocation itself rejects "..", absolute paths, and
	// anything else that could escape the zone database.
	if name == "" || len(name) > 64 {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

// resolveLocation decides whose clock a request is answered in.
//
// The family override wins when it is set, because the school day is a shared
// thing: a tablet someone left on the wrong zone should not move everyone's
// lessons. With no override the browser's own zone is used, so a phone in
// another state shows the day that phone is living in. The process zone is the
// last resort, which is what the desktop build runs on.
func resolveLocation(family, device string) *time.Location {
	if loc := loadLocation(family); loc != nil {
		return loc
	}
	if loc := loadLocation(device); loc != nil {
		return loc
	}
	return time.Local
}

func withLocation(ctx context.Context, loc *time.Location) context.Context {
	return context.WithValue(ctx, locationContextKey, loc)
}

// locationFrom falls back to the process zone so code reached outside the
// middleware — tests, and the startup template parse — still gets an answer.
func locationFrom(ctx context.Context) *time.Location {
	if loc, ok := ctx.Value(locationContextKey).(*time.Location); ok && loc != nil {
		return loc
	}
	return time.Local
}

func requestLocation(r *http.Request) *time.Location {
	if r == nil {
		return time.Local
	}
	return locationFrom(r.Context())
}

func nowIn(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	return time.Now().In(loc)
}

func todayIn(loc *time.Location) string {
	return nowIn(loc).Format(dateLayout)
}

// requestToday is the date the person making this request is living in, and is
// what every calendar decision should be measured against.
func requestToday(r *http.Request) string {
	return todayIn(requestLocation(r))
}

// requestNow is for the handful of places that need a whole timestamp rather
// than a date, such as snapping to the start of the current week.
func requestNow(r *http.Request) time.Time {
	return nowIn(requestLocation(r))
}

func cookieTZ(r *http.Request) string {
	if r == nil {
		return ""
	}
	c, err := r.Cookie(tzCookie)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(c.Value)
	// document.cookie writers often encodeURIComponent the value, which turns
	// America/Los_Angeles into America%2FLos_Angeles. LoadLocation will not
	// accept that, and we would silently fall back to UTC.
	if decoded, err := url.QueryUnescape(value); err == nil {
		value = strings.TrimSpace(decoded)
	}
	return value
}

// commonTimezones is the short list offered in Settings. Any other IANA name
// can be typed in; these are just the ones a US household is likely to want.
var commonTimezones = []string{
	"America/Los_Angeles",
	"America/Denver",
	"America/Phoenix",
	"America/Chicago",
	"America/New_York",
	"America/Anchorage",
	"Pacific/Honolulu",
	"UTC",
}

// today is the process zone's date. It stays for the desktop build and for
// things measured against the server's own clock, such as naming a backup
// file. Anything the calendar shows should use requestToday instead.
func today() string {
	return time.Now().Format(dateLayout)
}

// parseDateIn is parseDate for request paths: a missing or malformed date
// falls back to the day the reader is in, not the server's.
func parseDateIn(value string, loc *time.Location) time.Time {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(dateLayout, value); err == nil {
		return t
	}
	now := nowIn(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
