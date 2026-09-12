package main

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const (
	sessionCookie   = "school_nanny_session"
	sessionLifetime = 30 * 24 * time.Hour
	settingPassword = "family_password"
	settingSecret   = "session_secret"
	settingTimezone = "family_timezone"
)

// templateSet is every template, with its date-aware helpers fixed to one
// calendar day.
type templateSet struct {
	pages    map[string]*template.Template
	partials *template.Template
}

// App wires the store, the data folder, and the parsed templates together.
type App struct {
	store     *Store
	dataDir   string
	uploadDir string
	secret    []byte

	// Hosted multi-tenant fields. Local (desktop) mode leaves these zero.
	hosted       bool
	control      *ControlStore
	host         *App // tenant apps point at the hosted root
	dataRoot     string
	baseURL      string
	inviteCode   string
	cookieSecure bool
	tenantsMu    sync.Mutex
	tenants      map[string]*App

	// Templates are parsed per calendar date, because helpers like isToday and
	// prettyDate have to be fixed to a day and html/template will not let a
	// template be cloned once it has been executed. Households span one or two
	// dates at a time, so this parses about as often as the day changes.
	tmplMu  sync.Mutex
	tmplSet map[string]*templateSet
}

// pageNames are the full-page templates; each one defines a "content" block
// that the shared layout renders.
var pageNames = []string{
	"home", "planner", "kid", "subject", "lesson", "tests", "settings", "login", "signup",
	"attendance", "curriculum", "curriculum_plan", "curriculum_apply", "archive", "series", "assignment",
	"adult", "adult_schedule",
}

func NewApp(store *Store, dataDir string) (*App, error) {
	cfg := loadHostedConfig(dataDir)
	app := &App{
		store:        store,
		dataDir:      dataDir,
		uploadDir:    filepath.Join(dataDir, uploadsFolderName),
		tmplSet:      map[string]*templateSet{},
		baseURL:      cfg.BaseURL,
		cookieSecure: cfg.CookieSecure,
	}

	// Build one set now so a broken template is a startup error rather than a
	// surprise on the first page someone opens.
	if _, err := app.templatesFor(today()); err != nil {
		return nil, err
	}

	secret, err := store.Setting(settingSecret)
	if err != nil {
		return nil, err
	}
	if secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		secret = hex.EncodeToString(buf)
		if err := store.SetSetting(settingSecret, secret); err != nil {
			return nil, err
		}
	}
	app.secret = []byte(secret)

	return app, nil
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	h := func(fn func(*App, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			fn(appFrom(r), w, r)
		}
	}

	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	mux.HandleFunc("GET /healthz", h((*App).handleHealthz))
	mux.HandleFunc("GET /login", h((*App).handleLoginForm))
	mux.HandleFunc("POST /login", h((*App).handleLogin))
	mux.HandleFunc("GET /signup", h((*App).handleSignupForm))
	mux.HandleFunc("POST /signup", h((*App).handleSignup))
	mux.HandleFunc("POST /logout", h((*App).handleLogout))

	mux.HandleFunc("GET /{$}", h((*App).handleHome))
	mux.HandleFunc("GET /planner", h((*App).handlePlanner))

	mux.HandleFunc("GET /attendance", h((*App).handleAttendance))
	mux.HandleFunc("POST /attendance", h((*App).handleSaveAttendance))

	mux.HandleFunc("GET /curriculum", h((*App).handleCurriculum))
	mux.HandleFunc("POST /curriculum", h((*App).handleCreateCurriculumPlan))
	mux.HandleFunc("POST /curriculum/import", h((*App).handleImportCurriculum))
	mux.HandleFunc("GET /curriculum/{id}", h((*App).handleCurriculumPlan))
	mux.HandleFunc("POST /curriculum/{id}", h((*App).handleUpdateCurriculumPlan))
	mux.HandleFunc("POST /curriculum/{id}/delete", h((*App).handleDeleteCurriculumPlan))
	mux.HandleFunc("POST /curriculum/{id}/items", h((*App).handleCreateCurriculumItem))
	mux.HandleFunc("POST /curriculum/{id}/items/{itemID}", h((*App).handleUpdateCurriculumItem))
	mux.HandleFunc("POST /curriculum/{id}/items/{itemID}/delete", h((*App).handleDeleteCurriculumItem))
	mux.HandleFunc("POST /curriculum/{id}/items/{itemID}/move", h((*App).handleMoveCurriculumItem))
	mux.HandleFunc("GET /curriculum/{id}/apply", h((*App).handleApplyCurriculumForm))
	mux.HandleFunc("POST /curriculum/{id}/apply", h((*App).handleApplyCurriculum))

	mux.HandleFunc("GET /archive", h((*App).handleArchive))
	mux.HandleFunc("POST /archive/export", h((*App).handleArchiveExport))

	mux.HandleFunc("POST /lessons", h((*App).handleCreateLesson))
	mux.HandleFunc("GET /lessons/{id}", h((*App).handleLesson))
	mux.HandleFunc("POST /lessons/{id}", h((*App).handleUpdateLesson))
	mux.HandleFunc("POST /lessons/{id}/status", h((*App).handleLessonStatus))
	mux.HandleFunc("POST /lessons/{id}/reschedule", h((*App).handleRescheduleLesson))
	mux.HandleFunc("POST /lessons/{id}/clone", h((*App).handleCloneLesson))
	mux.HandleFunc("POST /lessons/{id}/delete", h((*App).handleDeleteLesson))
	mux.HandleFunc("POST /lessons/{id}/delete-future", h((*App).handleDeleteSeriesFuture))

	mux.HandleFunc("GET /series/{id}", h((*App).handleSeries))
	mux.HandleFunc("POST /series/{id}", h((*App).handleUpdateSeries))
	mux.HandleFunc("POST /series/{id}/stop", h((*App).handleStopSeries))

	mux.HandleFunc("GET /assignments/{id}", h((*App).handleAssignment))
	mux.HandleFunc("POST /assignments/{id}", h((*App).handleUpdateAssignment))
	mux.HandleFunc("POST /assignments/{id}/stop", h((*App).handleStopAssignment))
	mux.HandleFunc("POST /assignments/{id}/pause", h((*App).handlePauseAssignment))
	mux.HandleFunc("POST /lessons/{id}/push", h((*App).handlePushLesson))
	mux.HandleFunc("POST /lessons/{id}/pull", h((*App).handlePullLesson))

	mux.HandleFunc("GET /adults/{id}", h((*App).handleAdult))
	mux.HandleFunc("GET /adults/{id}/schedule", h((*App).handleAdultSchedule))
	mux.HandleFunc("POST /adults/{id}/schedule", h((*App).handleCreateAdultLesson))
	mux.HandleFunc("GET /adults/{id}/calendar", h((*App).handleAdultCalendar))
	mux.HandleFunc("POST /adults/{id}/events", h((*App).handleCreateAdultEvent))
	mux.HandleFunc("POST /adults/{id}/events/{eventID}", h((*App).handleUpdateAdultEvent))
	mux.HandleFunc("POST /adults/{id}/events/{eventID}/label", h((*App).handleSetAdultEventLabel))
	mux.HandleFunc("POST /adults/{id}/events/{eventID}/delete", h((*App).handleDeleteAdultEvent))
	mux.HandleFunc("POST /adults/{id}/labels", h((*App).handleCreateAdultEventLabel))
	mux.HandleFunc("POST /adults/{id}/labels/{labelID}", h((*App).handleUpdateAdultEventLabel))
	mux.HandleFunc("POST /adults/{id}/labels/{labelID}/delete", h((*App).handleDeleteAdultEventLabel))
	mux.HandleFunc("POST /adults/{id}/holidays", h((*App).handleUpsertHolidayNote))
	mux.HandleFunc("POST /adults/{id}/cards", h((*App).handleCreateAdultCard))
	mux.HandleFunc("POST /adults/{id}/cards/{cardID}", h((*App).handleUpdateAdultCard))
	mux.HandleFunc("POST /adults/{id}/cards/{cardID}/move", h((*App).handleMoveAdultCard))
	mux.HandleFunc("POST /adults/{id}/cards/{cardID}/delete", h((*App).handleDeleteAdultCard))

	mux.HandleFunc("GET /kids/{id}", h((*App).handleKid))
	mux.HandleFunc("GET /kids/{id}/subjects/{subjectID}", h((*App).handleSubject))
	mux.HandleFunc("GET /kids/{id}/tests", h((*App).handleTests))

	mux.HandleFunc("POST /assessments", h((*App).handleCreateAssessment))
	mux.HandleFunc("POST /assessments/{id}/delete", h((*App).handleDeleteAssessment))

	mux.HandleFunc("POST /notes", h((*App).handleCreateNote))
	mux.HandleFunc("POST /notes/{id}/delete", h((*App).handleDeleteNote))

	mux.HandleFunc("GET /avatars/kids/{id}", h((*App).handleKidAvatarImage))
	mux.HandleFunc("GET /avatars/adults/{id}", h((*App).handleAdultAvatarImage))

	mux.HandleFunc("POST /files", h((*App).handleUpload))
	mux.HandleFunc("GET /files/{id}", h((*App).handleDownload))
	mux.HandleFunc("POST /files/{id}/delete", h((*App).handleDeleteFile))

	mux.HandleFunc("GET /settings", h((*App).handleSettings))
	mux.HandleFunc("POST /settings/kids", h((*App).handleSaveKid))
	mux.HandleFunc("POST /settings/kids/{id}/delete", h((*App).handleDeleteKid))
	mux.HandleFunc("POST /settings/kids/{id}/avatar", h((*App).handleKidAvatarUpload))
	mux.HandleFunc("POST /settings/kids/{id}/avatar/delete", h((*App).handleKidAvatarDelete))
	mux.HandleFunc("POST /settings/adults", h((*App).handleSaveAdult))
	mux.HandleFunc("POST /settings/adults/{id}/avatar", h((*App).handleAdultAvatarUpload))
	mux.HandleFunc("POST /settings/adults/{id}/avatar/delete", h((*App).handleAdultAvatarDelete))
	mux.HandleFunc("POST /settings/subjects", h((*App).handleSaveSubject))
	mux.HandleFunc("POST /settings/subjects/{id}/delete", h((*App).handleDeleteSubject))
	mux.HandleFunc("POST /settings/years", h((*App).handleSaveSchoolYear))
	mux.HandleFunc("POST /settings/years/{id}/delete", h((*App).handleDeleteSchoolYear))
	mux.HandleFunc("POST /settings/password", h((*App).handleSavePassword))
	mux.HandleFunc("POST /settings/account-password", h((*App).handleChangeAccountPassword))
	mux.HandleFunc("POST /settings/timezone", h((*App).handleSaveTimezone))
	mux.HandleFunc("POST /settings/backups", h((*App).handleMakeBackup))
	mux.HandleFunc("GET /settings/backups/{name}", h((*App).handleDownloadBackup))
	mux.HandleFunc("POST /settings/backups/{name}/restore", h((*App).handleRestoreBackup))
	mux.HandleFunc("POST /settings/backups/{name}/delete", h((*App).handleDeleteBackup))
	mux.HandleFunc("GET /settings/export", h((*App).handleFamilyExport))
	mux.HandleFunc("POST /settings/import", h((*App).handleFamilyImport))

	stack := a.withTimezone(mux)
	if a.hosted {
		stack = a.hostedGate(stack)
	} else {
		stack = a.bindApp(a.requireLogin(stack))
	}
	return a.recoverPanic(a.sameSiteOnly(stack))
}

// withTimezone hangs the reader's calendar zone on the request so every
// handler and template for that request answers "today" the same way.
func (a *App) withTimezone(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		app := a
		if v, ok := r.Context().Value(ctxKeyApp).(*App); ok && v != nil {
			app = v
		}
		family := ""
		if app.store != nil {
			var err error
			family, err = app.store.Setting(settingTimezone)
			if err != nil {
				family = ""
			}
		}
		next.ServeHTTP(w, r.WithContext(withLocation(r.Context(), resolveLocation(family, cookieTZ(r)))))
	})
}

// requireLogin gates the app behind the family password, when one is set.
// Hosted mode uses hostedGate instead.
func (a *App) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") ||
			strings.HasPrefix(r.URL.Path, "/login") ||
			r.URL.Path == "/healthz" ||
			r.URL.Path == "/signup" {
			next.ServeHTTP(w, r)
			return
		}
		hash, err := a.store.Setting(settingPassword)
		if err != nil {
			a.serverError(w, err)
			return
		}
		if hash == "" || a.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", "/login")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
}

// sameSiteOnly rejects writes initiated by another site. The app has no
// cross-site callers, so this is a cheap stand-in for CSRF tokens.
func (a *App) sameSiteOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			switch r.Header.Get("Sec-Fetch-Site") {
			case "", "same-origin", "none":
			default:
				http.Error(w, "cross-site requests are not allowed", http.StatusForbidden)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				if !a.originAllowed(origin, r) {
					http.Error(w, "cross-site requests are not allowed", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) originAllowed(origin string, r *http.Request) bool {
	if strings.HasSuffix(origin, "//"+r.Host) {
		return true
	}
	if a.baseURL != "" && origin == a.baseURL {
		return true
	}
	return false
}

func (a *App) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.serverError(w, fmt.Errorf("panic: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// pageData assembles what every page needs: the child list for navigation,
// today's date in the reader's zone, and which nav item is active.
func (a *App) pageData(r *http.Request, active string) (map[string]any, error) {
	kids, err := a.store.Kids(false)
	if err != nil {
		return nil, err
	}
	adults, err := a.store.Adults(false)
	if err != nil {
		return nil, err
	}
	hasPassword, err := a.store.Setting(settingPassword)
	if err != nil {
		return nil, err
	}
	family, err := a.store.Setting(settingTimezone)
	if err != nil {
		return nil, err
	}
	familyListed := false
	for _, z := range commonTimezones {
		if z == family {
			familyListed = true
			break
		}
	}
	data := map[string]any{
		"Active":               active,
		"NavKids":              kids,
		"NavAdults":            adults,
		"Today":                requestToday(r),
		"HasPassword":          hasPassword != "",
		"FamilyTimezone":       family,
		"FamilyTimezoneListed": familyListed,
		"DeviceTimezone":       cookieTZ(r),
		"CommonTimezones":      commonTimezones,
		"Hosted":               a.hosted,
	}
	if a.hosted {
		if sess := sessionFrom(r); sess != nil {
			if u, err := a.control.User(sess.UserID); err == nil {
				data["AccountEmail"] = u.Email
			}
		}
	}
	return data, nil
}

// templatesFor returns the parsed templates whose date helpers are fixed to
// one calendar day. html/template will not let a template be cloned once it
// has been executed, so each distinct "today" gets its own set.
func (a *App) templatesFor(day string) (*templateSet, error) {
	if a.host != nil {
		return a.host.templatesFor(day)
	}
	if day == "" {
		day = today()
	}
	a.tmplMu.Lock()
	defer a.tmplMu.Unlock()
	if set, ok := a.tmplSet[day]; ok {
		return set, nil
	}
	set, err := parseTemplateSet(day)
	if err != nil {
		return nil, err
	}
	// Households live on one or two dates at a time. Drop an older set rather
	// than keep every day someone once travelled through.
	if len(a.tmplSet) >= 8 {
		for k := range a.tmplSet {
			if k != day {
				delete(a.tmplSet, k)
				break
			}
		}
	}
	a.tmplSet[day] = set
	return set, nil
}

func parseTemplateSet(day string) (*templateSet, error) {
	set := &templateSet{pages: map[string]*template.Template{}}
	for _, name := range pageNames {
		t, err := template.New(name).Funcs(templateFuncs(day)).ParseFS(templateFS,
			"templates/layout.html", "templates/partials.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", name, err)
		}
		set.pages[name] = t
	}
	partials, err := template.New("partials").Funcs(templateFuncs(day)).ParseFS(templateFS, "templates/partials.html")
	if err != nil {
		return nil, fmt.Errorf("parsing partials: %w", err)
	}
	set.partials = partials
	return set, nil
}

func todayFromData(data map[string]any) string {
	if data != nil {
		if day, ok := data["Today"].(string); ok && day != "" {
			return day
		}
	}
	return today()
}

func (a *App) render(w http.ResponseWriter, page string, data map[string]any) {
	set, err := a.templatesFor(todayFromData(data))
	if err != nil {
		a.serverError(w, err)
		return
	}
	t, ok := set.pages[page]
	if !ok {
		a.serverError(w, fmt.Errorf("unknown page template %q", page))
		return
	}
	var buf strings.Builder
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		a.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

func (a *App) renderPartial(w http.ResponseWriter, name string, data any) {
	day := today()
	if m, ok := data.(map[string]any); ok {
		day = todayFromData(m)
	}
	set, err := a.templatesFor(day)
	if err != nil {
		a.serverError(w, err)
		return
	}
	var buf strings.Builder
	if err := set.partials.ExecuteTemplate(&buf, name, data); err != nil {
		a.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

func (a *App) serverError(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	http.Error(w, "Something went wrong. Check the terminal window for details.", http.StatusInternalServerError)
}

func (a *App) notFound(w http.ResponseWriter) {
	http.Error(w, "Not found", http.StatusNotFound)
}

// redirect sends the browser onward, using HTMX's redirect header when the
// request came from HTMX so the whole page swaps rather than a fragment.
func (a *App) redirect(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func pathID(r *http.Request, name string) int64 {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func parseInt64(raw string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// safeRedirect keeps "where to go back to" form fields pointed at this app.
func safeRedirect(raw, fallback string) string {
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	return fallback
}

func formID(r *http.Request, name string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue(name)), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func formInt(r *http.Request, name string) int {
	n, err := strconv.Atoi(strings.TrimSpace(r.FormValue(name)))
	if err != nil {
		return 0
	}
	return n
}

// formFloat returns nil when the field was left blank, which is how an
// unscored test stays unscored.
func formFloat(r *http.Request, name string) *float64 {
	raw := strings.TrimSpace(r.FormValue(name))
	if raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &f
}

func formDate(r *http.Request, name string) string {
	raw := strings.TrimSpace(r.FormValue(name))
	if _, err := time.Parse(dateLayout, raw); err != nil {
		return requestToday(r)
	}
	return raw
}

func formDateOrEmpty(r *http.Request, name string) string {
	raw := strings.TrimSpace(r.FormValue(name))
	if _, err := time.Parse(dateLayout, raw); err != nil {
		return ""
	}
	return raw
}

// Session handling -----------------------------------------------------------

func (a *App) issueSession(w http.ResponseWriter) {
	expiry := time.Now().Add(sessionLifetime).Unix()
	payload := strconv.FormatInt(expiry, 10)
	value := payload + "." + a.sign(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expiry, 0),
	})
}

func (a *App) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (a *App) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	payload, mac, ok := strings.Cut(cookie.Value, ".")
	if !ok || !hmac.Equal([]byte(mac), []byte(a.sign(payload))) {
		return false
	}
	expiry, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < expiry
}

func (a *App) sign(payload string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// Password hashing -----------------------------------------------------------

const pbkdf2Iterations = 200_000

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pbkdf2Iterations,
		hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

func checkPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := deriveKey(password, salt)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

var errEmptyPassword = errors.New("password must not be empty")

func deriveKey(password string, salt []byte) ([]byte, error) {
	if password == "" {
		return nil, errEmptyPassword
	}
	return pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, 32)
}
