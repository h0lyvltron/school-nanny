package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (a *App) handleAssignment(w http.ResponseWriter, r *http.Request) {
	asg, ok := a.lookupAssignment(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("")
	if err != nil {
		a.serverError(w, err)
		return
	}
	lessons, err := a.store.LessonsForAssignment(asg.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}

	var planned []Lesson
	progress := Progress{}
	for _, l := range lessons {
		switch l.Status {
		case StatusPlanned:
			progress.Planned++
			planned = append(planned, l)
		case StatusDone:
			progress.Done++
			progress.Minutes += l.Minutes
		case StatusSkipped:
			progress.Skipped++
		}
	}

	data["Assignment"] = asg
	data["Weekdays"] = weekdayChoices(asg.Weekdays)
	data["Lessons"] = planned
	data["Progress"] = progress
	data["ResumeOn"] = today()
	a.render(w, "assignment", data)
}

func (a *App) handleUpdateAssignment(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	existing, err := a.store.Assignment(id)
	if err != nil {
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

	name := strings.TrimSpace(r.FormValue("name"))
	weekdays := formWeekdays(r.Form["weekday"])
	if name == "" {
		http.Error(w, "A scheduled plan needs a name.", http.StatusBadRequest)
		return
	}
	if weekdays == "" {
		http.Error(w, errNoWeekdays.Error(), http.StatusBadRequest)
		return
	}

	scheduleChanged := existing.Weekdays != weekdays
	if err := a.store.UpdateAssignment(id, name, weekdays); err != nil {
		a.serverError(w, err)
		return
	}
	if scheduleChanged {
		existing.Name = name
		existing.Weekdays = weekdays
		if err := a.store.RelayoutPlannedFrom(existing, today(), 0); err != nil {
			a.serverError(w, err)
			return
		}
	}
	a.redirect(w, r, "/assignments/"+r.PathValue("id"))
}

func (a *App) handlePauseAssignment(w http.ResponseWriter, r *http.Request) {
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
	resume := formDate(r, "resume_on")
	if err := a.store.PauseAssignmentUntil(id, resume); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/assignments/"+r.PathValue("id")))
}

func (a *App) handleStopAssignment(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	from := today()
	if err := a.deletePlannedAssignmentFilesByDate(id, from); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.DeletePlannedAssignmentFrom(id, from); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/planner"))
}

func (a *App) handlePushLesson(w http.ResponseWriter, r *http.Request) {
	lesson, err := a.store.Lesson(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	if err := a.store.PushAssignmentLesson(lesson); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), plannerURL(
		weekStart(parseDate(lesson.ScheduledOn)).Format(dateLayout), lesson.KidID)))
}

func (a *App) lookupAssignment(w http.ResponseWriter, r *http.Request) (PlanAssignment, bool) {
	asg, err := a.store.Assignment(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return PlanAssignment{}, false
		}
		a.serverError(w, err)
		return PlanAssignment{}, false
	}
	return asg, true
}

func (a *App) deletePlannedAssignmentFiles(assignmentID int64, fromSequence int) error {
	ids, err := a.store.PlannedAssignmentIDsFrom(assignmentID, fromSequence)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := a.deleteLessonFiles(id); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) deletePlannedAssignmentFilesByDate(assignmentID int64, from string) error {
	lessons, err := a.store.LessonsForAssignment(assignmentID)
	if err != nil {
		return err
	}
	for _, l := range lessons {
		if l.Status != StatusPlanned || l.ScheduledOn < from {
			continue
		}
		if err := a.deleteLessonFiles(l.ID); err != nil {
			return err
		}
	}
	return nil
}
