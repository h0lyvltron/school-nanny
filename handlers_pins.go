package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func (a *App) canIssuePIN(sess *Session, role string) bool {
	if sess == nil {
		return false
	}
	if sess.IsOwner() {
		return true
	}
	return sess.CanManageKidLogins && role == roleKid
}

func (a *App) handleCreatePIN(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	sess := sessionFrom(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	role := r.FormValue("role")
	if !a.canIssuePIN(sess, role) {
		http.Error(w, "Only the family owner can do that.", http.StatusForbidden)
		return
	}
	username := r.FormValue("username")
	pin := r.FormValue("pin")
	display := r.FormValue("display_name")
	kidID, _ := strconv.ParseInt(r.FormValue("kid_id"), 10, 64)
	canManage := r.FormValue("can_manage_kid_logins") == "1" && sess.IsOwner()
	mem, err := a.control.IssuePIN(sess.FamilyID, sess.AccountID, username, pin, role, display, kidID, 0, canManage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.redirect(w, r, fmt.Sprintf("/settings?saved=pin-created&pin=%s&who=%s",
		url.QueryEscape(pin), url.QueryEscape(mem.DisplayName)))
}

func (a *App) handleResetPIN(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	sess := sessionFrom(r)
	if sess == nil || (!sess.IsOwner() && !sess.CanManageKidLogins) {
		http.Error(w, "Only the family owner can do that.", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read that form.", http.StatusBadRequest)
		return
	}
	pin := r.FormValue("pin")
	id := pathID(r, "id")
	if !sess.IsOwner() {
		members, err := a.control.ListMemberships(sess.FamilyID)
		if err != nil {
			a.serverError(w, err)
			return
		}
		ok := false
		for _, m := range members {
			if m.ID == id && m.Role == roleKid {
				ok = true
				break
			}
		}
		if !ok {
			http.Error(w, "Only the family owner can do that.", http.StatusForbidden)
			return
		}
	}
	if err := a.control.ResetPIN(id, sess.FamilyID, pin); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.redirect(w, r, "/settings?saved=pin-reset&pin="+url.QueryEscape(pin))
}

func (a *App) handleRevokePIN(w http.ResponseWriter, r *http.Request) {
	if !a.hosted {
		http.NotFound(w, r)
		return
	}
	if !a.requireOwner(w, r) {
		return
	}
	sess := sessionFrom(r)
	id := pathID(r, "id")
	if err := a.control.RevokeMembership(id, sess.FamilyID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.redirect(w, r, "/settings?saved=pin-revoked")
}
