package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

// handleCreateLesson covers both halves of the workflow: scheduling something
// for later and recording something that already happened today.
func (a *App) handleCreateLesson(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	lesson := Lesson{
		KidID:       formID(r, "kid_id"),
		SubjectID:   formID(r, "subject_id"),
		ScheduledOn: formDate(r, "scheduled_on"),
		Title:       strings.TrimSpace(r.FormValue("title")),
		Minutes:     formInt(r, "minutes"),
		Notes:       strings.TrimSpace(r.FormValue("notes")),
		Status:      StatusPlanned,
	}
	if r.FormValue("status") == StatusDone {
		lesson.Status = StatusDone
	}
	if lesson.KidID == 0 || lesson.SubjectID == 0 || lesson.Title == "" {
		http.Error(w, "A lesson needs a child, a subject, and a title.", http.StatusBadRequest)
		return
	}

	if _, err := a.store.CreateLesson(lesson); err != nil {
		a.serverError(w, err)
		return
	}

	// The planner swaps just the day that changed; everywhere else reloads.
	if r.Header.Get("HX-Request") == "true" && r.FormValue("view") == "planner" {
		a.renderPlannerDay(w, lesson.ScheduledOn, formID(r, "kid_filter"))
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
}

func (a *App) handleUpdateLesson(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	lesson := Lesson{
		KidID:       formID(r, "kid_id"),
		SubjectID:   formID(r, "subject_id"),
		ScheduledOn: formDate(r, "scheduled_on"),
		Title:       strings.TrimSpace(r.FormValue("title")),
		Minutes:     formInt(r, "minutes"),
		Notes:       strings.TrimSpace(r.FormValue("notes")),
	}
	if lesson.KidID == 0 || lesson.SubjectID == 0 || lesson.Title == "" {
		http.Error(w, "A lesson needs a child, a subject, and a title.", http.StatusBadRequest)
		return
	}
	if err := a.store.UpdateLesson(id, lesson); err != nil {
		a.serverError(w, err)
		return
	}
	if status := r.FormValue("status"); status != "" {
		if err := a.store.SetLessonStatus(id, normalizeStatus(status)); err != nil {
			a.serverError(w, err)
			return
		}
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/lessons/"+r.PathValue("id")))
}

func (a *App) handleLessonStatus(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	if err := a.store.SetLessonStatus(id, normalizeStatus(r.FormValue("status"))); err != nil {
		a.serverError(w, err)
		return
	}

	if r.Header.Get("HX-Request") != "true" {
		a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
		return
	}

	lesson, err := a.store.Lesson(id)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.renderPartial(w, "lesson_row", map[string]any{
		"Lesson":      lesson,
		"ShowKid":     r.FormValue("show_kid") == "true",
		"ShowSubject": r.FormValue("show_subject") != "false",
		"Back":        safeRedirect(r.FormValue("back"), "/"),
	})
}

func (a *App) handleRescheduleLesson(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	if err := a.store.RescheduleLesson(id, formDate(r, "scheduled_on")); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/planner"))
}

func (a *App) handleDeleteLesson(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	lesson, err := a.store.Lesson(id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, err)
		return
	}
	if err := a.deleteLessonFiles(id); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.DeleteLesson(id); err != nil {
		a.serverError(w, err)
		return
	}

	if r.Header.Get("HX-Request") == "true" && r.FormValue("view") == "planner" {
		a.renderPlannerDay(w, lesson.ScheduledOn, formID(r, "kid_filter"))
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/planner"))
}

// renderPlannerDay re-renders one day of the week grid after it changed.
func (a *App) renderPlannerDay(w http.ResponseWriter, date string, kidFilter int64) {
	lessons, err := a.store.LessonsBetween(date, date, kidFilter)
	if err != nil {
		a.serverError(w, err)
		return
	}
	kids, err := a.store.Kids(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.renderPartial(w, "planner_day", map[string]any{
		"Day":       PlannerDay{Date: date, Lessons: lessons},
		"Kids":      kids,
		"Subjects":  subjects,
		"KidFilter": kidFilter,
		"Today":     today(),
	})
}

func (a *App) handleCreateAssessment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	test := Assessment{
		KidID:     formID(r, "kid_id"),
		SubjectID: formID(r, "subject_id"),
		LessonID:  formID(r, "lesson_id"),
		GivenOn:   formDate(r, "given_on"),
		Name:      strings.TrimSpace(r.FormValue("name")),
		Score:     formFloat(r, "score"),
		MaxScore:  formFloat(r, "max_score"),
		Letter:    strings.TrimSpace(r.FormValue("letter")),
		Notes:     strings.TrimSpace(r.FormValue("notes")),
	}
	if test.KidID == 0 || test.SubjectID == 0 || test.Name == "" {
		http.Error(w, "A test needs a child, a subject, and a name.", http.StatusBadRequest)
		return
	}
	if _, err := a.store.CreateAssessment(test); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/kids/"+r.FormValue("kid_id")+"/tests"))
}

func (a *App) handleDeleteAssessment(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if err := a.deleteAssessmentFiles(id); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.DeleteAssessment(id); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
}

func (a *App) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	note := Note{
		KidID:     formID(r, "kid_id"),
		SubjectID: formID(r, "subject_id"),
		NotedOn:   formDate(r, "noted_on"),
		Body:      strings.TrimSpace(r.FormValue("body")),
	}
	if note.KidID == 0 || note.Body == "" {
		http.Error(w, "A note needs a child and something to say.", http.StatusBadRequest)
		return
	}
	if _, err := a.store.CreateNote(note); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
}

func (a *App) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteNote(pathID(r, "id")); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
}

func normalizeStatus(raw string) string {
	switch raw {
	case StatusDone:
		return StatusDone
	case StatusSkipped:
		return StatusSkipped
	default:
		return StatusPlanned
	}
}
