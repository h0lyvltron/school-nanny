package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *App) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if a.hosted {
		if sess, err := a.hostedSessionFromRequest(r); err == nil && sess != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		a.render(w, "login", map[string]any{
			"Active":     "login",
			"Hosted":     true,
			"FamilySlug": r.URL.Query().Get("family"),
		})
		return
	}

	hash, err := a.store.Setting(settingPassword)
	if err != nil {
		a.serverError(w, err)
		return
	}
	if hash == "" || a.validSession(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.render(w, "login", map[string]any{"Active": "login"})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	if a.hosted {
		if r.FormValue("method") == "pin" || (r.FormValue("family_slug") != "" && r.FormValue("username") != "") {
			a.handlePINLogin(w, r)
			return
		}
		user, err := a.control.Authenticate(r.FormValue("email"), r.FormValue("password"))
		if errors.Is(err, errBadCredentials) {
			w.WriteHeader(http.StatusUnauthorized)
			a.render(w, "login", map[string]any{
				"Active": "login",
				"Hosted": true,
				"Error":  "That email or password did not match.",
			})
			return
		}
		if err != nil {
			a.serverError(w, err)
			return
		}
		sess, err := a.control.CreateSession(user.ID, user.FamilyID, sessionLifetime)
		if err != nil {
			a.serverError(w, err)
			return
		}
		a.issueHostedSession(w, sess)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	hash, err := a.store.Setting(settingPassword)
	if err != nil {
		a.serverError(w, err)
		return
	}
	if hash == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !checkPassword(hash, r.FormValue("password")) {
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, "login", map[string]any{"Active": "login", "Error": "That password did not match."})
		return
	}
	a.issueSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handlePINLogin(w http.ResponseWriter, r *http.Request) {
	slug := r.FormValue("family_slug")
	username := r.FormValue("username")
	pin := r.FormValue("pin")
	sess, err := a.control.AuthenticatePIN(slug, username, pin)
	if errors.Is(err, errPINLocked) {
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, "login", map[string]any{
			"Active":     "login",
			"Hosted":     true,
			"FamilySlug": slug,
			"Error":      "That PIN is locked for a few minutes. Try again later, or ask the owner to reset it.",
		})
		return
	}
	if errors.Is(err, errBadCredentials) || errors.Is(err, errBadPIN) {
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, "login", map[string]any{
			"Active":     "login",
			"Hosted":     true,
			"FamilySlug": slug,
			"Error":      "That family, username, or PIN did not match.",
		})
		return
	}
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.issueHostedSession(w, sess)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleSignupForm(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if sess, err := a.hostedSessionFromRequest(r); err == nil && sess != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.render(w, "signup", map[string]any{
		"Active":         "signup",
		"Hosted":         true,
		"InviteRequired": a.inviteCode != "",
	})
}

func (a *App) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	familyName := r.FormValue("family_name")
	invite := r.FormValue("invite_code")

	user, family, err := a.control.Signup(email, password, familyName, invite, a.inviteCode)
	if errors.Is(err, errEmailTaken) {
		w.WriteHeader(http.StatusConflict)
		a.render(w, "signup", map[string]any{
			"Active":         "signup",
			"Hosted":         true,
			"InviteRequired": a.inviteCode != "",
			"Error":          "That email is already registered.",
			"Email":          email,
			"FamilyName":     familyName,
		})
		return
	}
	if errors.Is(err, errInviteRequired) || errors.Is(err, errBadInvite) {
		w.WriteHeader(http.StatusForbidden)
		a.render(w, "signup", map[string]any{
			"Active":         "signup",
			"Hosted":         true,
			"InviteRequired": a.inviteCode != "",
			"Error":          "That invite code did not work.",
			"Email":          email,
			"FamilyName":     familyName,
		})
		return
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "required") || strings.Contains(msg, "password") {
			w.WriteHeader(http.StatusBadRequest)
			a.render(w, "signup", map[string]any{
				"Active":         "signup",
				"Hosted":         true,
				"InviteRequired": a.inviteCode != "",
				"Error":          msg,
				"Email":          email,
				"FamilyName":     familyName,
			})
			return
		}
		a.serverError(w, err)
		return
	}

	if err := a.adoptLegacyData(family.ID); err != nil {
		a.serverError(w, err)
		return
	}
	if _, err := a.tenantApp(family.ID); err != nil {
		a.serverError(w, err)
		return
	}

	sess, err := a.control.CreateSession(user.ID, user.FamilyID, sessionLifetime)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.issueHostedSession(w, sess)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if a.hosted {
		if c, err := r.Cookie(hostedSIDCookie); err == nil && c.Value != "" {
			_ = a.control.RevokeSession(c.Value)
		}
		a.clearHostedSession(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	a.clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *App) handleChangeAccountPassword(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if !a.requireOwner(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	sess := sessionFrom(r)
	password := r.FormValue("password")
	if password == "" {
		http.Error(w, "Password must not be empty.", http.StatusBadRequest)
		return
	}
	if err := a.control.UpdatePassword(sess.AccountID, password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = a.control.KillSessionsForAccount(sess.AccountID)
	fresh, err := a.control.CreateSession(sess.AccountID, sess.FamilyID, sessionLifetime)
	if err != nil {
		a.serverError(w, err)
		return
	}
	a.issueHostedSession(w, fresh)
	a.redirect(w, r, "/settings/access?saved=account-password")
}

// requireOwner rejects non-owner hosted sessions. Local mode always passes.
func (a *App) requireOwner(w http.ResponseWriter, r *http.Request) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess == nil || !sess.IsOwner() {
		http.Error(w, "Only the family owner can do that.", http.StatusForbidden)
		return false
	}
	return true
}

func (a *App) requireNotKid(w http.ResponseWriter, r *http.Request) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess != nil && sess.IsKid() {
		http.Error(w, "That page is for grown-ups.", http.StatusForbidden)
		return false
	}
	return true
}

// requirePlanningAccess is the planner gate: owner, co-parent, and teacher.
// Kids and caregivers are blocked. Local mode always passes.
func (a *App) requirePlanningAccess(w http.ResponseWriter, r *http.Request) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess == nil {
		return true
	}
	if sess.IsKid() || sess.Role == roleCaregiver {
		http.Error(w, "That page is for grown-ups.", http.StatusForbidden)
		return false
	}
	return true
}

// requireDayOps allows planners and caregivers (today, planner, attendance,
// lesson status). Kids are blocked. Local mode always passes.
func (a *App) requireDayOps(w http.ResponseWriter, r *http.Request) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess != nil && sess.IsKid() {
		http.Error(w, "That page is for grown-ups.", http.StatusForbidden)
		return false
	}
	return true
}

// enforceKidScope rejects kid sessions looking at another child's records.
func (a *App) enforceKidScope(w http.ResponseWriter, r *http.Request, kidID int64) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess != nil && sess.IsKid() && sess.KidID != kidID {
		http.Error(w, "That page is not yours.", http.StatusForbidden)
		return false
	}
	return true
}

// enforceLessonAccess rejects kids viewing or touching another child's lesson
// or any adult-only lesson.
func (a *App) enforceLessonAccess(w http.ResponseWriter, r *http.Request, lesson Lesson) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess == nil || !sess.IsKid() {
		return true
	}
	if lesson.KidID == 0 || lesson.KidID != sess.KidID {
		http.Error(w, "That page is not yours.", http.StatusForbidden)
		return false
	}
	return true
}

// requireLessonStatusWrite allows planners and caregivers for kid lessons, and
// kids for their own lessons only. Adult lessons stay planner-only.
func (a *App) requireLessonStatusWrite(w http.ResponseWriter, r *http.Request, lesson Lesson) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess == nil {
		return true
	}
	if sess.IsKid() {
		return a.enforceLessonAccess(w, r, lesson)
	}
	if sess.Role == roleCaregiver && lesson.KidID == 0 {
		http.Error(w, "That page is for grown-ups.", http.StatusForbidden)
		return false
	}
	return true
}

// enforceAttachmentAccess limits kid downloads to their own lesson/assessment/
// resource files. Curriculum PDFs stay readable (course books used in lessons).
func (a *App) enforceAttachmentAccess(w http.ResponseWriter, r *http.Request, att Attachment) bool {
	if !a.hosted {
		return true
	}
	sess := sessionFrom(r)
	if sess == nil || !sess.IsKid() {
		return true
	}
	if att.OwnerType == OwnerCurriculum {
		return true
	}
	kidID := att.KidID
	if kidID == 0 && att.LessonID != 0 {
		lesson, err := a.store.Lesson(att.LessonID)
		if err != nil {
			http.Error(w, "That page is not yours.", http.StatusForbidden)
			return false
		}
		kidID = lesson.KidID
	}
	if kidID == 0 && att.AssessmentID != 0 {
		assessment, err := a.store.Assessment(att.AssessmentID)
		if err != nil {
			http.Error(w, "That page is not yours.", http.StatusForbidden)
			return false
		}
		kidID = assessment.KidID
	}
	if kidID == 0 || kidID != sess.KidID {
		http.Error(w, "That page is not yours.", http.StatusForbidden)
		return false
	}
	return true
}

const pinFlashCookie = "sn_pin_flash"

func (a *App) setPINFlash(w http.ResponseWriter, pin, who string) {
	http.SetCookie(w, &http.Cookie{
		Name:     pinFlashCookie,
		Value:    url.QueryEscape(pin) + "|" + url.QueryEscape(who),
		Path:     "/settings",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   120,
	})
}

func (a *App) takePINFlash(w http.ResponseWriter, r *http.Request) (pin, who string) {
	c, err := r.Cookie(pinFlashCookie)
	if err != nil || c.Value == "" {
		return "", ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     pinFlashCookie,
		Value:    "",
		Path:     "/settings",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	parts := strings.SplitN(c.Value, "|", 2)
	pin, _ = url.QueryUnescape(parts[0])
	if len(parts) > 1 {
		who, _ = url.QueryUnescape(parts[1])
	}
	return pin, who
}
