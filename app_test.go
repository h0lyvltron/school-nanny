package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func itoa64(id int64) string {
	return strconv.FormatInt(id, 10)
}

func readFileIfExists(path string) ([]byte, error) {
	return os.ReadFile(path)
}

type testApp struct {
	*App
	server *httptest.Server
	client *http.Client
	t      *testing.T
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()

	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, dbFileName))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	app, err := NewApp(store, dir)
	if err != nil {
		t.Fatalf("building app: %v", err)
	}

	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("building cookie jar: %v", err)
	}

	return &testApp{
		App:    app,
		server: server,
		client: &http.Client{Jar: jar},
		t:      t,
	}
}

func (ta *testApp) get(path string) (int, string) {
	ta.t.Helper()
	resp, err := ta.client.Get(ta.server.URL + path)
	if err != nil {
		ta.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func (ta *testApp) postFile(path, field, filename string, content []byte) (int, string) {
	ta.t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		ta.t.Fatalf("building upload: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		ta.t.Fatalf("writing upload: %v", err)
	}
	if err := writer.Close(); err != nil {
		ta.t.Fatalf("closing upload: %v", err)
	}
	resp, err := ta.client.Post(ta.server.URL+path, writer.FormDataContentType(), &buf)
	if err != nil {
		ta.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func (ta *testApp) post(path string, form url.Values) (int, string) {
	ta.t.Helper()
	resp, err := ta.client.PostForm(ta.server.URL+path, form)
	if err != nil {
		ta.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// postHTMX mimics a button in the page rather than a full form submit, so the
// handler answers with a fragment instead of a redirect.
func (ta *testApp) postHTMX(path string, form url.Values) (int, string) {
	ta.t.Helper()
	req, err := http.NewRequest(http.MethodPost, ta.server.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		ta.t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	resp, err := ta.client.Do(req)
	if err != nil {
		ta.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func (ta *testApp) addKid(name string) int64 {
	ta.t.Helper()
	if _, err := ta.store.CreateKid(name, "3rd", "#5b8def"); err != nil {
		ta.t.Fatalf("creating kid: %v", err)
	}
	kids, err := ta.store.Kids(false)
	if err != nil {
		ta.t.Fatalf("listing kids: %v", err)
	}
	for _, kid := range kids {
		if kid.Name == name {
			return kid.ID
		}
	}
	ta.t.Fatalf("kid %s not found after creating", name)
	return 0
}

func (ta *testApp) mathSubjectID() int64 {
	ta.t.Helper()
	subjects, err := ta.store.Subjects(false)
	if err != nil {
		ta.t.Fatalf("listing subjects: %v", err)
	}
	for _, s := range subjects {
		if s.Slug == "math" {
			return s.ID
		}
	}
	ta.t.Fatal("seeded math subject is missing")
	return 0
}

func mustContain(t *testing.T, body, want, what string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("%s: expected page to contain %q", what, want)
	}
}

func mustNotContain(t *testing.T, body, unwanted, what string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Errorf("%s: expected page not to contain %q", what, unwanted)
	}
}

func TestSubjectsAreSeeded(t *testing.T) {
	ta := newTestApp(t)
	subjects, err := ta.store.Subjects(false)
	if err != nil {
		t.Fatalf("listing subjects: %v", err)
	}
	want := []string{"Math", "Language Arts", "Science", "Social Studies",
		"History", "Japanese", "Music & Art", "Other/Elective"}
	if len(subjects) != len(want) {
		t.Fatalf("expected %d seeded subjects, got %d", len(want), len(subjects))
	}
	for i, name := range want {
		if subjects[i].Name != name {
			t.Errorf("subject %d: expected %q, got %q", i, name, subjects[i].Name)
		}
	}
}

// The theme is applied by a script in the head, so a page that renders without
// it flashes the wrong colours or ignores the saved choice entirely. go:embed
// silently skips files it does not match, so check the asset is really served.
func TestThemeSwitchIsWiredUp(t *testing.T) {
	ta := newTestApp(t)

	_, body := ta.get("/")
	mustContain(t, body, `data-theme-toggle`, "theme button on home")
	mustContain(t, body, "school-nanny-theme", "inline theme script")
	mustContain(t, body, `src="/static/theme.js"`, "theme script tag")

	status, script := ta.get("/static/theme.js")
	if status != http.StatusOK {
		t.Fatalf("theme.js returned %d, so it is missing from the binary", status)
	}
	mustContain(t, script, "prefers-color-scheme", "theme.js")

	_, css := ta.get("/static/app.css")
	mustContain(t, css, `[data-theme="dark"]`, "dark palette in app.css")
}

func TestSaveToastIsWiredUp(t *testing.T) {
	ta := newTestApp(t)

	_, body := ta.get("/")
	mustContain(t, body, `src="/static/ui.js"`, "ui script tag")

	status, script := ta.get("/static/ui.js")
	if status != http.StatusOK {
		t.Fatalf("ui.js returned %d, so it is missing from the binary", status)
	}
	mustContain(t, script, "school-nanny-scroll", "ui.js")
	mustContain(t, script, "Saved!", "ui.js default toast")
	mustContain(t, script, "scrollRestoration", "ui.js")

	_, css := ta.get("/static/app.css")
	mustContain(t, css, ".save-toast", "toast styles")
}

func TestFirstRunAsksForKids(t *testing.T) {
	ta := newTestApp(t)

	status, body := ta.get("/")
	if status != http.StatusOK {
		t.Fatalf("home returned %d", status)
	}
	mustContain(t, body, "Welcome to School Nanny", "first run home")

	ta.addKid("Mia")
	_, body = ta.get("/")
	mustNotContain(t, body, "Welcome to School Nanny", "home after adding a kid")
	mustContain(t, body, "Mia", "home after adding a kid")
}

func TestPlanThenCompleteLesson(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	tomorrow := addDays(today(), 1)

	status, _ := ta.post("/lessons", url.Values{
		"kid_id":       {itoa64(kid)},
		"subject_id":   {itoa64(subject)},
		"scheduled_on": {tomorrow},
		"title":        {"Long division"},
		"minutes":      {"45"},
		"back":         {"/planner"},
	})
	if status != http.StatusOK {
		t.Fatalf("creating a lesson returned %d", status)
	}

	_, body := ta.get("/planner")
	mustContain(t, body, "Long division", "planner")

	lessons, err := ta.store.LessonsBetween(tomorrow, tomorrow, kid)
	if err != nil || len(lessons) != 1 {
		t.Fatalf("expected 1 lesson, got %d (err %v)", len(lessons), err)
	}
	lesson := lessons[0]
	if lesson.Status != StatusPlanned {
		t.Errorf("new lesson should start planned, got %q", lesson.Status)
	}

	// Marking done from the page swaps a single row back in.
	status, fragment := ta.postHTMX("/lessons/"+itoa64(lesson.ID)+"/status", url.Values{
		"status": {StatusDone},
	})
	if status != http.StatusOK {
		t.Fatalf("marking done returned %d", status)
	}
	mustContain(t, fragment, "status-done", "status fragment")
	mustNotContain(t, fragment, "<!doctype html>", "status fragment")

	lesson, err = ta.store.Lesson(lesson.ID)
	if err != nil {
		t.Fatalf("reloading lesson: %v", err)
	}
	if !lesson.IsDone() {
		t.Error("lesson should be done")
	}
	if lesson.CompletedAt == "" {
		t.Error("finishing a lesson should stamp completed_at")
	}

	// Undoing clears the stamp again.
	ta.postHTMX("/lessons/"+itoa64(lesson.ID)+"/status", url.Values{"status": {StatusPlanned}})
	lesson, _ = ta.store.Lesson(lesson.ID)
	if lesson.CompletedAt != "" {
		t.Error("undoing should clear completed_at")
	}
}

func TestLogWorkThatAlreadyHappened(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Noah")
	subject := ta.mathSubjectID()

	ta.post("/lessons", url.Values{
		"kid_id":       {itoa64(kid)},
		"subject_id":   {itoa64(subject)},
		"scheduled_on": {today()},
		"title":        {"Times tables"},
		"status":       {StatusDone},
		"minutes":      {"20"},
		"back":         {"/"},
	})

	lessons, err := ta.store.LessonsBetween(today(), today(), kid)
	if err != nil || len(lessons) != 1 {
		t.Fatalf("expected 1 lesson, got %d (err %v)", len(lessons), err)
	}
	if !lessons[0].IsDone() {
		t.Error("logging past work should record it as done")
	}

	progress, err := ta.store.ProgressBetween(today(), today(), kid, 0)
	if err != nil {
		t.Fatalf("reading progress: %v", err)
	}
	if progress.Done != 1 || progress.Minutes != 20 {
		t.Errorf("expected 1 done and 20 minutes, got %d and %d", progress.Done, progress.Minutes)
	}
}

func TestOverdueLessonsSurfaceOnHome(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Ivy")
	subject := ta.mathSubjectID()
	yesterday := addDays(today(), -1)

	if _, err := ta.store.CreateLesson(Lesson{
		KidID:       kid,
		SubjectID:   subject,
		ScheduledOn: yesterday,
		Title:       "Missed spelling",
		Status:      StatusPlanned,
	}); err != nil {
		t.Fatalf("creating lesson: %v", err)
	}

	_, body := ta.get("/")
	mustContain(t, body, "Still open from earlier days", "home")
	mustContain(t, body, "Missed spelling", "home")
}

func TestPlannerAddSwapsOnlyThatDay(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	status, fragment := ta.postHTMX("/lessons", url.Values{
		"view":         {"planner"},
		"kid_id":       {itoa64(kid)},
		"subject_id":   {itoa64(subject)},
		"scheduled_on": {today()},
		"title":        {"Extra drill"},
		"kid_filter":   {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("planner add returned %d", status)
	}
	mustContain(t, fragment, `id="day-`+today()+`"`, "planner day fragment")
	mustContain(t, fragment, "Extra drill", "planner day fragment")
	mustNotContain(t, fragment, "<!doctype html>", "planner day fragment")
}

func TestFileRoundTrip(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	back := "/kids/" + itoa64(kid) + "/subjects/" + itoa64(subject)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("owner_type", OwnerResource)
	writer.WriteField("kid_id", itoa64(kid))
	writer.WriteField("subject_id", itoa64(subject))
	writer.WriteField("back", back)
	part, err := writer.CreateFormFile("file", "fractions worksheet.pdf")
	if err != nil {
		t.Fatalf("building upload: %v", err)
	}
	part.Write([]byte("pretend pdf bytes"))
	writer.Close()

	resp, err := ta.client.Post(ta.server.URL+"/files", writer.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	resp.Body.Close()

	files, err := ta.store.ResourceAttachments(kid, subject)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected 1 stored file, got %d (err %v)", len(files), err)
	}
	stored := files[0]
	if stored.OriginalName != "fractions worksheet.pdf" {
		t.Errorf("original name should be preserved, got %q", stored.OriginalName)
	}
	// The name on disk is sanitised even though the display name is not.
	if strings.Contains(stored.StoredPath, " ") {
		t.Errorf("stored path should not contain spaces, got %q", stored.StoredPath)
	}

	status, body := ta.get("/files/" + itoa64(stored.ID))
	if status != http.StatusOK {
		t.Fatalf("downloading returned %d", status)
	}
	if body != "pretend pdf bytes" {
		t.Errorf("downloaded content did not match, got %q", body)
	}

	_, page := ta.get(back)
	mustContain(t, page, "fractions worksheet.pdf", "subject page")

	ta.post("/files/"+itoa64(stored.ID)+"/delete", url.Values{"back": {back}})
	files, _ = ta.store.ResourceAttachments(kid, subject)
	if len(files) != 0 {
		t.Errorf("expected the file to be gone, %d remain", len(files))
	}
	if path, ok := ta.resolveUpload(stored.StoredPath); ok {
		if _, err := readFileIfExists(path); err == nil {
			t.Error("deleting an attachment should remove it from disk too")
		}
	}
}

func TestUploadCannotEscapeTheDataFolder(t *testing.T) {
	ta := newTestApp(t)
	if _, ok := ta.resolveUpload("../../etc/passwd"); ok {
		t.Error("a traversal path should be refused")
	}
	if _, ok := ta.resolveUpload("/etc/passwd"); ok {
		t.Error("an absolute path should be refused")
	}
	if _, ok := ta.resolveUpload("2026/08/file.pdf"); !ok {
		t.Error("an ordinary stored path should be accepted")
	}
}

func TestRecordTestAndSeeTheScore(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	ta.post("/assessments", url.Values{
		"kid_id":     {itoa64(kid)},
		"subject_id": {itoa64(subject)},
		"given_on":   {today()},
		"name":       {"Chapter 3 test"},
		"score":      {"18"},
		"max_score":  {"20"},
		"back":       {"/kids/" + itoa64(kid) + "/tests"},
	})

	_, body := ta.get("/kids/" + itoa64(kid) + "/tests")
	mustContain(t, body, "Chapter 3 test", "tests page")
	mustContain(t, body, "90%", "tests page")

	_, body = ta.get("/kids/" + itoa64(kid))
	mustContain(t, body, "18 / 20 (90%)", "kid dashboard")
}

func TestUnscoredTestIsAllowed(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	ta.post("/assessments", url.Values{
		"kid_id":     {itoa64(kid)},
		"subject_id": {itoa64(subject)},
		"given_on":   {today()},
		"name":       {"Oral reading check"},
		"letter":     {"B+"},
	})

	tests, err := ta.store.Assessments(kid, 0, 10)
	if err != nil || len(tests) != 1 {
		t.Fatalf("expected 1 test, got %d (err %v)", len(tests), err)
	}
	if tests[0].HasPercent() {
		t.Error("a test with no score should not claim a percentage")
	}
	if got := tests[0].ScoreLabel(); got != "B+" {
		t.Errorf("expected the letter grade as the label, got %q", got)
	}
}

func TestNotesAttachToKidAndSubject(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	ta.post("/notes", url.Values{
		"kid_id":   {itoa64(kid)},
		"noted_on": {today()},
		"body":     {"Reading has really clicked"},
		"back":     {"/kids/" + itoa64(kid)},
	})
	ta.post("/notes", url.Values{
		"kid_id":     {itoa64(kid)},
		"subject_id": {itoa64(subject)},
		"noted_on":   {today()},
		"body":       {"Still shaky on borrowing"},
		"back":       {"/kids/" + itoa64(kid) + "/subjects/" + itoa64(subject)},
	})

	_, body := ta.get("/kids/" + itoa64(kid))
	mustContain(t, body, "Reading has really clicked", "kid page")
	mustContain(t, body, "Still shaky on borrowing", "kid page")

	subjectNotes, err := ta.store.Notes(kid, subject, 10)
	if err != nil {
		t.Fatalf("listing subject notes: %v", err)
	}
	if len(subjectNotes) != 1 {
		t.Fatalf("expected only the subject note, got %d", len(subjectNotes))
	}
}

func TestProgressIgnoresSkippedWork(t *testing.T) {
	p := Progress{Planned: 1, Done: 3, Skipped: 2}
	if p.Total() != 6 {
		t.Errorf("total should count everything, got %d", p.Total())
	}
	if p.PercentDone() != 75 {
		t.Errorf("skipped lessons should not count against completion, got %d%%", p.PercentDone())
	}
	if (Progress{}).PercentDone() != 0 {
		t.Error("an empty week should report zero rather than dividing by zero")
	}
	if got := (Progress{Minutes: 95}).HoursLabel(); got != "1h 35m" {
		t.Errorf("expected 1h 35m, got %q", got)
	}
}

func TestPasswordLocksAndUnlocks(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")

	if status, _ := ta.get("/"); status != http.StatusOK {
		t.Fatalf("home should be open before a password is set, got %d", status)
	}

	ta.post("/settings/password", url.Values{"password": {"letmein"}})
	if status, _ := ta.get("/"); status != http.StatusOK {
		t.Fatalf("setting a password should keep the current session in, got %d", status)
	}

	// A browser without the cookie gets sent to the lock screen.
	stranger := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := stranger.Get(ta.server.URL + "/")
	if err != nil {
		t.Fatalf("stranger GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Errorf("expected a redirect to /login, got %d %s",
			resp.StatusCode, resp.Header.Get("Location"))
	}

	if _, body := ta.get("/login"); !strings.Contains(body, "Today") {
		// Already signed in, so /login bounces home.
		_ = body
	}

	ta.post("/settings/password", url.Values{"password": {""}})
	resp, err = stranger.Get(ta.server.URL + "/")
	if err != nil {
		t.Fatalf("stranger GET after clearing: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("clearing the password should unlock the app, got %d", resp.StatusCode)
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := hashPassword("correct horse")
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if strings.Contains(hash, "correct horse") {
		t.Error("the stored value must not contain the password")
	}
	if !checkPassword(hash, "correct horse") {
		t.Error("the right password should verify")
	}
	if checkPassword(hash, "wrong horse") {
		t.Error("the wrong password should not verify")
	}

	other, _ := hashPassword("correct horse")
	if other == hash {
		t.Error("two hashes of the same password should differ because of the salt")
	}
}

func TestCrossSiteWritesAreRefused(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")

	req, err := http.NewRequest(http.MethodPost, ta.server.URL+"/lessons",
		strings.NewReader(url.Values{
			"kid_id":     {itoa64(kid)},
			"subject_id": {itoa64(ta.mathSubjectID())},
			"title":      {"Injected"},
		}.Encode()))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, err := ta.client.Do(req)
	if err != nil {
		t.Fatalf("cross-site POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for a cross-site write, got %d", resp.StatusCode)
	}
}

func TestUnknownRecordsReturnNotFound(t *testing.T) {
	ta := newTestApp(t)
	for _, path := range []string{"/kids/4242", "/lessons/4242", "/files/4242", "/kids/4242/tests", "/curriculum/4242", "/series/4242"} {
		if status, _ := ta.get(path); status != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", path, status)
		}
	}
}

func TestWeekStartsOnMonday(t *testing.T) {
	sunday := time.Date(2026, 8, 30, 15, 0, 0, 0, time.Local)
	if got := weekStart(sunday).Format(dateLayout); got != "2026-08-24" {
		t.Errorf("Sunday should belong to the week starting Monday 2026-08-24, got %s", got)
	}
	monday := time.Date(2026, 8, 24, 6, 0, 0, 0, time.Local)
	if got := weekStart(monday).Format(dateLayout); got != "2026-08-24" {
		t.Errorf("Monday should be its own week start, got %s", got)
	}
}

func TestSafeRedirectStaysInsideTheApp(t *testing.T) {
	cases := map[string]string{
		"/planner":           "/planner",
		"//evil.example.com": "/",
		"https://evil.test":  "/",
		"":                   "/",
		"/kids/1/subjects/2": "/kids/1/subjects/2",
	}
	for input, want := range cases {
		if got := safeRedirect(input, "/"); got != want {
			t.Errorf("safeRedirect(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestOccurrenceDatesSkipsOtherDays(t *testing.T) {
	dates, err := occurrenceDates("2026-08-26", "", 3, parseWeekdays("1,2,3,4,5"))
	if err != nil {
		t.Fatalf("occurrenceDates: %v", err)
	}
	want := []string{"2026-08-26", "2026-08-27", "2026-08-28"}
	if len(dates) != len(want) {
		t.Fatalf("got %v, want %v", dates, want)
	}
	for i := range want {
		if dates[i] != want[i] {
			t.Errorf("date %d: got %s, want %s", i, dates[i], want[i])
		}
	}
}

func TestAttendanceMarkAndMonthTotals(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	if _, err := ta.store.CreateSchoolYear("2026–2027", "2026-08-01", "2027-05-31", true); err != nil {
		t.Fatalf("creating year: %v", err)
	}

	status, _ := ta.post("/attendance", url.Values{
		"kid_id":      {itoa64(kid)},
		"attended_on": {today()},
		"status":      {AttendancePresent},
		"notes":       {"On the porch"},
		"back":        {"/"},
	})
	if status != http.StatusOK {
		t.Fatalf("marking present returned %d", status)
	}

	marks, err := ta.store.AttendanceOnDate(today())
	if err != nil {
		t.Fatalf("reading attendance: %v", err)
	}
	got, ok := marks[kid]
	if !ok || !got.IsPresent() {
		t.Fatalf("expected present, got %+v", got)
	}
	if got.Notes != "On the porch" {
		t.Errorf("expected the note to stick, got %q", got.Notes)
	}

	_, body := ta.get("/")
	mustContain(t, body, "Present", "home attendance")

	_, page := ta.get("/attendance?kid=" + itoa64(kid))
	mustContain(t, page, "present", "attendance page")
	mustContain(t, page, "Year report", "attendance page")

	totals, err := ta.store.AttendanceTotalsBetween(today(), today(), kid)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if totals.Present != 1 {
		t.Errorf("expected 1 present, got %d", totals.Present)
	}
}

func TestRepeatingSeriesClonesAndEditFutureOnly(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	start := today()

	status, _ := ta.post("/lessons", url.Values{
		"kid_id":       {itoa64(kid)},
		"subject_id":   {itoa64(subject)},
		"scheduled_on": {start},
		"title":        {"Piano"},
		"minutes":      {"20"},
		"repeat":       {"on"},
		"weekday":      {"1", "3", "5"},
		"repeat_count": {"6"},
		"back":         {"/planner"},
	})
	if status != http.StatusOK {
		t.Fatalf("creating series returned %d", status)
	}

	lessons, err := ta.store.LessonsInRange(start, addDays(start, 40), kid, subject)
	if err != nil {
		t.Fatalf("listing lessons: %v", err)
	}
	if len(lessons) != 6 {
		t.Fatalf("expected 6 clones, got %d", len(lessons))
	}
	if lessons[0].SeriesID == 0 {
		t.Fatal("clones should point at the series")
	}

	first := lessons[0]
	if err := ta.store.SetLessonStatus(first.ID, StatusDone); err != nil {
		t.Fatalf("marking first done: %v", err)
	}

	seriesID := first.SeriesID
	status, _ = ta.post("/series/"+itoa64(seriesID), url.Values{
		"kid_id":           {itoa64(kid)},
		"subject_id":       {itoa64(subject)},
		"title":            {"Piano practice"},
		"minutes":          {"20"},
		"weekday":          {"1", "3", "5"},
		"starts_on":        {start},
		"occurrence_count": {"6"},
	})
	if status != http.StatusOK {
		t.Fatalf("updating series returned %d", status)
	}

	first, _ = ta.store.Lesson(first.ID)
	if first.Title != "Piano" {
		t.Errorf("done copy should keep its title, got %q", first.Title)
	}
	updated := 0
	for _, l := range lessons[1:] {
		got, err := ta.store.Lesson(l.ID)
		if err != nil {
			t.Fatalf("reloading clone: %v", err)
		}
		if got.Title != "Piano practice" {
			t.Errorf("future planned copy %s should take the new title, got %q", got.ScheduledOn, got.Title)
		}
		updated++
	}
	if updated != 5 {
		t.Errorf("expected 5 future copies to update, got %d", updated)
	}
}

func TestCurriculumImportYAMLAndCSV(t *testing.T) {
	ta := newTestApp(t)

	_, page := ta.get("/curriculum")
	mustContain(t, page, `accept=".yaml,.yml,.csv"`, "import file picker")
	mustContain(t, page, "plans:", "YAML schema hint")
	mustContain(t, page, "plan,subject,week,title,minutes,notes", "CSV schema hint")
	mustContain(t, page, `data-toast="Imported!"`, "import toast")

	yamlBody := []byte(`plans:
  - name: 3rd grade Math
    subject: math
    notes: Scope and sequence
    items:
      - week: 1
        title: Place value
        minutes: 40
      - title: Addition
        minutes: 30
        notes: extra drill
  - name: 3rd grade LA
    subject: Language Arts
    items:
      - week: 2
        title: Nouns
`)
	status, page := ta.postFile("/curriculum/import", "file", "plans.yaml", yamlBody)
	if status != http.StatusOK {
		t.Fatalf("importing YAML returned %d", status)
	}
	mustContain(t, page, "Imported 2 plans.", "YAML import flash")

	plans, err := ta.store.CurriculumPlans()
	if err != nil || len(plans) != 2 {
		t.Fatalf("expected 2 plans after YAML import, got %d (err %v)", len(plans), err)
	}
	math, ok := planByName(plans, "3rd grade Math")
	if !ok {
		t.Fatal("math plan missing after YAML import")
	}
	if math.Kind != PlanAuthored || math.Notes != "Scope and sequence" {
		t.Errorf("math plan: %+v", math)
	}
	full, err := ta.store.CurriculumPlan(math.ID)
	if err != nil {
		t.Fatalf("loading math plan: %v", err)
	}
	if len(full.Items) != 2 || full.Items[0].Title != "Place value" || full.Items[0].WeekNumber != 1 ||
		full.Items[0].Minutes != 40 || full.Items[1].Title != "Addition" || full.Items[1].Notes != "extra drill" {
		t.Fatalf("math items: %+v", full.Items)
	}
	la, ok := planByName(plans, "3rd grade LA")
	if !ok {
		t.Fatal("language arts plan missing after YAML import")
	}
	laFull, err := ta.store.CurriculumPlan(la.ID)
	if err != nil || len(laFull.Items) != 1 || laFull.Items[0].Title != "Nouns" || laFull.Items[0].WeekNumber != 2 {
		t.Fatalf("language arts items: %+v (err %v)", laFull.Items, err)
	}

	csvBody := []byte("plan,subject,week,title,minutes,notes\n" +
		"CSV Math,math,1,Place value,40,\n" +
		"CSV Math,math,,Addition,30,extra drill\n" +
		"CSV LA,Language Arts,2,Nouns,,\n")
	status, page = ta.postFile("/curriculum/import", "file", "plans.csv", csvBody)
	if status != http.StatusOK {
		t.Fatalf("importing CSV returned %d", status)
	}
	mustContain(t, page, "Imported 2 plans.", "CSV import flash")

	plans, err = ta.store.CurriculumPlans()
	if err != nil || len(plans) != 4 {
		t.Fatalf("expected 4 plans after CSV import, got %d (err %v)", len(plans), err)
	}
	csvMath, ok := planByName(plans, "CSV Math")
	if !ok {
		t.Fatal("CSV math plan missing")
	}
	csvFull, err := ta.store.CurriculumPlan(csvMath.ID)
	if err != nil || len(csvFull.Items) != 2 || csvFull.Items[0].Title != "Place value" ||
		csvFull.Items[1].Title != "Addition" {
		t.Fatalf("CSV math items: %+v (err %v)", csvFull.Items, err)
	}
}

func TestCurriculumImportUnknownSubjectWritesNothing(t *testing.T) {
	ta := newTestApp(t)

	status, page := ta.postFile("/curriculum/import", "file", "bad.yaml", []byte(`plans:
  - name: 3rd grade Math
    subject: math
    items:
      - title: Place value
  - name: Bogus
    subject: unicorn
    items:
      - title: Hello
`))
	if status != http.StatusOK {
		t.Fatalf("unknown subject import returned %d", status)
	}
	mustContain(t, page, "form-error", "import error on page")
	mustContain(t, page, "unknown subject", "unknown subject message")
	mustNotContain(t, page, "saved-flash", "failed import flash")

	plans, err := ta.store.CurriculumPlans()
	if err != nil {
		t.Fatalf("listing plans: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("failed import should write nothing, got %d plans", len(plans))
	}
}

func planByName(plans []CurriculumPlan, name string) (CurriculumPlan, bool) {
	for _, p := range plans {
		if p.Name == name {
			return p, true
		}
	}
	return CurriculumPlan{}, false
}

func TestCurriculumApplyCreatesWeekdayLessons(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	status, _ := ta.post("/curriculum", url.Values{
		"name":       {"3rd grade Math"},
		"subject_id": {itoa64(subject)},
	})
	if status != http.StatusOK {
		t.Fatalf("creating plan returned %d", status)
	}
	plans, err := ta.store.CurriculumPlans()
	if err != nil || len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d (err %v)", len(plans), err)
	}
	planID := plans[0].ID

	for _, title := range []string{"Place value", "Addition", "Subtraction"} {
		ta.post("/curriculum/"+itoa64(planID)+"/items", url.Values{"title": {title}})
	}

	_, page := ta.get("/curriculum/" + itoa64(planID))
	mustContain(t, page, "Place value", "plan page")

	status, _ = ta.post("/curriculum/"+itoa64(planID)+"/apply", url.Values{
		"kid_id":  {itoa64(kid)},
		"start":   {"2026-08-26"},
		"weekday": {"1", "2", "3", "4", "5"},
	})
	if status != http.StatusOK {
		t.Fatalf("applying plan returned %d", status)
	}

	lessons, err := ta.store.LessonsInRange("2026-08-26", "2026-08-28", kid, subject)
	if err != nil {
		t.Fatalf("listing applied lessons: %v", err)
	}
	if len(lessons) != 3 {
		t.Fatalf("expected 3 lessons, got %d", len(lessons))
	}
	if lessons[0].Title != "Place value" || lessons[0].ScheduledOn != "2026-08-26" {
		t.Errorf("first applied lesson: %+v", lessons[0])
	}
	if lessons[2].Title != "Subtraction" || lessons[2].ScheduledOn != "2026-08-28" {
		t.Errorf("last applied lesson: %+v", lessons[2])
	}
}

func TestArchiveExportsDoneLessonsAsFromYearPlan(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	yearID, err := ta.store.CreateSchoolYear("2025–2026", "2025-08-01", "2026-05-31", true)
	if err != nil {
		t.Fatalf("creating year: %v", err)
	}

	if _, err := ta.store.CreateLesson(Lesson{
		KidID: kid, SubjectID: subject, ScheduledOn: "2025-09-10",
		Title: "Fractions", Minutes: 40, Status: StatusDone, Notes: "Went well",
	}); err != nil {
		t.Fatalf("creating done lesson: %v", err)
	}
	if _, err := ta.store.CreateLesson(Lesson{
		KidID: kid, SubjectID: subject, ScheduledOn: "2025-09-11",
		Title: "Skip this", Status: StatusSkipped,
	}); err != nil {
		t.Fatalf("creating skipped lesson: %v", err)
	}

	_, body := ta.get("/archive?kid=" + itoa64(kid) + "&year=" + itoa64(yearID))
	mustContain(t, body, "Fractions", "archive page")
	mustContain(t, body, "Save as curriculum template", "archive page")

	status, _ := ta.post("/archive/export", url.Values{
		"kid_id":     {itoa64(kid)},
		"year_id":    {itoa64(yearID)},
		"subject_id": {itoa64(subject)},
	})
	if status != http.StatusOK {
		t.Fatalf("export returned %d", status)
	}

	plans, err := ta.store.CurriculumPlans()
	if err != nil || len(plans) != 1 {
		t.Fatalf("expected 1 exported plan, got %d (err %v)", len(plans), err)
	}
	if plans[0].Kind != PlanFromYear {
		t.Errorf("expected from_year kind, got %q", plans[0].Kind)
	}
	plan, err := ta.store.CurriculumPlan(plans[0].ID)
	if err != nil {
		t.Fatalf("loading plan: %v", err)
	}
	if len(plan.Items) != 1 || plan.Items[0].Title != "Fractions" {
		t.Fatalf("exported items: %+v", plan.Items)
	}
	if plan.Items[0].Minutes != 40 || plan.Items[0].Notes != "Went well" {
		t.Errorf("exported item should keep minutes and notes, got %+v", plan.Items[0])
	}

	_, page := ta.get("/curriculum")
	mustContain(t, page, "Saved from a year", "curriculum index")
}
