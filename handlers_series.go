package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (a *App) handleSeries(w http.ResponseWriter, r *http.Request) {
	series, ok := a.lookupSeries(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("")
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	lessons, err := a.store.LessonsForSeries(series.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Series"] = series
	data["Subjects"] = subjects
	data["Kids"] = data["NavKids"]
	data["Weekdays"] = weekdayChoices(series.Weekdays)
	data["Lessons"] = lessons
	a.render(w, "series", data)
}

func (a *App) handleUpdateSeries(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	existing, err := a.store.Series(id)
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

	series := LessonSeries{
		KidID:           formID(r, "kid_id"),
		SubjectID:       formID(r, "subject_id"),
		Title:           strings.TrimSpace(r.FormValue("title")),
		Minutes:         formInt(r, "minutes"),
		Notes:           strings.TrimSpace(r.FormValue("notes")),
		Weekdays:        formWeekdays(r.Form["weekday"]),
		StartsOn:        formDate(r, "starts_on"),
		EndsOn:          formDateOrEmpty(r, "ends_on"),
		OccurrenceCount: formInt(r, "occurrence_count"),
	}
	if series.KidID == 0 || series.SubjectID == 0 || series.Title == "" {
		http.Error(w, "A series needs a child, a subject, and a title.", http.StatusBadRequest)
		return
	}
	if series.Weekdays == "" {
		http.Error(w, errNoWeekdays.Error(), http.StatusBadRequest)
		return
	}
	if series.EndsOn == "" && series.OccurrenceCount <= 0 {
		http.Error(w, errNoSeriesEnd.Error(), http.StatusBadRequest)
		return
	}
	if _, err := occurrenceDates(series.StartsOn, series.EndsOn, series.OccurrenceCount, parseWeekdays(series.Weekdays)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	scheduleChanged := existing.Weekdays != series.Weekdays ||
		existing.StartsOn != series.StartsOn ||
		existing.EndsOn != series.EndsOn ||
		existing.OccurrenceCount != series.OccurrenceCount

	if err := a.store.UpdateSeries(id, series); err != nil {
		a.serverError(w, err)
		return
	}
	series.ID = id

	from := today()
	if err := a.store.UpdateFutureSeriesLessons(id, series, from); err != nil {
		a.serverError(w, err)
		return
	}
	if scheduleChanged {
		if err := a.deletePlannedSeriesFiles(id, from); err != nil {
			a.serverError(w, err)
			return
		}
		if err := a.store.DeletePlannedSeriesFrom(id, from); err != nil {
			a.serverError(w, err)
			return
		}
		if err := a.store.RematerializeSeries(series, from); err != nil {
			a.serverError(w, err)
			return
		}
	}
	a.redirect(w, r, "/series/"+r.PathValue("id"))
}

func (a *App) handleStopSeries(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	from := today()
	if err := a.deletePlannedSeriesFiles(id, from); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.DeletePlannedSeriesFrom(id, from); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/planner"))
}

func (a *App) handleDeleteSeriesFuture(w http.ResponseWriter, r *http.Request) {
	lesson, err := a.store.Lesson(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	if lesson.SeriesID == 0 {
		http.Error(w, "That lesson is not part of a series.", http.StatusBadRequest)
		return
	}
	from := lesson.ScheduledOn
	if err := a.deletePlannedSeriesFiles(lesson.SeriesID, from); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.DeletePlannedSeriesFrom(lesson.SeriesID, from); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/planner"))
}

func (a *App) lookupSeries(w http.ResponseWriter, r *http.Request) (LessonSeries, bool) {
	series, err := a.store.Series(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return LessonSeries{}, false
		}
		a.serverError(w, err)
		return LessonSeries{}, false
	}
	return series, true
}

func (a *App) deletePlannedSeriesFiles(seriesID int64, from string) error {
	ids, err := a.store.PlannedSeriesIDsFrom(seriesID, from)
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

func seriesFromForm(r *http.Request, lesson Lesson) (LessonSeries, []string, error) {
	sr := LessonSeries{
		KidID:           lesson.KidID,
		SubjectID:       lesson.SubjectID,
		Title:           lesson.Title,
		Minutes:         lesson.Minutes,
		Notes:           lesson.Notes,
		Weekdays:        formWeekdays(r.Form["weekday"]),
		StartsOn:        lesson.ScheduledOn,
		EndsOn:          formDateOrEmpty(r, "repeat_until"),
		OccurrenceCount: formInt(r, "repeat_count"),
	}
	if sr.Weekdays == "" {
		return sr, nil, errNoWeekdays
	}
	if sr.EndsOn == "" && sr.OccurrenceCount <= 0 {
		return sr, nil, errNoSeriesEnd
	}
	dates, err := occurrenceDates(sr.StartsOn, sr.EndsOn, sr.OccurrenceCount, parseWeekdays(sr.Weekdays))
	return sr, dates, err
}
