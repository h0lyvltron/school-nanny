package main

import (
	"database/sql"
	"errors"
	"net/http"
	"time"
)

// KidToday is one child's slice of the family home page.
type KidToday struct {
	Kid     Kid
	Lessons []Lesson
	Week    Progress
}

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	data, err := a.pageData("home")
	if err != nil {
		a.serverError(w, err)
		return
	}

	kids, _ := data["NavKids"].([]Kid)
	now := today()
	start := weekStart(time.Now()).Format(dateLayout)
	end := addDays(start, 6)

	weekLessons, err := a.store.LessonsBetween(start, end, 0)
	if err != nil {
		a.serverError(w, err)
		return
	}

	byKidToday := map[int64][]Lesson{}
	for _, l := range weekLessons {
		if l.ScheduledOn == now {
			byKidToday[l.KidID] = append(byKidToday[l.KidID], l)
		}
	}

	cards := make([]KidToday, 0, len(kids))
	for _, kid := range kids {
		progress, err := a.store.ProgressBetween(start, end, kid.ID, 0)
		if err != nil {
			a.serverError(w, err)
			return
		}
		cards = append(cards, KidToday{Kid: kid, Lessons: byKidToday[kid.ID], Week: progress})
	}

	overdue, err := a.store.LessonsOverdue(now, 25)
	if err != nil {
		a.serverError(w, err)
		return
	}
	familyWeek, err := a.store.ProgressBetween(start, end, 0, 0)
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Cards"] = cards
	data["Overdue"] = overdue
	data["WeekStart"] = start
	data["WeekEnd"] = end
	data["FamilyWeek"] = familyWeek
	data["Subjects"] = subjects
	data["NeedsSetup"] = len(kids) == 0
	a.render(w, "home", data)
}

// PlannerDay is one column of the week grid.
type PlannerDay struct {
	Date    string
	Lessons []Lesson
}

func (a *App) handlePlanner(w http.ResponseWriter, r *http.Request) {
	data, err := a.pageData("planner")
	if err != nil {
		a.serverError(w, err)
		return
	}

	start := weekStart(parseDate(r.URL.Query().Get("week"))).Format(dateLayout)
	end := addDays(start, 6)
	kidFilter := int64(0)
	if raw := r.URL.Query().Get("kid"); raw != "" {
		kidFilter = parseInt64(raw)
	}

	lessons, err := a.store.LessonsBetween(start, end, kidFilter)
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
	progress, err := a.store.ProgressBetween(start, end, kidFilter, 0)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Days"] = days
	data["Subjects"] = subjects
	data["KidFilter"] = kidFilter
	data["WeekStart"] = start
	data["WeekEnd"] = end
	data["PrevWeek"] = addDays(start, -7)
	data["NextWeek"] = addDays(start, 7)
	data["ThisWeek"] = weekStart(time.Now()).Format(dateLayout)
	data["Progress"] = progress
	a.render(w, "planner", data)
}

// SubjectCard summarises one subject on a child's page.
type SubjectCard struct {
	Subject   Subject
	Week      Progress
	Year      Progress
	LastTest  *Assessment
	NextUp    *Lesson
	TotalWork int
}

func (a *App) handleKid(w http.ResponseWriter, r *http.Request) {
	kid, ok := a.lookupKid(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("kids")
	if err != nil {
		a.serverError(w, err)
		return
	}

	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}

	weekFrom := weekStart(time.Now()).Format(dateLayout)
	weekTo := addDays(weekFrom, 6)
	yearFrom, yearTo, yearName, err := a.yearRange()
	if err != nil {
		a.serverError(w, err)
		return
	}

	weekBySubject, err := a.store.ProgressBySubject(weekFrom, weekTo, kid.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	yearBySubject, err := a.store.ProgressBySubject(yearFrom, yearTo, kid.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}

	tests, err := a.store.Assessments(kid.ID, 0, 200)
	if err != nil {
		a.serverError(w, err)
		return
	}
	lastTest := map[int64]Assessment{}
	for _, t := range tests {
		if _, seen := lastTest[t.SubjectID]; !seen {
			lastTest[t.SubjectID] = t
		}
	}

	upcoming, err := a.store.LessonsBetween(today(), addDays(today(), 30), kid.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	nextUp := map[int64]Lesson{}
	for _, l := range upcoming {
		if l.Status != StatusPlanned {
			continue
		}
		if _, seen := nextUp[l.SubjectID]; !seen {
			nextUp[l.SubjectID] = l
		}
	}

	cards := make([]SubjectCard, 0, len(subjects))
	for _, subject := range subjects {
		card := SubjectCard{
			Subject: subject,
			Week:    weekBySubject[subject.ID],
			Year:    yearBySubject[subject.ID],
		}
		if t, ok := lastTest[subject.ID]; ok {
			card.LastTest = &t
		}
		if l, ok := nextUp[subject.ID]; ok {
			card.NextUp = &l
		}
		card.TotalWork = card.Year.Total()
		cards = append(cards, card)
	}

	notes, err := a.store.Notes(kid.ID, 0, 10)
	if err != nil {
		a.serverError(w, err)
		return
	}
	weekTotal, err := a.store.ProgressBetween(weekFrom, weekTo, kid.ID, 0)
	if err != nil {
		a.serverError(w, err)
		return
	}
	yearTotal, err := a.store.ProgressBetween(yearFrom, yearTo, kid.ID, 0)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Kid"] = kid
	data["Cards"] = cards
	data["Notes"] = notes
	data["Subjects"] = subjects
	data["WeekTotal"] = weekTotal
	data["YearTotal"] = yearTotal
	data["YearName"] = yearName
	data["RecentTests"] = firstN(tests, 5)
	a.render(w, "kid", data)
}

func (a *App) handleSubject(w http.ResponseWriter, r *http.Request) {
	kid, ok := a.lookupKid(w, r)
	if !ok {
		return
	}
	subject, err := a.store.Subject(pathID(r, "subjectID"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	data, err := a.pageData("kids")
	if err != nil {
		a.serverError(w, err)
		return
	}

	upcoming, past, err := a.store.LessonsForKidSubjectSplit(kid.ID, subject.ID, 200)
	if err != nil {
		a.serverError(w, err)
		return
	}
	resources, err := a.store.ResourceAttachments(kid.ID, subject.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	tests, err := a.store.Assessments(kid.ID, subject.ID, 50)
	if err != nil {
		a.serverError(w, err)
		return
	}
	notes, err := a.store.Notes(kid.ID, subject.ID, 50)
	if err != nil {
		a.serverError(w, err)
		return
	}

	weekFrom := weekStart(time.Now()).Format(dateLayout)
	weekTo := addDays(weekFrom, 6)
	yearFrom, yearTo, yearName, err := a.yearRange()
	if err != nil {
		a.serverError(w, err)
		return
	}
	week, err := a.store.ProgressBetween(weekFrom, weekTo, kid.ID, subject.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	year, err := a.store.ProgressBetween(yearFrom, yearTo, kid.ID, subject.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Kid"] = kid
	data["Subject"] = subject
	data["Upcoming"] = upcoming
	data["Past"] = past
	data["Resources"] = resources
	data["Tests"] = tests
	data["Notes"] = notes
	data["Week"] = week
	data["Year"] = year
	data["YearName"] = yearName
	a.render(w, "subject", data)
}

func (a *App) handleLesson(w http.ResponseWriter, r *http.Request) {
	lesson, err := a.store.Lesson(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	data, err := a.pageData("")
	if err != nil {
		a.serverError(w, err)
		return
	}

	lesson.Attachments, err = a.store.AttachmentsForLesson(lesson.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	lesson.Assessments, err = a.store.AssessmentsForLesson(lesson.ID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Lesson"] = lesson
	data["Subjects"] = subjects
	data["Kids"] = data["NavKids"]
	a.render(w, "lesson", data)
}

func (a *App) handleTests(w http.ResponseWriter, r *http.Request) {
	kid, ok := a.lookupKid(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("kids")
	if err != nil {
		a.serverError(w, err)
		return
	}

	subjectFilter := parseInt64(r.URL.Query().Get("subject"))
	tests, err := a.store.Assessments(kid.ID, subjectFilter, 200)
	if err != nil {
		a.serverError(w, err)
		return
	}
	for i := range tests {
		tests[i].Attachments, err = a.store.AttachmentsForAssessment(tests[i].ID)
		if err != nil {
			a.serverError(w, err)
			return
		}
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Kid"] = kid
	data["Tests"] = tests
	data["Subjects"] = subjects
	data["SubjectFilter"] = subjectFilter
	a.render(w, "tests", data)
}

func (a *App) lookupKid(w http.ResponseWriter, r *http.Request) (Kid, bool) {
	kid, err := a.store.Kid(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return Kid{}, false
		}
		a.serverError(w, err)
		return Kid{}, false
	}
	return kid, true
}

// yearRange returns the current school year's bounds, falling back to the last
// twelve months when no year has been set up yet.
func (a *App) yearRange() (from, to, name string, err error) {
	year, err := a.store.CurrentSchoolYear()
	if err != nil {
		return "", "", "", err
	}
	if year.ID != 0 {
		return year.StartsOn, year.EndsOn, year.Name, nil
	}
	return addDays(today(), -365), addDays(today(), 365), "All time", nil
}

func firstN[T any](items []T, n int) []T {
	if len(items) <= n {
		return items
	}
	return items[:n]
}
