package main

import (
	"net/http"
)

// TranscriptCourse is one subject row on a printable year report.
type TranscriptCourse struct {
	Subject Subject
	Hours   Progress
	Grade   CourseGrade
	Tests   int
}

func (a *App) handleTranscript(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	kid, ok := a.lookupKid(w, r)
	if !ok {
		return
	}
	if !a.enforceKidScope(w, r, kid.ID) {
		return
	}

	data, err := a.pageData(r, "transcript")
	if err != nil {
		a.serverError(w, err)
		return
	}

	years, err := a.store.SchoolYears()
	if err != nil {
		a.serverError(w, err)
		return
	}
	yearID := parseInt64(r.URL.Query().Get("year"))
	if yearID == 0 {
		if current, err := a.store.CurrentSchoolYear(); err == nil && current.ID != 0 {
			yearID = current.ID
		} else if len(years) > 0 {
			yearID = years[0].ID
		}
	}
	if yearID == 0 {
		data["Kid"] = kid
		data["Years"] = years
		data["NeedsYear"] = true
		a.render(w, "transcript", data)
		return
	}
	year, err := a.store.SchoolYear(yearID)
	if err != nil {
		a.notFound(w)
		return
	}

	from, to := year.StartsOn, year.EndsOn
	bySubject, err := a.store.ProgressBySubject(from, to, kid.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	total, err := a.store.ProgressBetween(from, to, kid.ID, 0)
	if err != nil {
		a.serverError(w, err)
		return
	}
	attendance, err := a.store.AttendanceTotalsBetween(from, to, kid.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	tests, err := a.store.AssessmentsBetween(from, to, kid.ID, 0, 500)
	if err != nil {
		a.serverError(w, err)
		return
	}
	testsBySubject := map[int64][]Assessment{}
	for _, t := range tests {
		testsBySubject[t.SubjectID] = append(testsBySubject[t.SubjectID], t)
	}

	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	courses := make([]TranscriptCourse, 0, len(subjects))
	for _, sub := range subjects {
		hours := bySubject[sub.ID]
		grade := CourseGradeFromAssessments(testsBySubject[sub.ID])
		if hours.Total() == 0 && !grade.HasGrade() {
			continue
		}
		courses = append(courses, TranscriptCourse{
			Subject: sub,
			Hours:   hours,
			Grade:   grade,
			Tests:   len(testsBySubject[sub.ID]),
		})
	}

	data["Kid"] = kid
	data["Year"] = year
	data["Years"] = years
	data["YearID"] = yearID
	data["Courses"] = courses
	data["Total"] = total
	data["Attendance"] = attendance
	data["TestCount"] = len(tests)
	a.render(w, "transcript", data)
}
