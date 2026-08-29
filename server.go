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
)

// App wires the store, the data folder, and the parsed templates together.
type App struct {
	store     *Store
	dataDir   string
	uploadDir string
	pages     map[string]*template.Template
	partials  *template.Template
	secret    []byte
}

// pageNames are the full-page templates; each one defines a "content" block
// that the shared layout renders.
var pageNames = []string{
	"home", "planner", "kid", "subject", "lesson", "tests", "settings", "login",
	"attendance", "curriculum", "curriculum_plan", "curriculum_apply", "archive", "series",
}

func NewApp(store *Store, dataDir string) (*App, error) {
	app := &App{
		store:     store,
		dataDir:   dataDir,
		uploadDir: filepath.Join(dataDir, uploadsFolderName),
		pages:     map[string]*template.Template{},
	}

	for _, name := range pageNames {
		t, err := template.New(name).Funcs(templateFuncs()).ParseFS(templateFS,
			"templates/layout.html", "templates/partials.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", name, err)
		}
		app.pages[name] = t
	}

	partials, err := template.New("partials").Funcs(templateFuncs()).ParseFS(templateFS, "templates/partials.html")
	if err != nil {
		return nil, fmt.Errorf("parsing partials: %w", err)
	}
	app.partials = partials

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

	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	mux.HandleFunc("GET /login", a.handleLoginForm)
	mux.HandleFunc("POST /login", a.handleLogin)
	mux.HandleFunc("POST /logout", a.handleLogout)

	mux.HandleFunc("GET /{$}", a.handleHome)
	mux.HandleFunc("GET /planner", a.handlePlanner)

	mux.HandleFunc("GET /attendance", a.handleAttendance)
	mux.HandleFunc("POST /attendance", a.handleSaveAttendance)

	mux.HandleFunc("GET /curriculum", a.handleCurriculum)
	mux.HandleFunc("POST /curriculum", a.handleCreateCurriculumPlan)
	mux.HandleFunc("POST /curriculum/import", a.handleImportCurriculum)
	mux.HandleFunc("GET /curriculum/{id}", a.handleCurriculumPlan)
	mux.HandleFunc("POST /curriculum/{id}", a.handleUpdateCurriculumPlan)
	mux.HandleFunc("POST /curriculum/{id}/delete", a.handleDeleteCurriculumPlan)
	mux.HandleFunc("POST /curriculum/{id}/items", a.handleCreateCurriculumItem)
	mux.HandleFunc("POST /curriculum/{id}/items/{itemID}", a.handleUpdateCurriculumItem)
	mux.HandleFunc("POST /curriculum/{id}/items/{itemID}/delete", a.handleDeleteCurriculumItem)
	mux.HandleFunc("POST /curriculum/{id}/items/{itemID}/move", a.handleMoveCurriculumItem)
	mux.HandleFunc("GET /curriculum/{id}/apply", a.handleApplyCurriculumForm)
	mux.HandleFunc("POST /curriculum/{id}/apply", a.handleApplyCurriculum)

	mux.HandleFunc("GET /archive", a.handleArchive)
	mux.HandleFunc("POST /archive/export", a.handleArchiveExport)

	mux.HandleFunc("POST /lessons", a.handleCreateLesson)
	mux.HandleFunc("GET /lessons/{id}", a.handleLesson)
	mux.HandleFunc("POST /lessons/{id}", a.handleUpdateLesson)
	mux.HandleFunc("POST /lessons/{id}/status", a.handleLessonStatus)
	mux.HandleFunc("POST /lessons/{id}/reschedule", a.handleRescheduleLesson)
	mux.HandleFunc("POST /lessons/{id}/delete", a.handleDeleteLesson)
	mux.HandleFunc("POST /lessons/{id}/delete-future", a.handleDeleteSeriesFuture)

	mux.HandleFunc("GET /series/{id}", a.handleSeries)
	mux.HandleFunc("POST /series/{id}", a.handleUpdateSeries)
	mux.HandleFunc("POST /series/{id}/stop", a.handleStopSeries)

	mux.HandleFunc("GET /kids/{id}", a.handleKid)
	mux.HandleFunc("GET /kids/{id}/subjects/{subjectID}", a.handleSubject)
	mux.HandleFunc("GET /kids/{id}/tests", a.handleTests)

	mux.HandleFunc("POST /assessments", a.handleCreateAssessment)
	mux.HandleFunc("POST /assessments/{id}/delete", a.handleDeleteAssessment)

	mux.HandleFunc("POST /notes", a.handleCreateNote)
	mux.HandleFunc("POST /notes/{id}/delete", a.handleDeleteNote)

	mux.HandleFunc("POST /files", a.handleUpload)
	mux.HandleFunc("GET /files/{id}", a.handleDownload)
	mux.HandleFunc("POST /files/{id}/delete", a.handleDeleteFile)

	mux.HandleFunc("GET /settings", a.handleSettings)
	mux.HandleFunc("POST /settings/kids", a.handleSaveKid)
	mux.HandleFunc("POST /settings/kids/{id}/delete", a.handleDeleteKid)
	mux.HandleFunc("POST /settings/subjects", a.handleSaveSubject)
	mux.HandleFunc("POST /settings/subjects/{id}/delete", a.handleDeleteSubject)
	mux.HandleFunc("POST /settings/years", a.handleSaveSchoolYear)
	mux.HandleFunc("POST /settings/years/{id}/delete", a.handleDeleteSchoolYear)
	mux.HandleFunc("POST /settings/password", a.handleSavePassword)
	mux.HandleFunc("POST /settings/backups", a.handleMakeBackup)
	mux.HandleFunc("GET /settings/backups/{name}", a.handleDownloadBackup)
	mux.HandleFunc("POST /settings/backups/{name}/restore", a.handleRestoreBackup)
	mux.HandleFunc("POST /settings/backups/{name}/delete", a.handleDeleteBackup)

	return a.recoverPanic(a.sameSiteOnly(a.requireLogin(mux)))
}

// requireLogin gates the app behind the family password, when one is set.
func (a *App) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/login") {
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
				if !strings.HasSuffix(origin, "//"+r.Host) {
					http.Error(w, "cross-site requests are not allowed", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
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
// today's date, and which nav item is active.
func (a *App) pageData(active string) (map[string]any, error) {
	kids, err := a.store.Kids(false)
	if err != nil {
		return nil, err
	}
	hasPassword, err := a.store.Setting(settingPassword)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"Active":      active,
		"NavKids":     kids,
		"Today":       today(),
		"HasPassword": hasPassword != "",
	}, nil
}

func (a *App) render(w http.ResponseWriter, page string, data map[string]any) {
	t, ok := a.pages[page]
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
	var buf strings.Builder
	if err := a.partials.ExecuteTemplate(&buf, name, data); err != nil {
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
		return today()
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
