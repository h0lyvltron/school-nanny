package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type SubjectPlans struct {
	Subject Subject
	Plans   []CurriculumPlan
}

type ApplyPreview struct {
	Date  string
	Title string
}

func (a *App) handleCurriculum(w http.ResponseWriter, r *http.Request) {
	data, err := a.curriculumPageData()
	if err != nil {
		a.serverError(w, err)
		return
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("imported")); err == nil && n > 0 {
		data["Imported"] = n
	}
	a.render(w, "curriculum", data)
}

func (a *App) curriculumPageData() (map[string]any, error) {
	data, err := a.pageData("curriculum")
	if err != nil {
		return nil, err
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		return nil, err
	}
	plans, err := a.store.CurriculumPlans()
	if err != nil {
		return nil, err
	}

	bySubject := map[int64][]CurriculumPlan{}
	for _, p := range plans {
		bySubject[p.SubjectID] = append(bySubject[p.SubjectID], p)
	}
	groups := make([]SubjectPlans, 0, len(subjects))
	for _, sub := range subjects {
		groups = append(groups, SubjectPlans{Subject: sub, Plans: bySubject[sub.ID]})
	}

	data["Groups"] = groups
	data["Subjects"] = subjects
	data["PlanCount"] = len(plans)
	return data, nil
}

func (a *App) renderCurriculumImportError(w http.ResponseWriter, msg string) {
	data, err := a.curriculumPageData()
	if err != nil {
		a.serverError(w, err)
		return
	}
	data["ImportError"] = msg
	a.render(w, "curriculum", data)
}

func importUploadError(err error) string {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return "That file is too large. Keep imports under 1 MB."
	}
	return "That file was too large or the upload was incomplete."
}

func (a *App) handleImportCurriculum(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+64*1024)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		a.renderCurriculumImportError(w, importUploadError(err))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		a.renderCurriculumImportError(w, "Choose a YAML or CSV file to import.")
		return
	}
	header := files[0]
	src, err := header.Open()
	if err != nil {
		a.renderCurriculumImportError(w, "Could not read that file.")
		return
	}
	defer src.Close()

	body, err := io.ReadAll(io.LimitReader(src, int64(maxImportBytes)+1))
	if err != nil {
		a.renderCurriculumImportError(w, "Could not read that file.")
		return
	}
	if len(body) > maxImportBytes {
		a.renderCurriculumImportError(w, "That file is too large. Keep imports under 1 MB.")
		return
	}

	subjects, err := a.store.Subjects(true)
	if err != nil {
		a.serverError(w, err)
		return
	}
	plans, err := parseCurriculumImport(header.Filename, body, subjects)
	if err != nil {
		a.renderCurriculumImportError(w, err.Error())
		return
	}
	n, err := a.store.ImportCurriculum(plans)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum?imported="+strconv.Itoa(n))
}

func (a *App) handleCreateCurriculumPlan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	plan := CurriculumPlan{
		Name:      strings.TrimSpace(r.FormValue("name")),
		SubjectID: formID(r, "subject_id"),
		Kind:      PlanAuthored,
		Notes:     strings.TrimSpace(r.FormValue("notes")),
	}
	if plan.Name == "" || plan.SubjectID == 0 {
		http.Error(w, "A plan needs a name and a subject.", http.StatusBadRequest)
		return
	}
	id, err := a.store.CreateCurriculumPlan(plan)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum/"+itoa(id))
}

func (a *App) handleCurriculumPlan(w http.ResponseWriter, r *http.Request) {
	plan, ok := a.lookupPlan(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("curriculum")
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(false)
	if err != nil {
		a.serverError(w, err)
		return
	}
	kids, err := a.store.Kids(false)
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Plan"] = plan
	data["Subjects"] = subjects
	data["Kids"] = kids
	data["Weekdays"] = weekdayChoices(defaultWeekdays())
	data["Start"] = today()
	a.render(w, "curriculum_plan", data)
}

func (a *App) handleUpdateCurriculumPlan(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	plan := CurriculumPlan{
		Name:      strings.TrimSpace(r.FormValue("name")),
		SubjectID: formID(r, "subject_id"),
		Notes:     strings.TrimSpace(r.FormValue("notes")),
	}
	if plan.Name == "" || plan.SubjectID == 0 {
		http.Error(w, "A plan needs a name and a subject.", http.StatusBadRequest)
		return
	}
	if err := a.store.UpdateCurriculumPlan(id, plan); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum/"+r.PathValue("id"))
}

func (a *App) handleDeleteCurriculumPlan(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	files, err := a.store.AttachmentsForPlan(id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, err)
		return
	}
	a.removeFiles(files)
	if err := a.store.DeleteCurriculumPlan(id); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum")
}

func (a *App) handleCreateCurriculumItem(w http.ResponseWriter, r *http.Request) {
	planID := pathID(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	item := CurriculumItem{
		PlanID:     planID,
		Title:      strings.TrimSpace(r.FormValue("title")),
		Notes:      strings.TrimSpace(r.FormValue("notes")),
		Minutes:    formInt(r, "minutes"),
		WeekNumber: formInt(r, "week_number"),
	}
	if item.Title == "" {
		http.Error(w, "A lesson in the sequence needs a title.", http.StatusBadRequest)
		return
	}
	if _, err := a.store.CreateCurriculumItem(item); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum/"+r.PathValue("id"))
}

func (a *App) handleUpdateCurriculumItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	item := CurriculumItem{
		Title:      strings.TrimSpace(r.FormValue("title")),
		Notes:      strings.TrimSpace(r.FormValue("notes")),
		Minutes:    formInt(r, "minutes"),
		WeekNumber: formInt(r, "week_number"),
	}
	if item.Title == "" {
		http.Error(w, "A lesson in the sequence needs a title.", http.StatusBadRequest)
		return
	}
	if err := a.store.UpdateCurriculumItem(pathID(r, "itemID"), item); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum/"+r.PathValue("id"))
}

func (a *App) handleDeleteCurriculumItem(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteCurriculumItem(pathID(r, "itemID")); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum/"+r.PathValue("id"))
}

func (a *App) handleMoveCurriculumItem(w http.ResponseWriter, r *http.Request) {
	dir := 1
	if r.FormValue("dir") == "up" {
		dir = -1
	}
	if err := a.store.MoveCurriculumItem(pathID(r, "itemID"), dir); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/curriculum/"+r.PathValue("id"))
}

func (a *App) handleApplyCurriculumForm(w http.ResponseWriter, r *http.Request) {
	plan, ok := a.lookupPlan(w, r)
	if !ok {
		return
	}
	data, err := a.pageData("curriculum")
	if err != nil {
		a.serverError(w, err)
		return
	}
	kids, err := a.store.Kids(false)
	if err != nil {
		a.serverError(w, err)
		return
	}

	kidID := parseInt64(r.URL.Query().Get("kid"))
	if kidID == 0 && len(kids) > 0 {
		kidID = kids[0].ID
	}
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	if _, err := time.Parse(dateLayout, start); err != nil {
		start = today()
	}
	weekdays := formWeekdays(r.URL.Query()["weekday"])
	if weekdays == "" {
		weekdays = defaultWeekdays()
	}

	preview, last, err := curriculumPreview(plan, start, weekdays)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data["Plan"] = plan
	data["Kids"] = kids
	data["KidID"] = kidID
	data["Start"] = start
	data["Weekdays"] = weekdayChoices(weekdays)
	data["Preview"] = preview
	data["LastDate"] = last
	data["Count"] = len(plan.Items)
	a.render(w, "curriculum_apply", data)
}

func (a *App) handleApplyCurriculum(w http.ResponseWriter, r *http.Request) {
	plan, ok := a.lookupPlan(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	kidID := formID(r, "kid_id")
	if kidID == 0 {
		http.Error(w, "Pick a child to apply this plan to.", http.StatusBadRequest)
		return
	}
	start := formDate(r, "start")
	weekdays := formWeekdays(r.Form["weekday"])
	if weekdays == "" {
		weekdays = defaultWeekdays()
	}
	if len(plan.Items) == 0 {
		http.Error(w, "This plan has no lessons to apply.", http.StatusBadRequest)
		return
	}

	dates, err := occurrenceDates(start, "", len(plan.Items), parseWeekdays(weekdays))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.store.ApplyCurriculum(plan, kidID, dates); err != nil {
		a.serverError(w, err)
		return
	}

	first := start
	if len(dates) > 0 {
		first = dates[0]
	}
	a.redirect(w, r, "/planner?week="+weekStart(parseDate(first)).Format(dateLayout)+"&kid="+r.FormValue("kid_id"))
}

func curriculumPreview(plan CurriculumPlan, start, weekdays string) ([]ApplyPreview, string, error) {
	if len(plan.Items) == 0 {
		return nil, "", errors.New("this plan has no lessons to apply")
	}
	dates, err := occurrenceDates(start, "", len(plan.Items), parseWeekdays(weekdays))
	if err != nil {
		return nil, "", err
	}
	n := len(plan.Items)
	if len(dates) < n {
		n = len(dates)
	}
	out := make([]ApplyPreview, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, ApplyPreview{Date: dates[i], Title: plan.Items[i].Title})
	}
	last := ""
	if n > 0 {
		last = dates[n-1]
	}
	return out, last, nil
}

func (a *App) lookupPlan(w http.ResponseWriter, r *http.Request) (CurriculumPlan, bool) {
	plan, err := a.store.CurriculumPlan(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return CurriculumPlan{}, false
		}
		a.serverError(w, err)
		return CurriculumPlan{}, false
	}
	return plan, true
}

func itoa(id int64) string {
	return fmt.Sprintf("%d", id)
}
