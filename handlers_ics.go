package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (a *App) handleAdultCalendarICS(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	events, err := a.store.AdultEventsOverlapping(adult.ID, "1900-01-01", "2100-12-31")
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
	events, err := ParseICSEvents(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for i := range events {
		events[i].AdultID = adult.ID
	}
	if err := a.store.CreateAdultEvents(events); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, fmt.Sprintf("/adults/%d?imported=%d", adult.ID, len(events)))
}
