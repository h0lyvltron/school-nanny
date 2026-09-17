package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	settingsPeople = "people"
	settingsSchool = "school"
	settingsAccess = "access"
	settingsData   = "data"
)

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings/people", http.StatusSeeOther)
}

func (a *App) handleSettingsSection(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	switch section {
	case settingsPeople, settingsSchool, settingsAccess, settingsData:
	default:
		a.notFound(w)
		return
	}
	if !a.requireNotKid(w, r) {
		return
	}
	if a.hosted {
		if sess := sessionFrom(r); sess != nil && sess.Role == roleCaregiver {
			http.Error(w, "That page is for grown-ups.", http.StatusForbidden)
			return
		}
	}
	if section == settingsData && !a.requireOwner(w, r) {
		return
	}

	data, err := a.settingsPageData(r, section)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.render(w, "settings_"+section, data)
}

func (a *App) settingsPageData(r *http.Request, section string) (map[string]any, error) {
	data, err := a.pageData(r, "settings")
	if err != nil {
		return nil, err
	}

	kids, err := a.store.Kids(true)
	if err != nil {
		return nil, err
	}
	subjects, err := a.store.Subjects(true)
	if err != nil {
		return nil, err
	}
	years, err := a.store.SchoolYears()
	if err != nil {
		return nil, err
	}
	adults, err := a.store.Adults(true)
	if err != nil {
		return nil, err
	}
	backups, err := a.Backups()
	if err != nil {
		return nil, err
	}

	data["Kids"] = kids
	data["Adults"] = adults
	data["Subjects"] = subjects
	data["Years"] = years
	data["SuggestedYear"] = suggestedSchoolYear()
	data["Palette"] = kidPalette
	data["NextColor"] = kidPalette[len(kids)%len(kidPalette)]
	data["NextSubjectColor"] = subjectPalette[len(subjects)%len(subjectPalette)]
	data["Saved"] = r.URL.Query().Get("saved")
	data["NewPIN"] = r.URL.Query().Get("pin")
	data["NewPINWho"] = r.URL.Query().Get("who")
	data["Backups"] = backups
	data["SettingsSection"] = section
	data["ShowDataNav"] = !a.hosted || data["IsOwner"] == true

	if a.hosted {
		if sess := sessionFrom(r); sess != nil && (sess.IsOwner() || sess.CanManageKidLogins) {
			members, err := a.control.ListMemberships(sess.FamilyID)
			if err != nil {
				return nil, err
			}
			data["Memberships"] = members
		}
	}
	return data, nil
}

// kidPalette gives each child a distinct, readable color without asking the
// parent to think about hex codes.
var kidPalette = []string{
	"#5b8def", "#e0709a", "#3fae7f", "#e0913f", "#8d78e0", "#3fa8b8", "#c2544d", "#6f8f3f",
}

// subjectPalette colors the lesson titles. These are read as text rather than
// filled behind it, so they are deeper than the kid colors: they have to hold
// up against paper in the light theme and still lift off the dark one.
var subjectPalette = []string{
	"#2f6ecb", "#b8437a", "#1f8a63", "#b56a12", "#6f5bc9", "#12808f", "#b03a33", "#5a7226",
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
	a.redirect(w, r, "/settings/people?saved=kid")
}

func (a *App) handleDeleteKid(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	// The database cascade takes the records; the photo is a file on disk and
	// has to be cleaned up here.
	kid, err := a.store.Kid(id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.serverError(w, err)
		return
	}
	if err := a.store.DeleteKid(id); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.ClearHistory(); err != nil {
		a.serverError(w, err)
		return
	}
	a.removeUpload(kid.AvatarPath)
	_ = a.gcHistoryFiles()
	a.redirect(w, r, "/settings/people?saved=kid-removed")
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
	color := strings.TrimSpace(r.FormValue("color"))
	if color == "" {
		color = subjectPalette[0]
	}

	var err error
	if id := formID(r, "id"); id > 0 {
		err = a.store.UpdateSubject(id, name, color, r.FormValue("archived") == "on")
	} else {
		_, err = a.store.CreateSubject(name, color)
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings/school?saved=subject")
}

func (a *App) handleDeleteSubject(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteSubject(pathID(r, "id")); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.ClearHistory(); err != nil {
		a.serverError(w, err)
		return
	}
	_ = a.gcHistoryFiles()
	a.redirect(w, r, "/settings/school?saved=subject-removed")
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
	a.redirect(w, r, "/settings/school?saved=year")
}

func (a *App) handleDeleteSchoolYear(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteSchoolYear(pathID(r, "id")); err != nil {
		a.serverError(w, err)
		return
	}
	if err := a.store.ClearHistory(); err != nil {
		a.serverError(w, err)
		return
	}
	_ = a.gcHistoryFiles()
	a.redirect(w, r, "/settings/school?saved=year-removed")
}

func (a *App) handleSavePassword(w http.ResponseWriter, r *http.Request) {
	if a.hosted {
		http.NotFound(w, r)
		return
	}
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
		a.redirect(w, r, "/settings/access?saved=password-cleared")
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
	a.redirect(w, r, "/settings/access?saved=password")
}

func (a *App) handleSaveTimezone(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.FormValue("timezone"))
	if raw == "" {
		raw = strings.TrimSpace(r.FormValue("preset"))
	}
	if raw == "" {
		if err := a.store.DeleteSetting(settingTimezone); err != nil {
			a.serverError(w, err)
			return
		}
		a.redirect(w, r, "/settings/school?saved=timezone")
		return
	}
	if loadLocation(raw) == nil {
		http.Error(w, "That timezone is not recognised.", http.StatusBadRequest)
		return
	}
	if err := a.store.SetSetting(settingTimezone, raw); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings/school?saved=timezone")
}

func (a *App) handleSaveWeekStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	day := parseWeekStart(r.FormValue("week_starts_on"))
	if err := a.store.SetSetting(settingWeekStart, weekStartValue(day)); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings/school?saved=week-start")
}

// Backups --------------------------------------------------------------------

func (a *App) handleMakeBackup(w http.ResponseWriter, r *http.Request) {
	if !a.requireOwner(w, r) {
		return
	}
	if _, err := a.MakeBackup(); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings/data?saved=backup")
}

func (a *App) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if !a.requireOwner(w, r) {
		return
	}
	err := a.RestoreBackup(r.PathValue("name"))
	if errors.Is(err, errNoSuchBackup) || errors.Is(err, errNotABackup) {
		http.Error(w, "That backup could not be found.", http.StatusNotFound)
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings/data?saved=restored")
}

func (a *App) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if !a.requireOwner(w, r) {
		return
	}
	err := a.DeleteBackup(r.PathValue("name"))
	if errors.Is(err, errNoSuchBackup) {
		http.Error(w, "That backup could not be found.", http.StatusNotFound)
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, "/settings/data?saved=backup-removed")
}

// handleDownloadBackup hands over a snapshot so it can be kept somewhere other
// than this computer, which is the only kind of backup that survives the disk.
func (a *App) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	if !a.requireOwner(w, r) {
		return
	}
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
