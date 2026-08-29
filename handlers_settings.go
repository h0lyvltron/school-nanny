package main

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	data, err := a.pageData("settings")
	if err != nil {
		a.serverError(w, err)
		return
	}

	kids, err := a.store.Kids(true)
	if err != nil {
		a.serverError(w, err)
		return
	}
	subjects, err := a.store.Subjects(true)
	if err != nil {
		a.serverError(w, err)
		return
	}
	years, err := a.store.SchoolYears()
	if err != nil {
		a.serverError(w, err)
		return
	}

	backups, err := a.Backups()
	if err != nil {
		a.serverError(w, err)
		return
	}

	data["Kids"] = kids
	data["Subjects"] = subjects
	data["Years"] = years
	data["SuggestedYear"] = suggestedSchoolYear()
	data["Palette"] = kidPalette
	data["NextColor"] = kidPalette[len(kids)%len(kidPalette)]
	data["Saved"] = r.URL.Query().Get("saved")
	data["Backups"] = backups
	data["DataDir"] = a.dataDir
	a.render(w, "settings", data)
}

// kidPalette gives each child a distinct, readable colour without asking the
// parent to think about hex codes.
var kidPalette = []string{
	"#5b8def", "#e0709a", "#3fae7f", "#e0913f", "#8d78e0", "#3fa8b8", "#c2544d", "#6f8f3f",
}

func (a *App) handleSaveKid(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "A child needs a name.", http.StatusBadRequest)
		return
	}
	grade := strings.TrimSpace(r.FormValue("grade"))
	color := strings.TrimSpace(r.FormValue("color"))
	if color == "" {
		color = kidPalette[0]
	}
	archived := r.FormValue("archived") == "on"

	var err error
	if id := formID(r, "id"); id > 0 {
		err = a.store.UpdateKid(id, name, grade, color, archived)
	} else {
		_, err = a.store.CreateKid(name, grade, color)
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=kid")
}

func (a *App) handleDeleteKid(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteKid(pathID(r, "id")); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=kid-removed")
}

func (a *App) handleSaveSubject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "A subject needs a name.", http.StatusBadRequest)
		return
	}

	var err error
	if id := formID(r, "id"); id > 0 {
		err = a.store.UpdateSubject(id, name, r.FormValue("archived") == "on")
	} else {
		_, err = a.store.CreateSubject(name)
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=subject")
}

func (a *App) handleDeleteSubject(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteSubject(pathID(r, "id")); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=subject-removed")
}

func (a *App) handleSaveSchoolYear(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "A school year needs a name.", http.StatusBadRequest)
		return
	}
	starts := formDate(r, "starts_on")
	ends := formDate(r, "ends_on")
	if ends < starts {
		starts, ends = ends, starts
	}
	current := r.FormValue("is_current") == "on"

	var err error
	if id := formID(r, "id"); id > 0 {
		err = a.store.UpdateSchoolYear(id, name, starts, ends, current)
	} else {
		_, err = a.store.CreateSchoolYear(name, starts, ends, current)
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=year")
}

func (a *App) handleDeleteSchoolYear(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteSchoolYear(pathID(r, "id")); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=year-removed")
}

func (a *App) handleSavePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")

	if password == "" {
		if err := a.store.DeleteSetting(settingPassword); err != nil {
			a.serverError(w, err)
			return
		}
		a.clearSession(w)
		a.redirect(w, r, "/settings?saved=password-cleared")
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.SetSetting(settingPassword, hash); err != nil {
		a.serverError(w, err)
		return
	}
	a.issueSession(w)
	a.redirect(w, r, "/settings?saved=password")
}

// Backups --------------------------------------------------------------------

func (a *App) handleMakeBackup(w http.ResponseWriter, r *http.Request) {
	if _, err := a.MakeBackup(); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=backup")
}

func (a *App) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	err := a.RestoreBackup(r.PathValue("name"))
	if errors.Is(err, errNoSuchBackup) || errors.Is(err, errNotABackup) {
		http.Error(w, "That backup could not be found.", http.StatusNotFound)
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=restored")
}

func (a *App) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	err := a.DeleteBackup(r.PathValue("name"))
	if errors.Is(err, errNoSuchBackup) {
		http.Error(w, "That backup could not be found.", http.StatusNotFound)
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings?saved=backup-removed")
}

// handleDownloadBackup hands over a snapshot so it can be kept somewhere other
// than this computer, which is the only kind of backup that survives the disk.
func (a *App) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	path, err := a.backupPath(name)
	if errors.Is(err, errNoSuchBackup) {
		a.notFound(w)
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, path)
}

// suggestedSchoolYear proposes the academic year the family is most likely
// setting up, so the form starts out mostly filled in.
func suggestedSchoolYear() SchoolYear {
	now := time.Now()
	startYear := now.Year()
	if now.Month() < time.July {
		startYear--
	}
	start := time.Date(startYear, time.August, 1, 0, 0, 0, 0, now.Location())
	end := time.Date(startYear+1, time.May, 31, 0, 0, 0, 0, now.Location())
	return SchoolYear{
		Name:     start.Format("2006") + "–" + end.Format("2006"),
		StartsOn: start.Format(dateLayout),
		EndsOn:   end.Format(dateLayout),
	}
}
