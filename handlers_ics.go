package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (a *App) handleAdultCalendarICS(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	events, err := a.store.AdultEventMasters(adult.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	body := EmitAdultEventsICS(adult.Name+"'s calendar", events)
	name := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, adult.Name)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-calendar.ics"`, name))
	_, _ = w.Write(body)
}

func (a *App) handleAdultCalendarImport(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+64*1024)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		http.Error(w, "Could not read that calendar file.", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, "Choose an .ics calendar file.", http.StatusBadRequest)
		return
	}
	src, err := files[0].Open()
	if err != nil {
		http.Error(w, "Could not read that file.", http.StatusBadRequest)
		return
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, int64(maxImportBytes)+1))
	if err != nil {
		http.Error(w, "Could not read that file.", http.StatusBadRequest)
		return
	}
	if len(raw) > maxImportBytes {
		http.Error(w, "That calendar file is too large.", http.StatusBadRequest)
		return
	}
	loc := requestLocation(r)
	events, err := ParseICSEvents(raw, loc, nowIn(loc))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, err := a.store.AdultEventMasters(adult.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	fresh := unseenAdultEvents(existing, events)
	for i := range fresh {
		fresh[i].AdultID = adult.ID
		if fresh[i].UID == "" {
			fresh[i].UID = newEventUID()
		}
		if !fresh[i].AllDay && fresh[i].StartAt == "" {
			fresh[i].AllDay = true
		}
	}
	if len(fresh) > 0 {
		if err := a.store.CreateAdultEvents(fresh); err != nil {
			a.serverError(w, err)
			return
		}
	}
	q := url.Values{}
	if len(fresh) == 0 {
		q.Set("already", "1")
	} else {
		q.Set("imported", strconv.Itoa(len(fresh)))
		month, from, to := focusImportedEvent(fresh, todayIn(loc))
		if from != "" {
			if month != "" {
				q.Set("month", month)
			}
			q.Set("from", from)
			q.Set("to", to)
		}
	}
	a.redirect(w, r, fmt.Sprintf("/adults/%d?%s", adult.ID, q.Encode()))
}

// unseenAdultEvents drops rows that are already on her calendar, so importing
// the same file again adds the new anniversaries without cloning the rest.
func unseenAdultEvents(existing, incoming []AdultEvent) []AdultEvent {
	seen := make(map[string]bool, len(existing))
	for _, e := range existing {
		seen[adultEventKey(e)] = true
	}
	var out []AdultEvent
	for _, e := range incoming {
		key := adultEventKey(e)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

func adultEventKey(e AdultEvent) string {
	uid := strings.TrimSpace(e.UID)
	if uid != "" {
		return "uid:" + uid
	}
	return strings.TrimSpace(e.Title) + "\x00" + e.StartsOn + "\x00" + e.EndsOn + "\x00" + e.RRule
}

// focusImportedEvent picks the day the calendar should open on: today when
// something imported covers it, otherwise the next date, otherwise the most
// recent one. The month is empty when that day is already in this month.
func focusImportedEvent(events []AdultEvent, today string) (month, from, to string) {
	windowEnd := addDays(today, 366*recurHorizonYears)
	expanded := ExpandAdultEvents(events, today, windowEnd)
	if len(expanded) == 0 {
		expanded = events
	}
	var next, recent *AdultEvent
	for i := range expanded {
		e := &expanded[i]
		if e.Covers(today) {
			return "", today, today
		}
		if e.StartsOn >= today && (next == nil || e.StartsOn < next.StartsOn) {
			next = e
		} else if e.StartsOn < today && (recent == nil || e.StartsOn > recent.StartsOn) {
			recent = e
		}
	}
	pick := next
	if pick == nil {
		pick = recent
	}
	if pick == nil {
		return "", "", ""
	}
	if monthFirst(pick.StartsOn) != monthFirst(today) {
		month = monthFirst(pick.StartsOn)
	}
	return month, pick.StartsOn, pick.StartsOn
}
