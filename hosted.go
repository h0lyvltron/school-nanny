package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ctxKey int

const (
	ctxKeyApp ctxKey = iota + 1
	ctxKeySession
)

// HostedConfig is the env-driven hosted mode settings.
type HostedConfig struct {
	DataRoot     string
	BaseURL      string
	InviteCode   string
	CookieSecure bool
}

func hostedModeEnabled() bool {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SCHOOL_NANNY_MODE")))
	return mode == "hosted"
}

func loadHostedConfig(dataRoot string) HostedConfig {
	base := strings.TrimSpace(os.Getenv("BASE_URL"))
	base = strings.TrimRight(base, "/")
	secure := strings.HasPrefix(strings.ToLower(base), "https://")
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE"))); v == "1" || v == "true" {
		secure = true
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE"))); v == "0" || v == "false" {
		secure = false
	}
	return HostedConfig{
		DataRoot:     dataRoot,
		BaseURL:      base,
		InviteCode:   strings.TrimSpace(os.Getenv("INVITE_CODE")),
		CookieSecure: secure,
	}
}

func withApp(ctx context.Context, a *App) context.Context {
	return context.WithValue(ctx, ctxKeyApp, a)
}

func appFrom(r *http.Request) *App {
	if a, ok := r.Context().Value(ctxKeyApp).(*App); ok && a != nil {
		return a
	}
	panic("school-nanny: missing app in request context")
}

func withSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

func sessionFrom(r *http.Request) *Session {
	s, _ := r.Context().Value(ctxKeySession).(*Session)
	return s
}

// NewHostedApp opens the control plane and prepares tenant caching. Family DBs
// open lazily on first authenticated request.
func NewHostedApp(cfg HostedConfig) (*App, error) {
	if err := os.MkdirAll(cfg.DataRoot, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataRoot, familiesDirName), 0o755); err != nil {
		return nil, err
	}
	control, err := OpenControlStore(filepath.Join(cfg.DataRoot, controlDBName))
	if err != nil {
		return nil, err
	}
	app := &App{
		dataDir:      cfg.DataRoot,
		uploadDir:    "",
		tmplSet:      map[string]*templateSet{},
		hosted:       true,
		control:      control,
		dataRoot:     cfg.DataRoot,
		baseURL:      cfg.BaseURL,
		inviteCode:   cfg.InviteCode,
		cookieSecure: cfg.CookieSecure,
		tenants:      map[string]*App{},
	}
	if _, err := app.templatesFor(today()); err != nil {
		control.Close()
		return nil, err
	}
	return app, nil
}

func (a *App) closeTenants() {
	a.tenantsMu.Lock()
	defer a.tenantsMu.Unlock()
	for id, t := range a.tenants {
		if t.store != nil {
			_ = t.store.Close()
		}
		delete(a.tenants, id)
	}
	if a.control != nil {
		_ = a.control.Close()
	}
}

// openFamilyDir ensures the family folder, DB, and uploads exist and returns a ready App.
func (a *App) tenantApp(familyID string) (*App, error) {
	if familyID == "" {
		return nil, fmt.Errorf("empty family id")
	}
	a.tenantsMu.Lock()
	defer a.tenantsMu.Unlock()
	if t, ok := a.tenants[familyID]; ok {
		return t, nil
	}

	dir := familyDataDir(a.dataRoot, familyID)
	if err := os.MkdirAll(filepath.Join(dir, uploadsFolderName), 0o755); err != nil {
		return nil, err
	}
	store, err := OpenStore(filepath.Join(dir, dbFileName))
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(); err != nil {
		store.Close()
		return nil, err
	}
	tenant := &App{
		store:        store,
		dataDir:      dir,
		uploadDir:    filepath.Join(dir, uploadsFolderName),
		tmplSet:      a.tmplSet,
		hosted:       true,
		control:      a.control,
		host:         a,
		dataRoot:     a.dataRoot,
		baseURL:      a.baseURL,
		inviteCode:   a.inviteCode,
		cookieSecure: a.cookieSecure,
	}
	a.tenants[familyID] = tenant
	return tenant, nil
}

// adoptLegacyData moves a Phase-A single-tenant school.db + uploads into a new
// family directory when that family is the first signup and legacy files exist.
func (a *App) adoptLegacyData(familyID string) error {
	legacyDB := filepath.Join(a.dataRoot, dbFileName)
	if _, err := os.Stat(legacyDB); err != nil {
		return nil
	}
	dest := familyDataDir(a.dataRoot, familyID)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	destDB := filepath.Join(dest, dbFileName)
	if _, err := os.Stat(destDB); err == nil {
		return nil
	}
	log.Printf("hosted: adopting legacy %s into family %s", legacyDB, familyID)
	if err := os.Rename(legacyDB, destDB); err != nil {
		return err
	}
	for _, side := range []string{legacyDB + "-wal", legacyDB + "-shm"} {
		if _, err := os.Stat(side); err == nil {
			_ = os.Rename(side, filepath.Join(dest, filepath.Base(side)))
		}
	}
	legacyUploads := filepath.Join(a.dataRoot, uploadsFolderName)
	destUploads := filepath.Join(dest, uploadsFolderName)
	if info, err := os.Stat(legacyUploads); err == nil && info.IsDir() {
		if _, err := os.Stat(destUploads); err != nil {
			if err := os.Rename(legacyUploads, destUploads); err != nil {
				return err
			}
		}
	}
	legacyBackups := filepath.Join(a.dataRoot, backupsFolderName)
	destBackups := filepath.Join(dest, backupsFolderName)
	if info, err := os.Stat(legacyBackups); err == nil && info.IsDir() {
		if _, err := os.Stat(destBackups); err != nil {
			_ = os.Rename(legacyBackups, destBackups)
		}
	}
	return nil
}

func (a *App) issueHostedSession(w http.ResponseWriter, sess *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     hostedSIDCookie,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
		MaxAge:   int(time.Until(sess.ExpiresAt).Seconds()),
	})
}

func (a *App) clearHostedSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     hostedSIDCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (a *App) hostedSessionFromRequest(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(hostedSIDCookie)
	if err != nil || cookie.Value == "" {
		return nil, errNoSession
	}
	return a.control.Session(cookie.Value)
}

func isHostedPublicPath(path string) bool {
	if strings.HasPrefix(path, "/static/") {
		return true
	}
	switch path {
	case "/login", "/signup", "/healthz":
		return true
	}
	return false
}

func (a *App) bindApp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(withApp(r.Context(), a)))
	})
}

func (a *App) hostedGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isHostedPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r.WithContext(withApp(r.Context(), a)))
			return
		}
		sess, err := a.hostedSessionFromRequest(r)
		if err != nil {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		tenant, err := a.tenantApp(sess.FamilyID)
		if err != nil {
			a.serverError(w, err)
			return
		}
		ctx := withApp(r.Context(), tenant)
		ctx = withSession(ctx, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
