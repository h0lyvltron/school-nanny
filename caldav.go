package main

import (
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	calDAVRoot      = "/dav/"
	calDAVPrincipal = "/dav/principals/family/"
	calDAVHome      = "/dav/calendars/"
	maxCalDAVBytes  = 2 << 20
)

func (a *App) handleWellKnownCalDAV(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, calDAVRoot, http.StatusMovedPermanently)
}

func (a *App) handleCalDAV(w http.ResponseWriter, r *http.Request) {
	app, ok := a.authorizeCalDAV(w, r)
	if !ok {
		return
	}
	path := r.URL.Path
	if path == "/dav" {
		path = calDAVRoot
	}
	if !strings.HasPrefix(path, calDAVRoot) {
		a.notFound(w)
		return
	}

	switch r.Method {
	case http.MethodOptions:
		app.calDAVOptions(w, path)
	case "PROPFIND":
		app.calDAVPropfind(w, r, path)
	case "REPORT":
		app.calDAVReport(w, r, path)
	case http.MethodGet, http.MethodHead:
		app.calDAVGet(w, r, path)
	case http.MethodPut:
		app.calDAVPut(w, r, path)
	case http.MethodDelete:
		app.calDAVDelete(w, r, path)
	default:
		w.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) calDAVOptions(w http.ResponseWriter, path string) {
	w.Header().Set("DAV", "1, 3, calendar-access")
	w.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
	w.WriteHeader(http.StatusOK)
}

// authorizeCalDAV checks HTTP Basic against the calendar password. In hosted
// mode the username is the family slug (optionally "slug/calendar"), and the
// password lives in that family's settings.
func (a *App) authorizeCalDAV(w http.ResponseWriter, r *http.Request) (*App, bool) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		a.calDAVUnauthorized(w)
		return nil, false
	}
	app := a
	calUser := user
	if a.hosted {
		if a.control == nil {
			a.calDAVUnauthorized(w)
			return nil, false
		}
		slug := user
		calUser = "calendar"
		if i := strings.IndexByte(user, '/'); i >= 0 {
			slug = user[:i]
			if rest := strings.TrimSpace(user[i+1:]); rest != "" {
				calUser = rest
			}
		}
		fam, err := a.control.FamilyBySlug(slug)
		if err != nil {
			a.calDAVUnauthorized(w)
			return nil, false
		}
		tenant, err := a.tenantApp(fam.ID)
		if err != nil {
			a.serverError(w, err)
			return nil, false
		}
		app = tenant
	}
	wantUser, err := app.store.Setting(settingCalendarUser)
	if err != nil {
		a.serverError(w, err)
		return nil, false
	}
	if wantUser == "" {
		wantUser = "calendar"
	}
	hash, err := app.store.Setting(settingCalendarPassword)
	if err != nil {
		a.serverError(w, err)
		return nil, false
	}
	if hash == "" || subtleConstantTimeEq(calUser, wantUser) == 0 || !checkPassword(hash, pass) {
		a.calDAVUnauthorized(w)
		return nil, false
	}
	return app, true
}

func subtleConstantTimeEq(a, b string) int {
	if len(a) != len(b) {
		return 0
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	if v == 0 {
		return 1
	}
	return 0
}

func (a *App) calDAVUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="School Nanny Calendar"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func isCalDAVRoot(path string) bool {
	return path == calDAVRoot || path == "/dav"
}

func isCalDAVPrincipal(path string) bool {
	return path == calDAVPrincipal || path == strings.TrimSuffix(calDAVPrincipal, "/")
}

func isCalDAVHome(path string) bool {
	return path == calDAVHome || path == strings.TrimSuffix(calDAVHome, "/")
}

func isCalDAVCalendar(path string) bool {
	_, _, ok := parseCalDAVCalendarPath(path)
	return ok
}

func parseCalDAVCalendarPath(path string) (adultID int64, rest string, ok bool) {
	path = normalizeCalDAVHref(path)
	if !strings.HasPrefix(path, calDAVHome) {
		return 0, "", false
	}
	rest = strings.TrimPrefix(path, calDAVHome)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return 0, "", false
	}
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	if len(parts) == 1 {
		return id, "", true
	}
	return id, parts[1], true
}

func calDAVEventHref(adultID int64, uid string) string {
	return fmt.Sprintf("%s%d/%s.ics", calDAVHome, adultID, pathEscapeCalDAV(sanitizeCalDAVUID(uid)))
}

func sanitizeCalDAVUID(uid string) string {
	uid = strings.TrimSpace(uid)
	uid = strings.ReplaceAll(uid, "/", "_")
	return uid
}

func pathEscapeCalDAV(uid string) string {
	// PathEscape leaves @ alone in older Go; Encode path segments so phones
	// that percent-encode and ones that do not both round-trip.
	var b strings.Builder
	for _, r := range uid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '.', r == '_', r == '~':
			b.WriteRune(r)
		default:
			for _, p := range []byte(string(r)) {
				fmt.Fprintf(&b, "%%%02X", p)
			}
		}
	}
	return b.String()
}

func pathUnescapeCalDAV(seg string) string {
	out, err := url.PathUnescape(seg)
	if err != nil {
		return seg
	}
	return out
}

// normalizeCalDAVHref turns an absolute or relative href into a path under /dav/.
func normalizeCalDAVHref(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if u, err := url.Parse(href); err == nil {
		if u.Path != "" {
			href = u.Path
		}
	}
	if i := strings.Index(href, "://"); i >= 0 {
		if slash := strings.Index(href[i+3:], "/"); slash >= 0 {
			href = href[i+3+slash:]
		}
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return href
}

func parseCalDAVEventPath(path string) (adultID int64, uid string, ok bool) {
	adultID, rest, ok := parseCalDAVCalendarPath(path)
	if !ok || rest == "" {
		return 0, "", false
	}
	if !strings.HasSuffix(strings.ToLower(rest), ".ics") {
		return 0, "", false
	}
	uid = rest[:len(rest)-4]
	uid = pathUnescapeCalDAV(uid)
	if uid == "" {
		return 0, "", false
	}
	return adultID, uid, true
}

func (a *App) calDAVPropfind(w http.ResponseWriter, r *http.Request, path string) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxCalDAVBytes))
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "0"
	}
	props := requestedProps(body)

	var responses []davResponse
	switch {
	case isCalDAVRoot(path):
		responses = append(responses, a.propRoot(path, props))
	case isCalDAVPrincipal(path):
		responses = append(responses, a.propPrincipal(path, props))
	case isCalDAVHome(path):
		responses = append(responses, a.propHome(path, props))
		if depth != "0" {
			adults, err := a.store.Adults(false)
			if err != nil {
				a.serverError(w, err)
				return
			}
			for _, adult := range adults {
				href := fmt.Sprintf("%s%d/", calDAVHome, adult.ID)
				resp, err := a.propCalendar(href, adult, props)
				if err != nil {
					a.serverError(w, err)
					return
				}
				responses = append(responses, resp)
			}
		}
	default:
		adultID, rest, ok := parseCalDAVCalendarPath(path)
		if !ok {
			a.notFound(w)
			return
		}
		adult, err := a.store.Adult(adultID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				a.notFound(w)
				return
			}
			a.serverError(w, err)
			return
		}
		if rest == "" {
			href := fmt.Sprintf("%s%d/", calDAVHome, adult.ID)
			resp, err := a.propCalendar(href, adult, props)
			if err != nil {
				a.serverError(w, err)
				return
			}
			responses = append(responses, resp)
			if depth != "0" {
				masters, err := a.store.AdultEventMasters(adult.ID)
				if err != nil {
					a.serverError(w, err)
					return
				}
				for _, e := range masters {
					responses = append(responses, propEvent(calDAVEventHref(adult.ID, e.UID), e, props))
				}
			}
		} else {
			uid := strings.TrimSuffix(rest, ".ics")
			e, err := a.store.AdultEventByUID(adult.ID, uid)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					a.notFound(w)
					return
				}
				a.serverError(w, err)
				return
			}
			responses = append(responses, propEvent(calDAVEventHref(adult.ID, e.UID), e, props))
		}
	}
	writeMultistatus(w, responses)
}

func (a *App) propRoot(href string, props map[string]bool) davResponse {
	return davResponse{
		Href: href,
		Props: filterProps(props, map[string]string{
			"D:resourcetype":           "<D:collection/>",
			"D:displayname":            "School Nanny",
			"D:current-user-principal": fmt.Sprintf(`<D:href>%s</D:href>`, calDAVPrincipal),
			"C:calendar-home-set":      fmt.Sprintf(`<D:href>%s</D:href>`, calDAVHome),
		}),
	}
}

func (a *App) propPrincipal(href string, props map[string]bool) davResponse {
	return davResponse{
		Href: href,
		Props: filterProps(props, map[string]string{
			"D:resourcetype":      "<D:collection/><D:principal/>",
			"D:displayname":       "School Nanny",
			"C:calendar-home-set": fmt.Sprintf(`<D:href>%s</D:href>`, calDAVHome),
			"D:principal-URL":     fmt.Sprintf(`<D:href>%s</D:href>`, calDAVPrincipal),
		}),
	}
}

func (a *App) propHome(href string, props map[string]bool) davResponse {
	return davResponse{
		Href: href,
		Props: filterProps(props, map[string]string{
			"D:resourcetype": "<D:collection/>",
			"D:displayname":  "Calendars",
		}),
	}
}

func (a *App) propCalendar(href string, adult Adult, props map[string]bool) (davResponse, error) {
	token, err := a.store.AdultCalendarSyncToken(adult.ID)
	if err != nil {
		return davResponse{}, err
	}
	color := strings.TrimSpace(adult.Color)
	if color != "" && !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	vals := map[string]string{
		"D:resourcetype":                     "<D:collection/><C:calendar/>",
		"D:displayname":                      xmlEscape(adult.Name),
		"C:supported-calendar-component-set": `<C:comp name="VEVENT"/>`,
		"CS:getctag":                         fmt.Sprintf("%d", token),
		"D:sync-token":                       calDAVSyncToken(adult.ID, token),
		"C:calendar-description":             xmlEscape(adult.Name + "'s calendar"),
		"D:supported-report-set": `<D:supported-report><D:report><C:calendar-query/></D:report></D:supported-report>` +
			`<D:supported-report><D:report><C:calendar-multiget/></D:report></D:supported-report>` +
			`<D:supported-report><D:report><D:sync-collection/></D:report></D:supported-report>`,
	}
	if color != "" {
		vals["A:calendar-color"] = xmlEscape(color)
	}
	return davResponse{Href: href, Props: filterProps(props, vals)}, nil
}

func propEvent(href string, e AdultEvent, props map[string]bool) davResponse {
	cal := EmitAdultEventsICS("", []AdultEvent{e})
	return davResponse{
		Href: href,
		Props: filterProps(props, map[string]string{
			"D:getetag":        xmlEscape(e.ETag()),
			"D:getcontenttype": "text/calendar; charset=utf-8",
			"D:resourcetype":   "",
			"C:calendar-data":  string(cal),
		}),
	}
}

func calDAVSyncToken(adultID, token int64) string {
	return fmt.Sprintf("https://school-nanny/sync/%d/%d", adultID, token)
}

func parseCalDAVSyncToken(raw string) (adultID, token int64, ok bool) {
	raw = strings.TrimSpace(raw)
	const prefix = "https://school-nanny/sync/"
	if !strings.HasPrefix(raw, prefix) {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(raw, prefix), "/")
	if len(parts) != 2 {
		return 0, 0, false
	}
	adultID, err1 := strconv.ParseInt(parts[0], 10, 64)
	token, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return adultID, token, true
}

func requestedProps(body []byte) map[string]bool {
	out := map[string]bool{}
	if len(body) == 0 || !strings.Contains(strings.ToLower(string(body)), "propname") &&
		!strings.Contains(string(body), "<") {
		return out // empty = all
	}
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(se.Name.Local)
		switch name {
		case "propfind", "prop", "allprop", "propname":
			continue
		default:
			out[name] = true
		}
	}
	return out
}

func filterProps(want map[string]bool, have map[string]string) map[string]string {
	if len(want) == 0 {
		return have
	}
	out := map[string]string{}
	for key, val := range have {
		local := key
		if i := strings.IndexByte(key, ':'); i >= 0 {
			local = key[i+1:]
		}
		if want[strings.ToLower(local)] || want[strings.ToLower(key)] {
			out[key] = val
		}
	}
	return out
}

func ensureCalDAVProps(filtered, fallback map[string]string) map[string]string {
	if len(filtered) > 0 {
		return filtered
	}
	return fallback
}

func (a *App) calDAVReport(w http.ResponseWriter, r *http.Request, path string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCalDAVBytes))
	if err != nil {
		http.Error(w, "could not read body", http.StatusBadRequest)
		return
	}
	adultID, rest, ok := parseCalDAVCalendarPath(path)
	if !ok || rest != "" {
		http.Error(w, "REPORT only on a calendar collection", http.StatusForbidden)
		return
	}
	adult, err := a.store.Adult(adultID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	_ = adult
	lower := strings.ToLower(string(body))
	switch {
	case strings.Contains(lower, "sync-collection"):
		a.calDAVSyncReport(w, adultID, body)
	case strings.Contains(lower, "calendar-multiget"):
		a.calDAVMultiGet(w, adultID, body)
	case strings.Contains(lower, "calendar-query"):
		a.calDAVCalendarQuery(w, adultID, body)
	default:
		http.Error(w, "unsupported REPORT", http.StatusForbidden)
	}
}

func (a *App) calDAVSyncReport(w http.ResponseWriter, adultID int64, body []byte) {
	tokenNow, err := a.store.AdultCalendarSyncToken(adultID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	props := requestedProps(body)
	syncToken := ""
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !strings.EqualFold(se.Name.Local, "sync-token") {
			continue
		}
		var v string
		_ = dec.DecodeElement(&v, &se)
		syncToken = strings.TrimSpace(v)
	}

	masters, err := a.store.AdultEventMasters(adultID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	wantData := len(props) == 0 || props["calendar-data"]
	var responses []davResponse
	for _, e := range masters {
		href := calDAVEventHref(adultID, e.UID)
		vals := map[string]string{
			"D:getetag": xmlEscape(e.ETag()),
		}
		if wantData {
			vals["C:calendar-data"] = string(EmitAdultEventsICS("", []AdultEvent{e}))
		}
		responses = append(responses, davResponse{
			Href:  href,
			Props: ensureCalDAVProps(filterProps(props, vals), vals),
		})
	}
	if syncToken != "" {
		if _, prev, ok := parseCalDAVSyncToken(syncToken); ok {
			_ = prev
			tombs, err := a.store.AdultEventTombstones(adultID, "1970-01-01T00:00:00Z")
			if err != nil {
				a.serverError(w, err)
				return
			}
			for _, uid := range tombs {
				responses = append(responses, davResponse{
					Href:   calDAVEventHref(adultID, uid),
					Status: "HTTP/1.1 404 Not Found",
				})
			}
		}
	}
	writeMultistatusWithToken(w, responses, calDAVSyncToken(adultID, tokenNow))
}

func (a *App) calDAVMultiGet(w http.ResponseWriter, adultID int64, body []byte) {
	hrefs := extractHrefs(body)
	var responses []davResponse
	for _, href := range hrefs {
		_, uid, ok := parseCalDAVEventPath(href)
		if !ok {
			responses = append(responses, davResponse{Href: href, Status: "HTTP/1.1 404 Not Found"})
			continue
		}
		e, err := a.store.AdultEventByUID(adultID, uid)
		if err != nil {
			responses = append(responses, davResponse{Href: href, Status: "HTTP/1.1 404 Not Found"})
			continue
		}
		cal := EmitAdultEventsICS("", []AdultEvent{e})
		responses = append(responses, davResponse{
			Href: href,
			Props: map[string]string{
				"D:getetag":       xmlEscape(e.ETag()),
				"C:calendar-data": string(cal),
			},
		})
	}
	writeMultistatus(w, responses)
}

func (a *App) calDAVCalendarQuery(w http.ResponseWriter, adultID int64, body []byte) {
	masters, err := a.store.AdultEventMasters(adultID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	from, to := "1900-01-01", "2100-12-31"
	if f, t, ok := extractTimeRange(body); ok {
		from, to = f, t
	}
	expanded := ExpandAdultEvents(masters, from, to)
	seen := map[int64]bool{}
	var responses []davResponse
	for _, occ := range expanded {
		if seen[occ.ID] {
			continue
		}
		seen[occ.ID] = true
		var master AdultEvent
		for _, m := range masters {
			if m.ID == occ.ID {
				master = m
				break
			}
		}
		if master.ID == 0 {
			master = occ
		}
		href := calDAVEventHref(adultID, master.UID)
		cal := EmitAdultEventsICS("", []AdultEvent{master})
		responses = append(responses, davResponse{
			Href: href,
			Props: map[string]string{
				"D:getetag":       xmlEscape(master.ETag()),
				"C:calendar-data": string(cal),
			},
		})
	}
	writeMultistatus(w, responses)
}

func extractHrefs(body []byte) []string {
	var out []string
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !strings.EqualFold(se.Name.Local, "href") {
			continue
		}
		var v string
		if err := dec.DecodeElement(&v, &se); err == nil {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func extractTimeRange(body []byte) (from, to string, ok bool) {
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, okTok := tok.(xml.StartElement)
		if !okTok || !strings.EqualFold(se.Name.Local, "time-range") {
			continue
		}
		var start, end string
		for _, attr := range se.Attr {
			switch strings.ToLower(attr.Name.Local) {
			case "start":
				start = attr.Value
			case "end":
				end = attr.Value
			}
		}
		if len(start) >= 8 {
			from = start[:4] + "-" + start[4:6] + "-" + start[6:8]
		}
		if len(end) >= 8 {
			to = end[:4] + "-" + end[4:6] + "-" + end[6:8]
		}
		if from != "" && to != "" {
			return from, to, true
		}
	}
	return "", "", false
}

func (a *App) calDAVGet(w http.ResponseWriter, r *http.Request, path string) {
	adultID, uid, ok := parseCalDAVEventPath(path)
	if !ok {
		a.notFound(w)
		return
	}
	e, err := a.store.AdultEventByUID(adultID, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	body := EmitAdultEventsICS("", []AdultEvent{e})
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("ETag", e.ETag())
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(body)
}

func (a *App) calDAVPut(w http.ResponseWriter, r *http.Request, path string) {
	adultID, uid, ok := parseCalDAVEventPath(path)
	if !ok {
		http.Error(w, "PUT only on an event resource", http.StatusForbidden)
		return
	}
	if _, err := a.store.Adult(adultID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxCalDAVBytes))
	if err != nil {
		http.Error(w, "could not read body", http.StatusBadRequest)
		return
	}
	ev, err := ParseICSEventStrict(raw, requestLocation(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	if ev.UID == "" {
		ev.UID = uid
	}
	if sanitizeCalDAVUID(ev.UID) != sanitizeCalDAVUID(uid) && ev.UID != uid {
		// Prefer the href uid so the resource path stays stable.
		ev.UID = uid
	}
	ev.AdultID = adultID
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	saved, created, err := a.store.UpsertAdultEventByUID(ev, ifMatch)
	if errors.Is(err, errCalDAVPrecondition) {
		http.Error(w, "precondition failed", http.StatusPreconditionFailed)
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	w.Header().Set("ETag", saved.ETag())
	if created {
		w.WriteHeader(http.StatusCreated)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) calDAVDelete(w http.ResponseWriter, r *http.Request, path string) {
	adultID, uid, ok := parseCalDAVEventPath(path)
	if !ok {
		http.Error(w, "DELETE only on an event resource", http.StatusForbidden)
		return
	}
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	err := a.store.DeleteAdultEventByUID(adultID, uid, ifMatch)
	if errors.Is(err, errCalDAVPrecondition) {
		http.Error(w, "precondition failed", http.StatusPreconditionFailed)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		a.notFound(w)
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type davResponse struct {
	Href   string
	Props  map[string]string
	Status string
}

func writeMultistatus(w http.ResponseWriter, responses []davResponse) {
	writeMultistatusWithToken(w, responses, "")
}

func writeMultistatusWithToken(w http.ResponseWriter, responses []davResponse, syncToken string) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav" xmlns:CS="http://calendarserver.org/ns/" xmlns:A="http://apple.com/ns/ical/">`)
	for _, resp := range responses {
		b.WriteString("<D:response>")
		b.WriteString("<D:href>" + xmlEscape(resp.Href) + "</D:href>")
		if resp.Status != "" {
			b.WriteString("<D:status>" + xmlEscape(resp.Status) + "</D:status>")
		} else {
			b.WriteString("<D:propstat><D:prop>")
			for name, val := range resp.Props {
				local := name
				if i := strings.IndexByte(name, ':'); i >= 0 {
					local = name[i+1:]
				}
				if val == "" && strings.EqualFold(local, "resourcetype") {
					b.WriteString("<" + name + "/>")
					continue
				}
				if strings.EqualFold(local, "calendar-data") {
					// ICS as text; escape so a DESCRIPTION with & does not break XML.
					b.WriteString("<" + name + ">" + xmlEscape(val) + "</" + name + ">")
					continue
				}
				if strings.Contains(val, "<") {
					b.WriteString("<" + name + ">" + val + "</" + name + ">")
				} else {
					b.WriteString("<" + name + ">" + val + "</" + name + ">")
				}
			}
			b.WriteString("</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>")
		}
		b.WriteString("</D:response>")
	}
	if syncToken != "" {
		b.WriteString("<D:sync-token>" + xmlEscape(syncToken) + "</D:sync-token>")
	}
	b.WriteString("</D:multistatus>")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(b.String()))
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
