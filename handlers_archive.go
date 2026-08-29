package main

import (
	"fmt"
	"net/http"
)

func (a *App) handleArchive(w http.ResponseWriter, r *http.Request) {
	data, err := a.pageData("archive")
	if err != nil {
		a.serverError(w, err)
		return
	}

	kids, err := a.store.Kids(true)
	if err != nil {
		a.serverError(w, err)
		return
	}
	years, err := a.store.SchoolYears()
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(true)
	if err != nil {
		a.serverError(w, err)
		return
	}

	kidID := parseInt64(r.URL.Query().Get("kid"))
	yearID := parseInt64(r.URL.Query().Get("year"))
	subjectFilter := parseInt64(r.URL.Query().Get("subject"))

	if kidID == 0 && len(kids) > 0 {
		kidID = kids[0].ID
	}
	if yearID == 0 {
		if current, err := a.store.CurrentSchoolYear(); err == nil && current.ID != 0 {
			yearID = current.ID
		} else if len(years) > 0 {
			yearID = years[0].ID
		}
	}

	data["Kids"] = kids
	data["Years"] = years
	data["Subjects"] = subjects
	data["KidID"] = kidID
	data["YearID"] = yearID
	data["SubjectFilter"] = subjectFilter

	if kidID == 0 || yearID == 0 {
		data["NeedsSetup"] = len(kids) == 0 || len(years) == 0
		a.render(w, "archive", data)
		return
	}

	kid, err := a.store.Kid(kidID)
	if err != nil {
		a.notFound(w)
		return
	}
	year, err := a.store.SchoolYear(yearID)
	if err != nil {
		a.notFound(w)
		return
	}

	from, to := year.StartsOn, year.EndsOn
	progress, err := a.store.ProgressBetween(from, to, kid.ID, subjectFilter)
	if err != nil {
		a.serverError(w, err)
		return
	}
	bySubject, err := a.store.ProgressBySubject(from, to, kid.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	attendance, err := a.store.AttendanceTotalsBetween(from, to, kid.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	lessons, err := a.store.LessonsInRange(from, to, kid.ID, subjectFilter)
	if err != nil {
		a.serverError(w, err)
		return
	}
	tests, err := a.store.AssessmentsBetween(from, to, kid.ID, subjectFilter, 200)
	if err != nil {
		a.serverError(w, err)
		return
	}
	notes, err := a.store.NotesBetween(from, to, kid.ID, subjectFilter, 100)
	if err != nil {
		a.serverError(w, err)
		return
	}

	type subjectYear struct {
		Subject Subject
		Year    Progress
	}
	cards := make([]subjectYear, 0, len(subjects))
	activeSubjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	for _, sub := range activeSubjects {
		p := bySubject[sub.ID]
		if p.Total() == 0 {
			continue
		}
		cards = append(cards, subjectYear{Subject: sub, Year: p})
	}

	data["Kid"] = kid
	data["Year"] = year
	data["Progress"] = progress
	data["Attendance"] = attendance
	data["Lessons"] = lessons
	data["Tests"] = tests
	data["Notes"] = notes
	data["Cards"] = cards
	data["HasYear"] = true
	a.render(w, "archive", data)
}

func (a *App) handleArchiveExport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	kidID := formID(r, "kid_id")
	yearID := formID(r, "year_id")
	subjectID := formID(r, "subject_id")
	if kidID == 0 || yearID == 0 || subjectID == 0 {
		http.Error(w, "Pick a child, a year, and a subject to save as a template.", http.StatusBadRequest)
		return
	}

	kid, err := a.store.Kid(kidID)
	if err != nil {
		a.notFound(w)
		return
	}
	year, err := a.store.SchoolYear(yearID)
	if err != nil {
		a.notFound(w)
		return
	}
	subject, err := a.store.Subject(subjectID)
	if err != nil {
		a.notFound(w)
		return
	}

	lessons, err := a.store.DoneLessonsInRange(year.StartsOn, year.EndsOn, kidID, subjectID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	if len(lessons) == 0 {
		http.Error(w, "There are no finished lessons in that subject to save.", http.StatusBadRequest)
		return
	}

	name := fmt.Sprintf("%s · %s · %s", kid.Name, subject.Name, year.Name)
	id, err := a.store.CreatePlanFromLessons(name, subjectID, kidID, yearID, lessons)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum/"+itoa(id))
}
