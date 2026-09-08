package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// lookupAdult resolves the adult in the URL, answering 404 rather than an
// error when there is no such person.
func (a *App) lookupAdult(w http.ResponseWriter, r *http.Request) (Adult, bool) {
	adult, err := a.store.Adult(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return Adult{}, false
		}
		a.serverError(w, err)
		return Adult{}, false
	}
	return adult, true
}

// handleAdult is her profile: who she is, what she has pinned, the month at a
// glance, this week, and what she has written down lately.
func (a *App) handleAdult(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("adults")
	if err != nil {
		a.serverError(w, err)
		return
	}

	start := weekStart(time.Now()).Format(dateLayout)
	end := addDays(start, 6)

	cards, err := a.store.AdultCards(adult.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	week, err := a.store.AdultLessonsBetween(start, end, adult.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	notes, err := a.store.AdultNotes(adult.ID, 20)
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.populateAdultCalendar(data, adult, r.URL.Query()); err != nil {
		a.serverError(w, err)
		return
	}

	data["Adult"] = adult
	data["Cards"] = cards
	data["Week"] = week
	data["Notes"] = notes
	data["Subjects"] = subjects
	data["WeekStart"] = start
	data["WeekEnd"] = end
	a.render(w, "adult", data)
}

// Calendar ---------------------------------------------------------------

// handleAdultCalendar redraws the calendar on its own. Choosing a day used to
// mean reloading her whole profile, which threw the page back to the top and
// left her scrolling down to the month again after every click.
func (a *App) handleAdultCalendar(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	a.renderAdultCalendar(w, adult, r.URL.Query())
}

func (a *App) renderAdultCalendar(w http.ResponseWriter, adult Adult, query url.Values) {
	data := map[string]any{"Adult": adult}
	if err := a.populateAdultCalendar(data, adult, query); err != nil {
		a.serverError(w, err)
		return
	}
	a.renderPartial(w, "adult_calendar", data)
}

// AdultCalendarDay is one cell of her month: the holidays that always fall
// there, and whatever she has written on it herself.
type AdultCalendarDay struct {
	Date     string
	InMonth  bool
	Selected bool
	Holidays []Holiday
	Events   []AdultEvent
}

// populateAdultCalendar builds the month grid and whatever the selected run of
// days holds. Selection is a pair of inclusive dates because a stretch she
// dragged across the grid - a trip, a week of appointments - is one event, not
// several, and a single day is simply both ends landing together.
func (a *App) populateAdultCalendar(data map[string]any, adult Adult, query url.Values) error {
	month := parseMonthQuery(query.Get("month"))

	// The grid pads out to whole weeks, so it reaches a little into the months
	// on either side. Everything below works in those outer dates.
	gridFrom := weekStart(parseDate(monthFirst(month))).Format(dateLayout)
	gridTo := addDays(gridFrom, len(monthGridDates(month))-1)

	from, to := selectedRange(query.Get("from"), query.Get("to"), month, gridFrom, gridTo)

	events, err := a.store.AdultEventsOverlapping(adult.ID, gridFrom, gridTo)
	if err != nil {
		return err
	}
	holidays := holidaysBetween(gridFrom, gridTo)

	var weeks [][]AdultCalendarDay
	var week []AdultCalendarDay
	for _, date := range monthGridDates(month) {
		day := AdultCalendarDay{
			Date:     date,
			InMonth:  date >= monthFirst(month) && date <= monthLast(month),
			Selected: date >= from && date <= to,
		}
		for _, h := range holidays {
			if h.Date == date {
				day.Holidays = append(day.Holidays, h)
			}
		}
		// A run of days marks every cell it covers, so a week away reads as a
		// week away rather than a single dot on the day it started.
		for _, e := range events {
			if e.Covers(date) {
				day.Events = append(day.Events, e)
			}
		}
		week = append(week, day)
		if len(week) == 7 {
			weeks = append(weeks, week)
			week = nil
		}
	}
	if len(week) > 0 {
		weeks = append(weeks, week)
	}

	var selectedEvents []AdultEvent
	for _, e := range events {
		if e.StartsOn <= to && e.EndsOn >= from {
			selectedEvents = append(selectedEvents, e)
		}
	}
	var selectedHolidays []Holiday
	for _, h := range holidays {
		if h.Date >= from && h.Date <= to {
			selectedHolidays = append(selectedHolidays, h)
		}
	}

	data["Month"] = month
	data["MonthLabel"] = formatDate(month, "January 2006")
	data["PrevMonth"] = addMonths(month, -1)
	data["NextMonth"] = addMonths(month, 1)
	data["ThisMonth"] = monthFirst(today())
	data["Weeks"] = weeks
	data["SelectedFrom"] = from
	data["SelectedTo"] = to
	data["SelectionLabel"] = rangeLabel(from, to)
	data["DayEvents"] = selectedEvents
	data["DayHolidays"] = selectedHolidays
	return nil
}

// monthGridDates lists the days a month's grid shows, padded out to whole
// Monday-to-Sunday weeks the way the attendance calendar does.
func monthGridDates(month string) []string {
	first := parseDate(monthFirst(month))
	last := parseDate(monthLast(month))
	start := weekStart(first)
	end := last.AddDate(0, 0, (7-int(last.Weekday()))%7)

	var dates []string
	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		dates = append(dates, t.Format(dateLayout))
	}
	return dates
}

// selectedRange settles on the run of days the page is talking about. Dragging
// backwards across the grid is the same selection as dragging forwards, and
// anything outside the visible weeks is ignored rather than obeyed, so a
// hand-edited URL cannot select days that are not on screen.
func selectedRange(rawFrom, rawTo, month, gridFrom, gridTo string) (string, string) {
	from := dateInRange(rawFrom, gridFrom, gridTo)
	to := dateInRange(rawTo, gridFrom, gridTo)
	switch {
	case from == "" && to == "":
		// Nothing asked for: start on today when she is looking at this month,
		// and at the first of the month when she has paged away from it.
		if today() >= monthFirst(month) && today() <= monthLast(month) {
			return today(), today()
		}
		return monthFirst(month), monthFirst(month)
	case from == "":
		from = to
	case to == "":
		to = from
	}
	if to < from {
		from, to = to, from
	}
	return from, to
}

func dateInRange(raw, from, to string) string {
	raw = strings.TrimSpace(raw)
	if _, err := time.Parse(dateLayout, raw); err != nil {
		return ""
	}
	if raw < from || raw > to {
		return ""
	}
	return raw
}

func rangeLabel(from, to string) string {
	if from == to {
		return prettyDate(from)
	}
	return prettyDate(from) + " - " + prettyDate(to)
}

// handleCreateAdultEvent writes something onto her calendar. The end date is
// optional: leaving it off means a single day.
func (a *App) handleCreateAdultEvent(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "An event needs a title.", http.StatusBadRequest)
		return
	}
	starts := formDate(r, "starts_on")
	ends := formDateOrEmpty(r, "ends_on")
	if ends == "" {
		ends = starts
	}
	if ends < starts {
		http.Error(w, "An event cannot end before it starts.", http.StatusBadRequest)
		return
	}

	_, err := a.store.CreateAdultEvent(AdultEvent{
		AdultID:  adult.ID,
		StartsOn: starts,
		EndsOn:   ends,
		Title:    title,
		Body:     strings.TrimSpace(r.FormValue("body")),
	})
	if err != nil {
		a.serverError(w, err)
		return
	}
	if a.wantsCalendar(r) {
		a.renderAdultCalendar(w, adult, r.Form)
		return
	}
	a.redirect(w, r, a.backToAdult(r, adult))
}

func (a *App) handleDeleteAdultEvent(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	if err := a.store.DeleteAdultEvent(adult.ID, pathID(r, "eventID")); err != nil {
		a.serverError(w, err)
		return
	}
	if a.wantsCalendar(r) {
		a.renderAdultCalendar(w, adult, r.Form)
		return
	}
	a.redirect(w, r, a.backToAdult(r, adult))
}

// wantsCalendar reports a request made by the calendar itself, which wants the
// month back rather than a fresh page.
func (a *App) wantsCalendar(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.FormValue("view") == "calendar"
}

// handleAdultSchedule is her own week grid. It reuses the planner's day
// partial, which is why the days come back in the same shape.
func (a *App) handleAdultSchedule(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("adults")
	if err != nil {
		a.serverError(w, err)
		return
	}

	start := weekStart(parseDate(r.URL.Query().Get("week"))).Format(dateLayout)
	end := addDays(start, 6)

	lessons, err := a.store.AdultLessonsBetween(start, end, adult.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	byDay := map[string][]Lesson{}
	for _, l := range lessons {
		byDay[l.ScheduledOn] = append(byDay[l.ScheduledOn], l)
	}
	days := make([]PlannerDay, 0, 7)
	for i := 0; i < 7; i++ {
		date := addDays(start, i)
		days = append(days, PlannerDay{Date: date, Lessons: byDay[date]})
	}

	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Adult"] = adult
	data["Days"] = days
	data["Subjects"] = subjects
	data["WeekStart"] = start
	data["WeekEnd"] = end
	data["PrevWeek"] = addDays(start, -7)
	data["NextWeek"] = addDays(start, 7)
	data["ThisWeek"] = weekStart(time.Now()).Format(dateLayout)
	a.render(w, "adult_schedule", data)
}

// renderAdultDay re-renders one day of her week after it changed, the same way
// the planner does for the children.
func (a *App) renderAdultDay(w http.ResponseWriter, adult Adult, dates ...string) {
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	seen := map[string]bool{}
	for i, date := range dates {
		if date == "" || seen[date] {
			continue
		}
		seen[date] = true

		lessons, err := a.store.AdultLessonsBetween(date, date, adult.ID)
		if err != nil {
			a.serverError(w, err)
			return
		}
		a.renderPartial(w, "adult_day", map[string]any{
			"Day":      PlannerDay{Date: date, Lessons: lessons},
			"Adult":    adult,
			"Subjects": subjects,
			"OOB":      i > 0,
		})
	}
}

// handleCreateAdultLesson books something on her calendar. It is a lesson row
// like any other, which is what lets her week reuse the planner's machinery.
func (a *App) handleCreateAdultLesson(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	item := Lesson{
		AdultID:     adult.ID,
		SubjectID:   formID(r, "subject_id"),
		ScheduledOn: formDate(r, "scheduled_on"),
		Title:       strings.TrimSpace(r.FormValue("title")),
		Minutes:     formInt(r, "minutes"),
		Notes:       strings.TrimSpace(r.FormValue("notes")),
		Status:      StatusPlanned,
	}
	if r.FormValue("status") == StatusDone {
		item.Status = StatusDone
	}
	if item.SubjectID == 0 || item.Title == "" {
		http.Error(w, "This needs a subject and a title.", http.StatusBadRequest)
		return
	}
	if _, err := a.store.CreateLesson(item); err != nil {
		a.serverError(w, err)
		return
	}

	if r.Header.Get("HX-Request") == "true" && r.FormValue("view") == "adult" {
		a.renderAdultDay(w, adult, item.ScheduledOn)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/adults/"+r.PathValue("id")+"/schedule"))
}

// Pinboard cards ---------------------------------------------------------

func (a *App) handleCreateAdultCard(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "A card needs a title.", http.StatusBadRequest)
		return
	}
	_, err := a.store.CreateAdultCard(AdultCard{
		AdultID: adult.ID,
		Title:   title,
		Body:    strings.TrimSpace(r.FormValue("body")),
		Pinned:  r.FormValue("pinned") == "on",
	})
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, a.backToAdult(r, adult))
}

func (a *App) handleUpdateAdultCard(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "A card needs a title.", http.StatusBadRequest)
		return
	}
	err := a.store.UpdateAdultCard(pathID(r, "cardID"), title,
		strings.TrimSpace(r.FormValue("body")), r.FormValue("pinned") == "on")
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, a.backToAdult(r, adult))
}

func (a *App) handleDeleteAdultCard(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteAdultCard(pathID(r, "cardID")); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, a.backToAdult(r, adult))
}

func (a *App) handleMoveAdultCard(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	if err := a.store.MoveAdultCard(pathID(r, "cardID"), r.FormValue("direction") == "up"); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, a.backToAdult(r, adult))
}

func (a *App) backToAdult(r *http.Request, adult Adult) string {
	return safeRedirect(r.FormValue("back"), adult.URL())
}

// Settings and photo -----------------------------------------------------

func (a *App) handleSaveAdult(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "She needs a name.", http.StatusBadRequest)
		return
	}
	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		role = "Mom"
	}
	color := strings.TrimSpace(r.FormValue("color"))
	if color == "" {
		color = "#8d78e0"
	}
	if err := a.store.UpdateAdult(formID(r, "id"), name, role, color,
		r.FormValue("archived") == "on"); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=adult")
}

func (a *App) handleAdultAvatarUpload(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	stored, err := a.saveAvatarUpload(w, r)
	switch {
	case errors.Is(err, errNoPhotoChosen),
		errors.Is(err, errNotAnImage),
		errors.Is(err, errPhotoTooLarge),
		errors.Is(err, errPhotoIsHEIC):
		http.Error(w, err.Error()+".", http.StatusBadRequest)
		return
	case err != nil:
		a.serverError(w, err)
		return
	}

	previous, err := a.store.SetAdultAvatar(adult.ID, stored)
	if err != nil {
		a.removeUpload(stored)
		a.serverError(w, err)
		return
	}
	a.removeUpload(previous)
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/settings?saved=photo"))
}

func (a *App) handleAdultAvatarDelete(w http.ResponseWriter, r *http.Request) {
	previous, err := a.store.SetAdultAvatar(pathID(r, "id"), "")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	a.removeUpload(previous)
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/settings?saved=photo-removed"))
}

func (a *App) handleAdultAvatarImage(w http.ResponseWriter, r *http.Request) {
	adult, ok := a.lookupAdult(w, r)
	if !ok {
		return
	}
	a.serveAvatar(w, r, adult.AvatarPath)
}
