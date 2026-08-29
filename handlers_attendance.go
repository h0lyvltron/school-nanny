package main

import (
	"net/http"
	"strings"
	"time"
)

type CalendarDay struct {
	Date    string
	InMonth bool
	Marks   []Attendance
}

type MonthReport struct {
	Key    string
	Label  string
	From   string
	To     string
	Totals AttendanceTotals
}

type KidAttendanceReport struct {
	Kid    Kid
	Months []MonthReport
	Year   AttendanceTotals
}

func (a *App) handleAttendance(w http.ResponseWriter, r *http.Request) {
	data, err := a.pageData("attendance")
	if err != nil {
		a.serverError(w, err)
		return
	}

	month := parseMonthQuery(r.URL.Query().Get("month"))
	kidFilter := parseInt64(r.URL.Query().Get("kid"))

	if err := a.populateAttendance(data, month, kidFilter); err != nil {
		a.serverError(w, err)
		return
	}
	a.render(w, "attendance", data)
}

func (a *App) handleSaveAttendance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	kidID := formID(r, "kid_id")
	date := formDate(r, "attended_on")
	status := normalizeAttendance(r.FormValue("status"))

	if kidID == 0 {
		http.Error(w, "Attendance needs a child.", http.StatusBadRequest)
		return
	}
	if _, err := a.store.Kid(kidID); err != nil {
		http.Error(w, "That child could not be found.", http.StatusBadRequest)
		return
	}

	current, err := a.store.AttendanceForKidOn(kidID, date)
	if err != nil {
		a.serverError(w, err)
		return
	}

	notes := current.Notes
	if r.PostForm.Has("notes") {
		notes = strings.TrimSpace(r.FormValue("notes"))
	}

	if status == "cycle" {
		status = nextAttendanceStatus(current.Status)
	} else if status == current.Status && !r.PostForm.Has("notes") {
		status = ""
	}

	if status == "" {
		err = a.store.DeleteAttendance(kidID, date)
	} else {
		err = a.store.UpsertAttendance(kidID, date, status, notes)
	}
	if err != nil {
		a.serverError(w, err)
		return
	}

	if r.Header.Get("HX-Request") != "true" {
		a.redirect(w, r, safeRedirect(r.FormValue("back"), "/attendance"))
		return
	}

	switch r.FormValue("view") {
	case "today":
		a.renderTodayAttendance(w, kidID, date)
	case "calendar":
		month := monthFirst(r.FormValue("month"))
		if month == "" {
			month = monthFirst(date)
		}
		a.renderAttendanceBoard(w, month, formID(r, "kid_filter"))
	default:
		a.redirect(w, r, safeRedirect(r.FormValue("back"), "/attendance"))
	}
}

func (a *App) renderTodayAttendance(w http.ResponseWriter, kidID int64, date string) {
	kid, err := a.store.Kid(kidID)
	if err != nil {
		a.serverError(w, err)
		return
	}
	mark, err := a.store.AttendanceForKidOn(kidID, date)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.renderPartial(w, "attendance_mark", map[string]any{
		"Kid":        kid,
		"Attendance": mark,
		"Today":      date,
	})
}

func (a *App) renderAttendanceBoard(w http.ResponseWriter, month string, kidFilter int64) {
	data := map[string]any{}
	if err := a.populateAttendance(data, month, kidFilter); err != nil {
		a.serverError(w, err)
		return
	}
	a.renderPartial(w, "attendance_board", data)
}

func (a *App) populateAttendance(data map[string]any, month string, kidFilter int64) error {
	month = monthFirst(month)
	from := month
	to := monthLast(month)

	yearFrom, yearTo, yearName, err := a.yearRange()
	if err != nil {
		return err
	}
	rangeFrom := maxDate(from, yearFrom)
	rangeTo := minDate(to, yearTo)
	if rangeTo < rangeFrom {
		rangeFrom, rangeTo = from, to
	}

	kids, _ := data["NavKids"].([]Kid)
	if kids == nil {
		kids, err = a.store.Kids(false)
		if err != nil {
			return err
		}
	}

	marks, err := a.store.AttendanceBetween(from, to, kidFilter)
	if err != nil {
		return err
	}
	byKidDay := map[int64]map[string]Attendance{}
	for _, m := range marks {
		if byKidDay[m.KidID] == nil {
			byKidDay[m.KidID] = map[string]Attendance{}
		}
		byKidDay[m.KidID][m.AttendedOn] = m
	}

	showKids := kids
	if kidFilter > 0 {
		showKids = nil
		for _, k := range kids {
			if k.ID == kidFilter {
				showKids = []Kid{k}
				break
			}
		}
		if len(showKids) == 0 {
			kid, err := a.store.Kid(kidFilter)
			if err == nil {
				showKids = []Kid{kid}
			}
		}
	}

	weeks := attendanceMonthGrid(month, showKids, byKidDay)
	totals, err := a.store.AttendanceTotalsBetween(rangeFrom, rangeTo, kidFilter)
	if err != nil {
		return err
	}

	reports, err := a.attendanceYearReports(kids, kidFilter, yearFrom, yearTo)
	if err != nil {
		return err
	}

	data["Month"] = month
	data["MonthLabel"] = formatDate(month, "January 2006")
	data["PrevMonth"] = addMonths(month, -1)
	data["NextMonth"] = addMonths(month, 1)
	data["KidFilter"] = kidFilter
	data["Weeks"] = weeks
	data["Totals"] = totals
	data["YearName"] = yearName
	data["YearFrom"] = yearFrom
	data["YearTo"] = yearTo
	data["Reports"] = reports
	data["ShowKids"] = showKids
	data["NavKids"] = kids
	data["Today"] = today()
	return nil
}

func attendanceMonthGrid(month string, kids []Kid, byKidDay map[int64]map[string]Attendance) [][]CalendarDay {
	first := parseDate(monthFirst(month))
	start := weekStart(first)
	last := parseDate(monthLast(month))
	end := last.AddDate(0, 0, (7-int(last.Weekday()))%7)
	if end.Before(last) {
		end = last
	}

	var weeks [][]CalendarDay
	var week []CalendarDay
	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		date := t.Format(dateLayout)
		day := CalendarDay{Date: date, InMonth: t.Month() == first.Month()}
		for _, kid := range kids {
			mark := Attendance{KidID: kid.ID, AttendedOn: date, KidName: kid.Name, KidColor: kid.Color}
			if days, ok := byKidDay[kid.ID]; ok {
				if existing, ok := days[date]; ok {
					mark = existing
				}
			}
			day.Marks = append(day.Marks, mark)
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
	return weeks
}

func (a *App) attendanceYearReports(kids []Kid, kidFilter int64, yearFrom, yearTo string) ([]KidAttendanceReport, error) {
	var selected []Kid
	if kidFilter > 0 {
		for _, k := range kids {
			if k.ID == kidFilter {
				selected = []Kid{k}
				break
			}
		}
		if selected == nil {
			kid, err := a.store.Kid(kidFilter)
			if err != nil {
				return nil, err
			}
			selected = []Kid{kid}
		}
	} else {
		selected = kids
	}

	months := monthsInRange(yearFrom, yearTo)
	var reports []KidAttendanceReport
	for _, kid := range selected {
		rep := KidAttendanceReport{Kid: kid}
		year, err := a.store.AttendanceTotalsBetween(yearFrom, yearTo, kid.ID)
		if err != nil {
			return nil, err
		}
		rep.Year = year
		for _, m := range months {
			from := maxDate(m, yearFrom)
			to := minDate(monthLast(m), yearTo)
			tot, err := a.store.AttendanceTotalsBetween(from, to, kid.ID)
			if err != nil {
				return nil, err
			}
			rep.Months = append(rep.Months, MonthReport{
				Key:    m[:7],
				Label:  formatDate(m, "Jan 2006"),
				From:   from,
				To:     to,
				Totals: tot,
			})
		}
		reports = append(reports, rep)
	}
	return reports, nil
}

func monthsInRange(from, to string) []string {
	start := monthFirst(from)
	end := monthFirst(to)
	if end < start {
		return nil
	}
	var out []string
	for m := start; m <= end; m = addMonths(m, 1) {
		out = append(out, m)
		if len(out) > 24 {
			break
		}
	}
	return out
}

func normalizeAttendance(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case AttendancePresent, AttendanceAbsent, AttendanceExcused, "cycle":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func nextAttendanceStatus(current string) string {
	switch current {
	case AttendancePresent:
		return AttendanceAbsent
	case AttendanceAbsent:
		return AttendanceExcused
	case AttendanceExcused:
		return ""
	default:
		return AttendancePresent
	}
}

func parseMonthQuery(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 7 && raw[4] == '-' {
		raw = raw + "-01"
	}
	if _, err := time.Parse(dateLayout, raw); err != nil {
		return monthFirst(today())
	}
	return monthFirst(raw)
}
