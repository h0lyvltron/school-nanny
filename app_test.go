package main

import (
	"bytes"
	"html"
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
	when := today()

	status, _ := ta.post("/lessons", url.Values{
		"kid_id":       {itoa64(kid)},
		"subject_id":   {itoa64(subject)},
		"scheduled_on": {when},
		"title":        {"Long division"},
		"minutes":      {"45"},
		"back":         {"/planner"},
	})
	if status != http.StatusOK {
		t.Fatalf("creating a lesson returned %d", status)
	}

	_, body := ta.get("/planner")
	mustContain(t, body, "Long division", "planner")

	lessons, err := ta.store.LessonsBetween(when, when, kid)
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

// Dragging a lesson to another day has to fix up both ends of the move in one
// response, or the planner shows the lesson twice until the next reload.
func TestDragRescheduleRedrawsBothDays(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	from := today()
	to := addDays(from, 2)

	lesson := ta.insertUnassignedLesson(kid, subject, from, "Long division", "")

	status, fragment := ta.postHTMX("/lessons/"+itoa64(lesson)+"/reschedule", url.Values{
		"view":         {"planner"},
		"scheduled_on": {to},
		"kid_filter":   {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("reschedule returned %d", status)
	}

	// The destination day is swapped into the drop target; the day the lesson
	// left rides along out of band.
	mustContain(t, fragment, `id="day-`+to+`"`, "destination day")
	mustContain(t, fragment, `id="day-`+from+`"`, "source day")
	mustContain(t, fragment, `hx-swap-oob="true"`, "source day")
	mustContain(t, fragment, "Long division", "destination day")

	_, source, ok := strings.Cut(fragment, `id="day-`+from+`"`)
	if !ok {
		t.Fatal("source day fragment missing from the response")
	}
	mustNotContain(t, source, "Long division", "source day")

	moved, err := ta.store.Lesson(lesson)
	if err != nil {
		t.Fatalf("reading lesson: %v", err)
	}
	if moved.ScheduledOn != to {
		t.Errorf("expected lesson on %s, got %s", to, moved.ScheduledOn)
	}
}

// A drop back onto the same day should not produce a stray out-of-band swap
// for a day that is already the target.
func TestRescheduleOntoTheSameDayRendersOneDay(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	lesson := ta.insertUnassignedLesson(kid, ta.mathSubjectID(), today(), "Long division", "")

	_, fragment := ta.postHTMX("/lessons/"+itoa64(lesson)+"/reschedule", url.Values{
		"view":         {"planner"},
		"scheduled_on": {today()},
		"kid_filter":   {"0"},
	})
	if got := strings.Count(fragment, `id="day-`); got != 1 {
		t.Errorf("expected 1 day fragment, got %d", got)
	}
	mustNotContain(t, fragment, `hx-swap-oob`, "same-day reschedule")
}

// Ctrl-dragging leaves the original alone and drops a standalone copy, so a
// repeating plan is not quietly rewritten by a copy.
func TestCloneLessonLeavesTheOriginalInPlace(t *testing.T) {
	ta := newTestApp(t)
	_, _, lessons := ta.applyThreeMathLessons()
	source := lessons[0]
	to := addDays(source.ScheduledOn, 7)

	status, fragment := ta.postHTMX("/lessons/"+itoa64(source.ID)+"/clone", url.Values{
		"view":         {"planner"},
		"scheduled_on": {to},
		"kid_filter":   {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("clone returned %d", status)
	}
	mustContain(t, fragment, `id="day-`+to+`"`, "clone destination day")
	mustContain(t, fragment, source.Title, "clone destination day")

	still, err := ta.store.Lesson(source.ID)
	if err != nil {
		t.Fatalf("reading original: %v", err)
	}
	if still.ScheduledOn != source.ScheduledOn {
		t.Errorf("original moved to %s, expected it to stay on %s", still.ScheduledOn, source.ScheduledOn)
	}

	copies, err := ta.store.LessonsBetween(to, to, 0)
	if err != nil {
		t.Fatalf("listing destination day: %v", err)
	}
	var clone *Lesson
	for i := range copies {
		if copies[i].ID != source.ID && copies[i].Title == source.Title {
			clone = &copies[i]
		}
	}
	if clone == nil {
		t.Fatal("expected a copy on the destination day")
	}
	if clone.Status != StatusPlanned {
		t.Errorf("expected the copy to be planned, got %q", clone.Status)
	}
	if clone.AssignmentID != 0 || clone.SeriesID != 0 {
		t.Errorf("expected a standalone copy, got assignment %d series %d",
			clone.AssignmentID, clone.SeriesID)
	}
}

// Dropping onto another child's chip copies the work across to them.
func TestCloneLessonToAnotherKid(t *testing.T) {
	ta := newTestApp(t)
	mia := ta.addKid("Mia")
	theo := ta.addKid("Theo")
	subject := ta.mathSubjectID()
	date := today()

	lesson := ta.insertUnassignedLesson(mia, subject, date, "Long division", "")

	status, _ := ta.postHTMX("/lessons/"+itoa64(lesson)+"/clone", url.Values{
		"view":         {"planner"},
		"scheduled_on": {date},
		"kid_id":       {itoa64(theo)},
		"kid_filter":   {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("clone returned %d", status)
	}

	theirs, err := ta.store.LessonsBetween(date, date, theo)
	if err != nil {
		t.Fatalf("listing Theo's day: %v", err)
	}
	if len(theirs) != 1 || theirs[0].Title != "Long division" {
		t.Fatalf("expected one copied lesson for Theo, got %d", len(theirs))
	}

	hers, err := ta.store.LessonsBetween(date, date, mia)
	if err != nil {
		t.Fatalf("listing Mia's day: %v", err)
	}
	if len(hers) != 1 {
		t.Fatalf("expected Mia to keep exactly one lesson, got %d", len(hers))
	}
}

// The planner needs the drag handles and the drop chips in its markup for any
// of this to be reachable with a mouse.
func TestPlannerMarkupIsDraggable(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	ta.insertUnassignedLesson(kid, ta.mathSubjectID(), today(), "Long division", "")

	status, body := ta.get("/planner")
	if status != http.StatusOK {
		t.Fatalf("planner returned %d", status)
	}
	mustContain(t, body, `draggable="true"`, "planner")
	mustContain(t, body, `class="kid-target"`, "planner")
	mustContain(t, body, `class="day-copy-target"`, "planner")
	mustContain(t, body, `data-kid-filter="0"`, "planner")
	mustContain(t, body, "/static/planner.js", "planner")

	status, js := ta.get("/static/planner.js")
	if status != http.StatusOK {
		t.Fatalf("planner.js returned %d", status)
	}
	mustContain(t, js, "pointerdown", "touch long-press")
	mustContain(t, js, "LONG_PRESS_MS", "touch long-press")
}

// A one-pixel PNG, which is enough for the sniffing the upload does.
var tinyPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
	0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

func TestKidPhotoUploadServeAndRemove(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")

	status, _ := ta.postFile("/settings/kids/"+itoa64(kid)+"/avatar", "file", "mia.png", tinyPNG)
	if status != http.StatusOK {
		t.Fatalf("uploading a photo returned %d", status)
	}

	saved, err := ta.store.Kid(kid)
	if err != nil {
		t.Fatalf("reading kid: %v", err)
	}
	if !saved.HasPhoto() {
		t.Fatal("expected the child to have a photo")
	}
	if _, ok := ta.resolveUpload(saved.AvatarPath); !ok {
		t.Fatalf("stored photo path %q escapes the upload folder", saved.AvatarPath)
	}

	status, body := ta.get("/avatars/kids/" + itoa64(kid))
	if status != http.StatusOK {
		t.Fatalf("serving the photo returned %d", status)
	}
	if body != string(tinyPNG) {
		t.Error("served photo does not match what was uploaded")
	}

	// The photo should now stand in for the colour dot wherever the child is
	// named.
	_, home := ta.get("/")
	mustContain(t, home, "/avatars/kids/"+itoa64(kid), "home")

	stored, _ := ta.resolveUpload(saved.AvatarPath)
	status, _ = ta.post("/settings/kids/"+itoa64(kid)+"/avatar/delete", url.Values{})
	if status != http.StatusOK {
		t.Fatalf("removing the photo returned %d", status)
	}
	after, err := ta.store.Kid(kid)
	if err != nil {
		t.Fatalf("reading kid: %v", err)
	}
	if after.HasPhoto() {
		t.Error("expected the photo to be cleared")
	}
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Error("expected the photo file to be deleted from disk")
	}

	status, _ = ta.get("/avatars/kids/" + itoa64(kid))
	if status != http.StatusNotFound {
		t.Errorf("expected 404 for a child with no photo, got %d", status)
	}
}

// Replacing a photo should not leave the old file behind.
func TestReplacingAPhotoDeletesTheOldOne(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")

	ta.postFile("/settings/kids/"+itoa64(kid)+"/avatar", "file", "first.png", tinyPNG)
	first, _ := ta.store.Kid(kid)
	firstPath, _ := ta.resolveUpload(first.AvatarPath)

	ta.postFile("/settings/kids/"+itoa64(kid)+"/avatar", "file", "second.png", tinyPNG)
	second, _ := ta.store.Kid(kid)

	if second.AvatarPath == first.AvatarPath {
		t.Fatal("expected the replacement to be stored under a new name")
	}
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Error("expected the replaced photo to be deleted from disk")
	}
}

// The upload sniffs the file rather than trusting its name, so a renamed
// document cannot become a child's photo.
func TestNonImagePhotoIsRejected(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")

	status, body := ta.postFile("/settings/kids/"+itoa64(kid)+"/avatar", "file", "notes.png",
		[]byte("this is plain text pretending to be a png"))
	if status != http.StatusBadRequest {
		t.Fatalf("expected the upload to be refused, got %d", status)
	}
	mustContain(t, body, "not a JPEG", "rejection message")
	saved, _ := ta.store.Kid(kid)
	if saved.HasPhoto() {
		t.Error("expected no photo to be recorded")
	}
}

// Phone photos from Windows often sit well above the old 2 MB cap. A clear
// size error is more useful than pretending the file was not an image.
func TestOversizedPhotoIsRejectedWithAClearMessage(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")

	// A JPEG header followed by enough zeros to clear the limit. DetectContentType
	// only needs the first bytes; the size check happens before a full decode.
	big := make([]byte, maxAvatarBytes+1024)
	copy(big, []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'})
	status, body := ta.postFile("/settings/kids/"+itoa64(kid)+"/avatar", "file", "huge.jpg", big)
	if status != http.StatusBadRequest {
		t.Fatalf("expected the upload to be refused, got %d", status)
	}
	mustContain(t, body, "8 MB", "size rejection")
	saved, _ := ta.store.Kid(kid)
	if saved.HasPhoto() {
		t.Error("expected no photo to be recorded")
	}
}

func TestHEICPhotoGetsAUsefulMessage(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")

	// Minimal ftyp/heic brand; enough for looksLikeHEIC without a real image.
	heic := []byte{
		0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c',
		0x00, 0x00, 0x00, 0x00, 'm', 'i', 'f', '1', 'h', 'e', 'i', 'c',
	}
	status, body := ta.postFile("/settings/kids/"+itoa64(kid)+"/avatar", "file", "photo.heic", heic)
	if status != http.StatusBadRequest {
		t.Fatalf("expected the upload to be refused, got %d", status)
	}
	mustContain(t, body, "HEIC", "heic rejection")
}

func TestRemovingAChildDeletesTheirPhoto(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")

	ta.postFile("/settings/kids/"+itoa64(kid)+"/avatar", "file", "mia.png", tinyPNG)
	saved, _ := ta.store.Kid(kid)
	stored, _ := ta.resolveUpload(saved.AvatarPath)

	status, _ := ta.post("/settings/kids/"+itoa64(kid)+"/delete", url.Values{})
	if status != http.StatusOK {
		t.Fatalf("removing the child returned %d", status)
	}
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Error("expected the photo file to be deleted along with the child")
	}
}

// Making room for adults meant rebuilding the lessons and notes tables, and a
// rebuild done carelessly takes the rows that pointed at them along with it.
// This walks a database from the version before adults existed to the current
// one and checks that nothing was lost on the way.
func TestUpgradingToAdultsKeepsExistingRecords(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, dbFileName))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if _, err := store.db().Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("creating migration table: %v", err)
	}
	for _, name := range []string{
		"0001_init.sql", "0002_attendance.sql", "0003_curriculum.sql",
		"0004_lesson_series.sql", "0005_plan_assignments.sql", "0006_avatars.sql",
	} {
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		version, _ := migrationVersion(name)
		if err := store.applyMigration(name, version, string(body)); err != nil {
			t.Fatalf("applying %s: %v", name, err)
		}
	}

	// Records of the kind that hang off a lesson, which is what a bad rebuild
	// would silently cascade away.
	kid, err := store.CreateKid("Mia", "3rd", "#5b8def")
	if err != nil {
		t.Fatalf("creating kid: %v", err)
	}
	var subject int64
	if err := store.db().QueryRow(`SELECT id FROM subjects WHERE slug = 'math'`).Scan(&subject); err != nil {
		t.Fatalf("finding subject: %v", err)
	}
	res, err := store.db().Exec(`INSERT INTO lessons
		(kid_id, subject_id, scheduled_on, status, title, minutes, notes, created_at)
		VALUES (?, ?, ?, 'planned', 'Long division', 30, '', ?)`,
		kid, subject, today(), today())
	if err != nil {
		t.Fatalf("inserting lesson: %v", err)
	}
	lesson, _ := res.LastInsertId()

	if _, err := store.db().Exec(`INSERT INTO assessments
		(kid_id, subject_id, lesson_id, given_on, name, created_at)
		VALUES (?, ?, ?, ?, 'Chapter 3 test', ?)`,
		kid, subject, lesson, today(), today()); err != nil {
		t.Fatalf("inserting assessment: %v", err)
	}
	if _, err := store.db().Exec(`INSERT INTO attachments
		(owner_type, lesson_id, original_name, stored_path, size_bytes, created_at)
		VALUES ('lesson', ?, 'worksheet.pdf', '2026/01/abc-worksheet.pdf', 12, ?)`,
		lesson, today()); err != nil {
		t.Fatalf("inserting attachment: %v", err)
	}
	if _, err := store.db().Exec(`INSERT INTO notes (kid_id, noted_on, body, created_at)
		VALUES (?, ?, 'Struggled with remainders', ?)`, kid, today(), today()); err != nil {
		t.Fatalf("inserting note: %v", err)
	}

	if err := store.Migrate(); err != nil {
		t.Fatalf("upgrading: %v", err)
	}

	counts := map[string]int{}
	for _, table := range []string{"lessons", "assessments", "attachments", "notes"} {
		var n int
		if err := store.db().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		counts[table] = n
	}
	for table, n := range counts {
		if n != 1 {
			t.Errorf("expected 1 row in %s after the upgrade, got %d", table, n)
		}
	}

	// The assessment must still point at the lesson it was recorded against.
	var linked int64
	if err := store.db().QueryRow(`SELECT COALESCE(lesson_id, 0) FROM assessments`).Scan(&linked); err != nil {
		t.Fatalf("reading assessment: %v", err)
	}
	if linked != lesson {
		t.Errorf("assessment lost its lesson: got %d, want %d", linked, lesson)
	}

	rows, err := store.db().Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("checking foreign keys: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Error("the upgraded database has rows pointing at things that are not there")
	}
}

// The family always has one grown-up to work with, without anyone having to
// set her up first.
func (ta *testApp) mom() Adult {
	ta.t.Helper()
	adults, err := ta.store.Adults(false)
	if err != nil {
		ta.t.Fatalf("listing adults: %v", err)
	}
	if len(adults) != 1 {
		ta.t.Fatalf("expected exactly one adult, got %d", len(adults))
	}
	return adults[0]
}

func TestDefaultAdultIsReadyToUse(t *testing.T) {
	ta := newTestApp(t)
	mom := ta.mom()
	if mom.Name != "Mom" {
		t.Errorf("expected the default adult to be called Mom, got %q", mom.Name)
	}

	status, body := ta.get("/adults/" + itoa64(mom.ID))
	if status != http.StatusOK {
		t.Fatalf("her profile returned %d", status)
	}
	mustContain(t, body, "Pinboard", "adult profile")
	mustContain(t, body, "This week", "adult profile")

	// She is reachable from anywhere, next to the children.
	_, home := ta.get("/")
	mustContain(t, home, `href="/adults/`+itoa64(mom.ID)+`"`, "nav")
}

// Her dentist appointment is not a lesson for the kids, so it must not show up
// in any view built for them.
func TestAdultScheduleStaysOutOfKidViews(t *testing.T) {
	ta := newTestApp(t)
	mom := ta.mom()
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	date := today()

	ta.insertUnassignedLesson(kid, subject, date, "Long division", "")
	status, _ := ta.post("/adults/"+itoa64(mom.ID)+"/schedule", url.Values{
		"subject_id":   {itoa64(subject)},
		"scheduled_on": {date},
		"title":        {"Dentist appointment"},
	})
	if status != http.StatusOK {
		t.Fatalf("booking her appointment returned %d", status)
	}

	for _, path := range []string{"/", "/planner", "/kids/" + itoa64(kid), "/archive"} {
		_, body := ta.get(path)
		mustNotContain(t, body, "Dentist appointment", path)
	}

	// Nor in the numbers the kid views are built from.
	week, err := ta.store.LessonsBetween(date, date, 0)
	if err != nil {
		t.Fatalf("listing the week: %v", err)
	}
	if len(week) != 1 || week[0].Title != "Long division" {
		t.Fatalf("expected only the child's lesson, got %d", len(week))
	}
	progress, err := ta.store.ProgressBetween(date, date, 0, 0)
	if err != nil {
		t.Fatalf("reading progress: %v", err)
	}
	if progress.Total() != 1 {
		t.Errorf("expected the family week to count 1 lesson, got %d", progress.Total())
	}
	overdue, err := ta.store.LessonsOverdue(addDays(date, 30), 25)
	if err != nil {
		t.Fatalf("listing overdue: %v", err)
	}
	for _, l := range overdue {
		if l.ForAdult() {
			t.Error("an adult item turned up in the overdue list")
		}
	}

	// The curriculum backfill runs on every start and reads lessons that
	// belong to no plan, which is every adult item she has ever booked.
	if err := ta.store.Migrate(); err != nil {
		t.Fatalf("restarting: %v", err)
	}
	hers, err := ta.store.AdultLessonsBetween(date, date, mom.ID)
	if err != nil {
		t.Fatalf("listing her day: %v", err)
	}
	if len(hers) != 1 {
		t.Fatalf("expected her appointment to survive a restart, got %d", len(hers))
	}
	if hers[0].AssignmentID != 0 {
		t.Error("her appointment was swept into a curriculum plan")
	}

	// It is on her own week, though.
	_, page := ta.get("/adults/" + itoa64(mom.ID) + "/schedule")
	mustContain(t, page, "Dentist appointment", "her schedule")
}

func TestAdultNotesAreSeparateFromKidNotes(t *testing.T) {
	ta := newTestApp(t)
	mom := ta.mom()
	kid := ta.addKid("Mia")

	ta.post("/notes", url.Values{
		"adult_id": {itoa64(mom.ID)},
		"noted_on": {today()},
		"body":     {"Order more printer paper"},
		"back":     {"/adults/" + itoa64(mom.ID)},
	})
	ta.post("/notes", url.Values{
		"kid_id":   {itoa64(kid)},
		"noted_on": {today()},
		"body":     {"Struggled with remainders"},
		"back":     {"/kids/" + itoa64(kid)},
	})

	hers, err := ta.store.AdultNotes(mom.ID, 20)
	if err != nil {
		t.Fatalf("listing her notes: %v", err)
	}
	if len(hers) != 1 || hers[0].Body != "Order more printer paper" {
		t.Fatalf("expected exactly her own note, got %d", len(hers))
	}
	theirs, err := ta.store.Notes(kid, 0, 20)
	if err != nil {
		t.Fatalf("listing the child's notes: %v", err)
	}
	if len(theirs) != 1 || theirs[0].Body != "Struggled with remainders" {
		t.Fatalf("expected exactly the child's note, got %d", len(theirs))
	}

	_, page := ta.get("/kids/" + itoa64(kid))
	mustNotContain(t, page, "printer paper", "kid page")
}

// A note or lesson has to belong to someone, and to exactly one someone.
func TestALessonCannotBelongToBothAKidAndAnAdult(t *testing.T) {
	ta := newTestApp(t)
	mom := ta.mom()
	kid := ta.addKid("Mia")

	_, err := ta.store.db().Exec(`INSERT INTO lessons
		(kid_id, adult_id, subject_id, scheduled_on, status, title, minutes, notes, created_at)
		VALUES (?, ?, ?, ?, 'planned', 'Both at once', 0, '', ?)`,
		kid, mom.ID, ta.mathSubjectID(), today(), today())
	if err == nil {
		t.Error("expected the database to refuse a lesson belonging to two people")
	}

	_, err = ta.store.db().Exec(`INSERT INTO lessons
		(subject_id, scheduled_on, status, title, minutes, notes, created_at)
		VALUES (?, ?, 'planned', 'Nobody', 0, '', ?)`,
		ta.mathSubjectID(), today(), today())
	if err == nil {
		t.Error("expected the database to refuse a lesson belonging to nobody")
	}
}

func TestPinboardCardsRoundTrip(t *testing.T) {
	ta := newTestApp(t)
	mom := ta.mom()
	base := "/adults/" + itoa64(mom.ID)

	for _, title := range []string{"Curriculum wish list", "Field trip ideas"} {
		status, _ := ta.post(base+"/cards", url.Values{"title": {title}})
		if status != http.StatusOK {
			t.Fatalf("adding %q returned %d", title, status)
		}
	}

	cards, err := ta.store.AdultCards(mom.ID)
	if err != nil || len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d (err %v)", len(cards), err)
	}
	if cards[0].Title != "Curriculum wish list" {
		t.Errorf("expected the first card added to come first, got %q", cards[0].Title)
	}

	// Moving the second up should put it first.
	if status, _ := ta.post(base+"/cards/"+itoa64(cards[1].ID)+"/move",
		url.Values{"direction": {"up"}}); status != http.StatusOK {
		t.Fatalf("moving a card returned %d", status)
	}
	cards, _ = ta.store.AdultCards(mom.ID)
	if cards[0].Title != "Field trip ideas" {
		t.Errorf("expected the moved card first, got %q", cards[0].Title)
	}

	// Pinning lifts a card above the unpinned ones regardless of order.
	last := cards[len(cards)-1]
	ta.post(base+"/cards/"+itoa64(last.ID), url.Values{
		"title":  {last.Title},
		"body":   {"Ask the co-op about the science kit"},
		"pinned": {"on"},
	})
	cards, _ = ta.store.AdultCards(mom.ID)
	if !cards[0].Pinned || cards[0].ID != last.ID {
		t.Error("expected the pinned card to come first")
	}

	_, page := ta.get(base)
	mustContain(t, page, "Ask the co-op about the science kit", "pinboard")

	if status, _ := ta.post(base+"/cards/"+itoa64(last.ID)+"/delete", url.Values{}); status != http.StatusOK {
		t.Fatalf("deleting a card returned %d", status)
	}
	cards, _ = ta.store.AdultCards(mom.ID)
	if len(cards) != 1 {
		t.Errorf("expected 1 card left, got %d", len(cards))
	}
}

func TestAdultPhotoAndSettings(t *testing.T) {
	ta := newTestApp(t)
	mom := ta.mom()

	status, _ := ta.post("/settings/adults", url.Values{
		"id":    {itoa64(mom.ID)},
		"name":  {"Sarah"},
		"role":  {"Mom"},
		"color": {"#3fae7f"},
	})
	if status != http.StatusOK {
		t.Fatalf("saving her details returned %d", status)
	}
	updated, _ := ta.store.Adult(mom.ID)
	if updated.Name != "Sarah" {
		t.Errorf("expected her name to be saved, got %q", updated.Name)
	}

	if status, _ := ta.postFile("/settings/adults/"+itoa64(mom.ID)+"/avatar",
		"file", "sarah.png", tinyPNG); status != http.StatusOK {
		t.Fatalf("uploading her photo returned %d", status)
	}
	withPhoto, _ := ta.store.Adult(mom.ID)
	if !withPhoto.HasPhoto() {
		t.Fatal("expected her photo to be recorded")
	}
	if status, _ := ta.get("/avatars/adults/" + itoa64(mom.ID)); status != http.StatusOK {
		t.Errorf("serving her photo returned %d", status)
	}
	_, nav := ta.get("/")
	mustContain(t, nav, "/avatars/adults/"+itoa64(mom.ID), "nav")
}

// Removing a grown-up should take her schedule, notes, and pinboard with her.
func TestDeletingAnAdultClearsHerRecords(t *testing.T) {
	ta := newTestApp(t)
	mom := ta.mom()
	base := "/adults/" + itoa64(mom.ID)

	ta.post(base+"/schedule", url.Values{
		"subject_id":   {itoa64(ta.mathSubjectID())},
		"scheduled_on": {today()},
		"title":        {"Dentist appointment"},
	})
	ta.post("/notes", url.Values{
		"adult_id": {itoa64(mom.ID)},
		"noted_on": {today()},
		"body":     {"Order more printer paper"},
	})
	ta.post(base+"/cards", url.Values{"title": {"Curriculum wish list"}})

	if _, err := ta.store.db().Exec(`DELETE FROM adults WHERE id = ?`, mom.ID); err != nil {
		t.Fatalf("deleting her: %v", err)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM lessons WHERE adult_id IS NOT NULL`,
		`SELECT COUNT(*) FROM notes WHERE adult_id IS NOT NULL`,
		`SELECT COUNT(*) FROM adult_cards`,
	} {
		var n int
		if err := ta.store.db().QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if n != 0 {
			t.Errorf("expected nothing left for %q, got %d rows", q, n)
		}
	}
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
	if lessons[0].AssignmentID == 0 {
		t.Fatal("applied lessons should belong to a plan assignment")
	}
	if lessons[0].AssignmentID != lessons[1].AssignmentID || lessons[1].AssignmentID != lessons[2].AssignmentID {
		t.Fatal("applied lessons should share one assignment")
	}
	asg, err := ta.store.Assignment(lessons[0].AssignmentID)
	if err != nil {
		t.Fatalf("loading assignment: %v", err)
	}
	if asg.Name != "3rd grade Math" || asg.Weekdays != "1,2,3,4,5" {
		t.Errorf("assignment: %+v", asg)
	}
	if lessons[0].Sequence == 0 || lessons[1].Sequence <= lessons[0].Sequence {
		t.Errorf("expected increasing sequence, got %d, %d, %d",
			lessons[0].Sequence, lessons[1].Sequence, lessons[2].Sequence)
	}

	_, page = ta.get("/curriculum/" + itoa64(planID))
	mustContain(t, page, "Scheduled to a child", "plan page assignment")
	mustContain(t, page, "3rd grade Math for Mia", "plan page assignment link")

	_, page = ta.get("/assignments/" + itoa64(asg.ID))
	mustContain(t, page, "Pause and shift remaining", "assignment page")
	mustContain(t, page, "Place value", "assignment upcoming")
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

// A subject's colour is what stops a day of lessons reading as one block of
// black text, so it has to survive the settings form and reach the title.
func TestSubjectColourReachesTheLessonTitle(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	subjects, err := ta.store.Subjects(true)
	if err != nil {
		t.Fatalf("listing subjects: %v", err)
	}
	seen := map[string]bool{}
	for _, sub := range subjects {
		if sub.Color == "" {
			t.Fatalf("subject %q was left without a colour", sub.Name)
		}
		if seen[sub.Color] {
			t.Errorf("subject %q repeats the colour %s", sub.Name, sub.Color)
		}
		seen[sub.Color] = true
	}

	status, _ := ta.post("/settings/subjects", url.Values{
		"id":    {itoa64(subject)},
		"name":  {"Math"},
		"color": {"#b8437a"},
	})
	if status != http.StatusOK {
		t.Fatalf("saving the subject returned %d", status)
	}
	saved, err := ta.store.Subject(subject)
	if err != nil {
		t.Fatalf("reloading the subject: %v", err)
	}
	if saved.Color != "#b8437a" || saved.Name != "Math" {
		t.Fatalf("expected the colour to be saved, got %+v", saved)
	}

	ta.insertUnassignedLesson(kid, subject, today(), "Place value", today())
	_, home := ta.get("/")
	mustContain(t, home, "--subject:#b8437a", "today's lesson card")
	mustContain(t, home, `<span class="lesson-subject">Math</span>`, "today's subject label")

	_, planner := ta.get("/planner")
	mustContain(t, planner, "--subject:#b8437a", "the planner's lesson card")
	mustContain(t, planner, `<span class="lesson-subject">Math</span>`, "the planner's subject label")

	_, settings := ta.get("/settings")
	mustContain(t, settings, `value="#b8437a"`, "subject colour picker")
}

// schoolWeekdays is Monday to Friday, the mask every plan in these tests runs on.
const schoolWeekdays = "1,2,3,4,5"

func TestPushResumeOnUsesNextSchoolDay(t *testing.T) {
	weekdays := schoolWeekdays
	got := pushResumeOn("2026-08-26", "2026-08-26", weekdays)
	if got != "2026-08-27" {
		t.Errorf("pushing Wednesday's lesson on Wednesday: got %s, want Thursday", got)
	}
	got = pushResumeOn("2026-09-04", "2026-08-31", weekdays)
	if got != "2026-09-04" {
		t.Errorf("pushing overdue Monday on Friday: got %s, want Friday", got)
	}
	got = nextMatchingWeekdayOnOrAfter("2026-08-29", parseWeekdays(weekdays))
	if got != "2026-08-31" {
		t.Errorf("Saturday should snap to Monday, got %s", got)
	}
}

func TestSkipDoesNotShiftLaterLessons(t *testing.T) {
	ta := newTestApp(t)
	kid, subject, lessons := ta.applyThreeMathLessons()

	status, _ := ta.post("/lessons/"+itoa64(lessons[0].ID)+"/status", url.Values{
		"status": {StatusSkipped},
		"back":   {"/"},
	})
	if status != http.StatusOK {
		t.Fatalf("skip returned %d", status)
	}

	got, err := ta.store.LessonsInRange("2026-08-26", "2026-08-28", kid, subject)
	if err != nil {
		t.Fatalf("listing lessons: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 lessons, got %d", len(got))
	}
	if got[0].Status != StatusSkipped || got[0].ScheduledOn != "2026-08-26" {
		t.Errorf("skipped lesson moved or lost status: %+v", got[0])
	}
	if got[1].ScheduledOn != "2026-08-27" || got[2].ScheduledOn != "2026-08-28" {
		t.Errorf("later lessons shifted after skip: %s, %s", got[1].ScheduledOn, got[2].ScheduledOn)
	}
}

func TestPushMovesThisAndLaterPlanned(t *testing.T) {
	ta := newTestApp(t)
	kid, subject, lessons := ta.applyThreeMathLessons()

	if err := ta.store.SetLessonStatus(lessons[0].ID, StatusDone); err != nil {
		t.Fatalf("marking first done: %v", err)
	}

	status, _ := ta.post("/lessons/"+itoa64(lessons[1].ID)+"/push", url.Values{"back": {"/"}})
	if status != http.StatusOK {
		t.Fatalf("push returned %d", status)
	}

	first, err := ta.store.Lesson(lessons[0].ID)
	if err != nil {
		t.Fatalf("reloading first: %v", err)
	}
	if first.ScheduledOn != "2026-08-26" || first.Status != StatusDone {
		t.Errorf("done lesson should stay put: %+v", first)
	}

	asg, err := ta.store.Assignment(lessons[1].AssignmentID)
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	resume := pushResumeOn(today(), lessons[1].ScheduledOn, asg.Weekdays)
	want, err := occurrenceDates(resume, "", 2, parseWeekdays(asg.Weekdays))
	if err != nil {
		t.Fatalf("expected dates: %v", err)
	}

	second, _ := ta.store.Lesson(lessons[1].ID)
	third, _ := ta.store.Lesson(lessons[2].ID)
	if second.ScheduledOn != want[0] || third.ScheduledOn != want[1] {
		t.Errorf("pushed dates %s, %s; want %s, %s",
			second.ScheduledOn, third.ScheduledOn, want[0], want[1])
	}
	if second.Title != "Addition" || third.Title != "Subtraction" {
		t.Errorf("titles should stay with the lessons: %q, %q", second.Title, third.Title)
	}
	_ = kid
	_ = subject
}

// The kids want to keep going, so tomorrow's lesson is done today and the rest
// of the plan closes up behind it.
func TestPullMovesThisAndLaterPlannedOntoToday(t *testing.T) {
	ta := newTestApp(t)
	start := nextMatchingWeekdayOnOrAfter(addDays(today(), 14), parseWeekdays(schoolWeekdays))
	_, _, lessons := ta.applyThreeMathLessonsFrom(start)

	if err := ta.store.SetLessonStatus(lessons[0].ID, StatusDone); err != nil {
		t.Fatalf("marking first done: %v", err)
	}

	status, _ := ta.post("/lessons/"+itoa64(lessons[1].ID)+"/pull", url.Values{"back": {"/"}})
	if status != http.StatusOK {
		t.Fatalf("pull returned %d", status)
	}

	first, err := ta.store.Lesson(lessons[0].ID)
	if err != nil {
		t.Fatalf("reloading first: %v", err)
	}
	if first.ScheduledOn != lessons[0].ScheduledOn || first.Status != StatusDone {
		t.Errorf("done lesson should stay put: %+v", first)
	}

	want, err := occurrenceDates(nextMatchingWeekdayOnOrAfter(today(), parseWeekdays(schoolWeekdays)),
		"", 2, parseWeekdays(schoolWeekdays))
	if err != nil {
		t.Fatalf("expected dates: %v", err)
	}

	second, _ := ta.store.Lesson(lessons[1].ID)
	third, _ := ta.store.Lesson(lessons[2].ID)
	if second.ScheduledOn != want[0] || third.ScheduledOn != want[1] {
		t.Errorf("pulled dates %s, %s; want %s, %s",
			second.ScheduledOn, third.ScheduledOn, want[0], want[1])
	}
	if second.Title != "Addition" || third.Title != "Subtraction" {
		t.Errorf("titles should stay with the lessons: %q, %q", second.Title, third.Title)
	}
}

// Pull is only offered on work that is still ahead: there is nothing to bring
// forward on a lesson already sitting on today.
func TestPullIsOfferedOnlyOnLaterLessons(t *testing.T) {
	if nextMatchingWeekdayOnOrAfter(today(), parseWeekdays(schoolWeekdays)) != today() {
		t.Skip("run on a weekend, where a pulled lesson still lands ahead of today")
	}
	ta := newTestApp(t)
	start := nextMatchingWeekdayOnOrAfter(addDays(today(), 14), parseWeekdays(schoolWeekdays))
	_, _, lessons := ta.applyThreeMathLessonsFrom(start)

	_, page := ta.get("/lessons/" + itoa64(lessons[0].ID))
	mustContain(t, page, "/lessons/"+itoa64(lessons[0].ID)+"/pull", "pull form on a later lesson")

	status, _ := ta.post("/lessons/"+itoa64(lessons[0].ID)+"/pull", url.Values{"back": {"/"}})
	if status != http.StatusOK {
		t.Fatalf("pull returned %d", status)
	}

	_, page = ta.get("/lessons/" + itoa64(lessons[0].ID))
	mustNotContain(t, page, "/lessons/"+itoa64(lessons[0].ID)+"/pull", "pull form once the lesson is today")
	mustContain(t, page, "/lessons/"+itoa64(lessons[0].ID)+"/push", "push form on today's lesson")
}

func TestPullNeedsAScheduledPlan(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Ada")
	subject := ta.mathSubjectID()
	id := ta.insertUnassignedLesson(kid, subject, addDays(today(), 3), "Fractions", today())

	status, _ := ta.post("/lessons/"+itoa64(id)+"/pull", url.Values{"back": {"/"}})
	if status != http.StatusBadRequest {
		t.Fatalf("pulling a lesson with no plan returned %d, want 400", status)
	}
}

// Dragging a lesson that belongs to a plan is Push and Pull by hand: the rest
// of the plan follows it, in whichever direction it went. Dropping onto a
// Saturday keeps it there, because that is the day the parent pointed at, but
// the lessons behind it still land on school days.
func TestDraggingAPlanLessonEarlierBringsTheRestWithIt(t *testing.T) {
	ta := newTestApp(t)
	start := mondayWeeksOut(3)
	_, _, lessons := ta.applyThreeMathLessonsFrom(start)
	saturday := addDays(start, -2)

	status, _ := ta.postHTMX("/lessons/"+itoa64(lessons[0].ID)+"/reschedule", url.Values{
		"view":         {"planner"},
		"scheduled_on": {saturday},
		"kid_filter":   {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("reschedule returned %d", status)
	}

	first, _ := ta.store.Lesson(lessons[0].ID)
	second, _ := ta.store.Lesson(lessons[1].ID)
	third, _ := ta.store.Lesson(lessons[2].ID)
	if first.ScheduledOn != saturday {
		t.Errorf("dropped lesson should stay on %s, got %s", saturday, first.ScheduledOn)
	}
	if second.ScheduledOn != start || third.ScheduledOn != addDays(start, 1) {
		t.Errorf("later lessons should follow onto %s and %s, got %s and %s",
			start, addDays(start, 1), second.ScheduledOn, third.ScheduledOn)
	}
}

func TestDraggingAPlanLessonLaterPushesTheRestBack(t *testing.T) {
	ta := newTestApp(t)
	start := mondayWeeksOut(3)
	_, _, lessons := ta.applyThreeMathLessonsFrom(start)
	thursday := addDays(start, 3)

	status, fragment := ta.postHTMX("/lessons/"+itoa64(lessons[0].ID)+"/reschedule", url.Values{
		"view":         {"planner"},
		"scheduled_on": {thursday},
		"kid_filter":   {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("reschedule returned %d", status)
	}

	first, _ := ta.store.Lesson(lessons[0].ID)
	second, _ := ta.store.Lesson(lessons[1].ID)
	third, _ := ta.store.Lesson(lessons[2].ID)
	if first.ScheduledOn != thursday {
		t.Errorf("dropped lesson should be on %s, got %s", thursday, first.ScheduledOn)
	}
	if second.ScheduledOn != addDays(start, 4) || third.ScheduledOn != addDays(start, 7) {
		t.Errorf("later lessons should fall on %s and %s, got %s and %s",
			addDays(start, 4), addDays(start, 7), second.ScheduledOn, third.ScheduledOn)
	}

	// A cascade moves lessons the drop day and the day left behind know nothing
	// about, so the whole week has to come back.
	if got := strings.Count(fragment, `id="day-`); got != 7 {
		t.Errorf("expected the week's 7 days in the response, got %d", got)
	}
	mustContain(t, fragment, `id="day-`+thursday+`"`, "drop day")
	mustContain(t, fragment, `id="day-`+addDays(start, 4)+`"`, "day a later lesson moved onto")
}

// A lesson already marked done is a record of what happened, so dragging it to
// tidy the week must not rewrite what is still planned.
func TestDraggingADoneLessonLeavesThePlanAlone(t *testing.T) {
	ta := newTestApp(t)
	start := mondayWeeksOut(3)
	_, _, lessons := ta.applyThreeMathLessonsFrom(start)
	if err := ta.store.SetLessonStatus(lessons[0].ID, StatusDone); err != nil {
		t.Fatalf("marking first done: %v", err)
	}

	status, fragment := ta.postHTMX("/lessons/"+itoa64(lessons[0].ID)+"/reschedule", url.Values{
		"view":         {"planner"},
		"scheduled_on": {addDays(start, 3)},
		"kid_filter":   {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("reschedule returned %d", status)
	}

	first, _ := ta.store.Lesson(lessons[0].ID)
	second, _ := ta.store.Lesson(lessons[1].ID)
	third, _ := ta.store.Lesson(lessons[2].ID)
	if first.ScheduledOn != addDays(start, 3) {
		t.Errorf("the done lesson should still move where it was dropped, got %s", first.ScheduledOn)
	}
	if second.ScheduledOn != lessons[1].ScheduledOn || third.ScheduledOn != lessons[2].ScheduledOn {
		t.Errorf("planned lessons should stay put, got %s and %s",
			second.ScheduledOn, third.ScheduledOn)
	}
	if got := strings.Count(fragment, `id="day-`); got != 2 {
		t.Errorf("expected only the two days of the move, got %d", got)
	}
}

// mondayWeeksOut is the Monday of a week far enough ahead that a plan starting
// there is still entirely in the future whenever these tests are run.
func mondayWeeksOut(weeks int) string {
	return weekStart(parseDate(addDays(today(), 7*weeks))).Format(dateLayout)
}

// Pushing from a filtered week must send her back to that child, not the
// whole-family view she just left.
func TestPushFromFilteredPlannerKeepsTheKid(t *testing.T) {
	ta := newTestApp(t)
	kid, _, lessons := ta.applyThreeMathLessons()
	week := weekStart(parseDate(lessons[0].ScheduledOn)).Format(dateLayout)
	back := plannerURL(week, kid)

	status, body := ta.get("/planner?week=" + week + "&kid=" + itoa64(kid))
	if status != http.StatusOK {
		t.Fatalf("planner returned %d", status)
	}
	mustContain(t, body, `name="back" value="`+html.EscapeString(back)+`"`, "push back link")

	req, err := http.NewRequest(http.MethodPost, ta.server.URL+"/lessons/"+itoa64(lessons[0].ID)+"/push",
		strings.NewReader(url.Values{"back": {back}}.Encode()))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{
		Jar: ta.client.Jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("push returned %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != back {
		t.Fatalf("push redirected to %q, want %q", got, back)
	}

	status, body = ta.get(back)
	if status != http.StatusOK {
		t.Fatalf("filtered planner returned %d", status)
	}
	mustContain(t, body, `Week planner`, "filtered planner")
	if !strings.Contains(body, `class="filter-chip current"`) {
		t.Fatal("expected a selected kid filter chip")
	}
	if strings.Contains(body, `href="/planner?week=`+week+`" class="filter-chip current"`) {
		t.Error("push dumped the view back onto all kids")
	}
	if !strings.Contains(body, `&amp;kid=`+itoa64(kid)+`"`) &&
		!strings.Contains(body, `&kid=`+itoa64(kid)+`"`) {
		t.Errorf("expected the kid=%d filter to remain on the page", kid)
	}
}

func TestPauseUntilRelayoutsRemaining(t *testing.T) {
	ta := newTestApp(t)
	_, _, lessons := ta.applyThreeMathLessons()
	asgID := lessons[0].AssignmentID

	if err := ta.store.SetLessonStatus(lessons[0].ID, StatusDone); err != nil {
		t.Fatalf("marking first done: %v", err)
	}

	status, _ := ta.post("/assignments/"+itoa64(asgID)+"/pause", url.Values{
		"resume_on": {"2026-09-14"},
		"back":      {"/assignments/" + itoa64(asgID)},
	})
	if status != http.StatusOK {
		t.Fatalf("pause returned %d", status)
	}

	first, _ := ta.store.Lesson(lessons[0].ID)
	if first.ScheduledOn != "2026-08-26" {
		t.Errorf("done lesson should stay on 2026-08-26, got %s", first.ScheduledOn)
	}
	second, _ := ta.store.Lesson(lessons[1].ID)
	third, _ := ta.store.Lesson(lessons[2].ID)
	if second.ScheduledOn != "2026-09-14" || third.ScheduledOn != "2026-09-15" {
		t.Errorf("remaining should land on Sep 14–15, got %s, %s", second.ScheduledOn, third.ScheduledOn)
	}
}

func TestBackfillGroupsSameBatchAndLeavesOneOffs(t *testing.T) {
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

	created := "2026-08-26T12:00:00Z"
	titles := []string{"Place value", "Addition", "Subtraction"}
	dates := []string{"2026-08-26", "2026-08-27", "2026-08-28"}
	for i, title := range titles {
		ta.insertUnassignedLesson(kid, subject, dates[i], title, created)
	}
	oneOffID := ta.insertUnassignedLesson(kid, subject, "2026-09-01", "Extra drill", "2026-08-26T12:00:01Z")

	if err := ta.store.BackfillPlanAssignments(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	grouped, err := ta.store.LessonsInRange("2026-08-26", "2026-08-28", kid, subject)
	if err != nil {
		t.Fatalf("listing grouped: %v", err)
	}
	if len(grouped) != 3 {
		t.Fatalf("expected 3 grouped lessons, got %d", len(grouped))
	}
	if grouped[0].AssignmentID == 0 {
		t.Fatal("backfill should assign the batch")
	}
	if grouped[0].AssignmentID != grouped[2].AssignmentID {
		t.Fatal("batch should share one assignment")
	}
	asg, err := ta.store.Assignment(grouped[0].AssignmentID)
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	if asg.Name != "3rd grade Math" || asg.PlanID != planID {
		t.Errorf("expected plan match, got %+v", asg)
	}
	if grouped[0].Sequence != 1 || grouped[2].Sequence != 3 {
		t.Errorf("sequence: %d, %d, %d", grouped[0].Sequence, grouped[1].Sequence, grouped[2].Sequence)
	}

	oneOff, err := ta.store.Lesson(oneOffID)
	if err != nil {
		t.Fatalf("one-off: %v", err)
	}
	if oneOff.AssignmentID != 0 {
		t.Errorf("one-off should stay unassigned, got assignment %d", oneOff.AssignmentID)
	}
}

func TestBackfillMatchesPlanPrefixWhenLessonsWereRemoved(t *testing.T) {
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

	created := "2026-08-26T12:00:00Z"
	ta.insertUnassignedLesson(kid, subject, "2026-08-26", "Place value", created)
	ta.insertUnassignedLesson(kid, subject, "2026-08-27", "Addition", created)

	if err := ta.store.BackfillPlanAssignments(); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	lessons, err := ta.store.LessonsInRange("2026-08-26", "2026-08-27", kid, subject)
	if err != nil || len(lessons) != 2 {
		t.Fatalf("lessons: %d (err %v)", len(lessons), err)
	}
	asg, err := ta.store.Assignment(lessons[0].AssignmentID)
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	if asg.Name != "3rd grade Math" || asg.PlanID != planID {
		t.Errorf("expected prefix match to the plan, got %+v", asg)
	}
}

func (ta *testApp) applyThreeMathLessons() (kid, subject int64, lessons []Lesson) {
	ta.t.Helper()
	return ta.applyThreeMathLessonsFrom("2026-08-26")
}

// applyThreeMathLessonsFrom takes the start date because pulling a plan
// forward only means anything when the work is still ahead of today, which a
// fixed date in the calendar stops being.
func (ta *testApp) applyThreeMathLessonsFrom(start string) (kid, subject int64, lessons []Lesson) {
	ta.t.Helper()
	kid = ta.addKid("Mia")
	subject = ta.mathSubjectID()

	status, _ := ta.post("/curriculum", url.Values{
		"name":       {"3rd grade Math"},
		"subject_id": {itoa64(subject)},
	})
	if status != http.StatusOK {
		ta.t.Fatalf("creating plan returned %d", status)
	}
	plans, err := ta.store.CurriculumPlans()
	if err != nil || len(plans) != 1 {
		ta.t.Fatalf("expected 1 plan, got %d (err %v)", len(plans), err)
	}
	planID := plans[0].ID
	for _, title := range []string{"Place value", "Addition", "Subtraction"} {
		ta.post("/curriculum/"+itoa64(planID)+"/items", url.Values{"title": {title}})
	}
	status, _ = ta.post("/curriculum/"+itoa64(planID)+"/apply", url.Values{
		"kid_id":  {itoa64(kid)},
		"start":   {start},
		"weekday": {"1", "2", "3", "4", "5"},
	})
	if status != http.StatusOK {
		ta.t.Fatalf("applying plan returned %d", status)
	}
	dates, err := occurrenceDates(start, "", 3, parseWeekdays(schoolWeekdays))
	if err != nil {
		ta.t.Fatalf("expected dates: %v", err)
	}
	lessons, err = ta.store.LessonsInRange(dates[0], dates[2], kid, subject)
	if err != nil {
		ta.t.Fatalf("listing applied lessons: %v", err)
	}
	if len(lessons) != 3 {
		ta.t.Fatalf("expected 3 lessons, got %d", len(lessons))
	}
	return kid, subject, lessons
}

func (ta *testApp) insertUnassignedLesson(kid, subject int64, date, title, createdAt string) int64 {
	ta.t.Helper()
	res, err := ta.store.db().Exec(`INSERT INTO lessons
		(kid_id, subject_id, school_year_id, series_id, assignment_id, sequence,
		 scheduled_on, status, title, minutes, notes, completed_at, created_at)
		VALUES (?, ?, NULL, NULL, NULL, 0, ?, ?, ?, 0, '', NULL, ?)`,
		kid, subject, date, StatusPlanned, title, createdAt)
	if err != nil {
		ta.t.Fatalf("inserting unassigned lesson: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		ta.t.Fatalf("lesson id: %v", err)
	}
	return id
}
