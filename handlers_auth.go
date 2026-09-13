package main

import (
	"errors"
	"net/http"
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
	a.redirect(w, r, "/settings?saved=account-password")
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

// requirePlanningAccess blocks kids and caregivers from curriculum/attendance/archive/settings.
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
