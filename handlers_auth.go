package main

import "net/http"

func (a *App) handleLoginForm(w http.ResponseWriter, r *http.Request) {
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

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
