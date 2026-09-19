package main

import (
	"database/sql"
	"errors"
	"net/http"
)

func (a *App) handleDoubleUpLesson(w http.ResponseWriter, r *http.Request) {
	a.shiftAssignmentLesson(w, r, func(lesson Lesson) error {
		return a.store.DoubleUpLesson(lesson, requestToday(r))
	})
}

func (a *App) handleShiftLesson(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	scope := r.FormValue("scope")
	if scope == "" {
		scope = "later"
	}
	a.shiftAssignmentLesson(w, r, func(lesson Lesson) error {
		switch scope {
		case "this":
			return a.store.ShiftLessonForward(lesson)
		case "later":
			if lesson.AssignmentID == 0 {
				return a.store.ShiftLessonForward(lesson)
			}
			return a.store.ShiftAssignmentForward(lesson)
		default:
			return errors.New("unknown shift scope")
		}
	})
}

func (a *App) handleVacationAssignment(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	id := pathID(r, "id")
	if _, err := a.store.Assignment(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	from := formDate(r, "from")
	to := formDate(r, "to")
	if err := a.store.VacationShiftAssignment(id, from, to); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/assignments/"+r.PathValue("id")))
}
