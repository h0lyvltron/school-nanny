package main

import (
	"database/sql"
	"errors"
	"net/http"
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

// handleAdult is her profile: who she is, what she has pinned, this week, and
// what she has written down lately.
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

	data["Adult"] = adult
	data["Cards"] = cards
	data["Week"] = week
	data["Notes"] = notes
	data["Subjects"] = subjects
	data["WeekStart"] = start
	data["WeekEnd"] = end
	a.render(w, "adult", data)
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
	case errors.Is(err, errNoPhotoChosen), errors.Is(err, errNotAnImage):
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
