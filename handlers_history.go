package main

import (
	"errors"
	"net/http"
)

func (a *App) handleTrashRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/history", http.StatusMovedPermanently)
}

func (a *App) handleHistory(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	now := requestNow(r)
	if err := a.purgeExpiredLessonTrash(now); err != nil {
		a.serverError(w, err)
		return
	}
	roots, pos, err := a.store.HistoryTree()
	if err != nil {
		a.serverError(w, err)
		return
	}
	deleted, err := a.store.DeletedLessons(now)
	if err != nil {
		a.serverError(w, err)
		return
	}
	data, err := a.pageData(r, "history")
	if err != nil {
		a.serverError(w, err)
		return
	}
	data["HistoryNodes"] = historyFlat(roots)
	data["History"] = pos
	data["DeletedLessons"] = deleted
	data["Location"] = requestLocation(r)
	a.render(w, "history", data)
}

func (a *App) handleHistoryUndo(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	err := a.store.UndoAt(parseExpected(r), parseExpectedRevision(r))
	a.finishHistoryNavigation(w, r, err)
}

func (a *App) handleHistoryRedo(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	err := a.store.RedoAt(parseExpected(r), parseExpectedRevision(r))
	a.finishHistoryNavigation(w, r, err)
}

func (a *App) handleHistoryCheckout(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	err := a.store.CheckoutAt(parseExpected(r), parseExpectedRevision(r), pathID(r, "id"))
	a.finishHistoryNavigation(w, r, err)
}

func (a *App) finishHistoryNavigation(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errHistoryConflict):
		http.Error(w, "History changed in another tab. Reload and try again.", http.StatusConflict)
		return
	case errors.Is(err, errNoUndo), errors.Is(err, errNoRedo):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/history"))
}
