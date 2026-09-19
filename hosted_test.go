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

func newHostedTestApp(t *testing.T, invite string) *testApp {
	t.Helper()
	dir := t.TempDir()
	app, err := NewHostedApp(HostedConfig{
		DataRoot:     dir,
		BaseURL:      "http://example.test",
		InviteCode:   invite,
		CookieSecure: false,
	})
	if err != nil {
		t.Fatalf("NewHostedApp: %v", err)
	}
	t.Cleanup(app.closeTenants)

	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &testApp{App: app, server: server, client: &http.Client{Jar: jar}, t: t}
}

func (ta *testApp) postForm(path string, values url.Values) (int, string, string) {
	ta.t.Helper()
	resp, err := ta.client.PostForm(ta.server.URL+path, values)
	if err != nil {
		ta.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Request.URL.Path
}

func TestHealthz(t *testing.T) {
	ta := newTestApp(t)
	code, body := ta.get("/healthz")
	if code != 200 || !strings.Contains(body, "ok") {
		t.Fatalf("healthz: %d %q", code, body)
	}
}

func TestHostedSignupLoginIsolation(t *testing.T) {
	ta := newHostedTestApp(t, "secret-invite")

	code, body := ta.get("/")
	if code != 200 || !strings.Contains(body, "Sign in") {
		// redirect to login follows; jar client may land on login
		code, body = ta.get("/login")
		if code != 200 || !strings.Contains(body, "Sign in") {
			t.Fatalf("login page: %d %q", code, body)
		}
	}

	code, _, path := ta.postForm("/signup", url.Values{
		"email":       {"alpha@example.com"},
		"password":    {"alpha-pass-word"},
		"family_name": {"Alpha Family"},
		"invite_code": {"secret-invite"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup alpha: status=%d path=%s", code, path)
	}

	// Seed a kid in alpha's store via settings.
	code, _ = ta.post("/settings/kids", url.Values{
		"name":  {"Alice"},
		"grade": {"1"},
		"color": {"#aabbcc"},
	})
	if code != 200 {
		t.Fatalf("add kid: %d", code)
	}
	code, body = ta.get("/settings/people")
	if code != 200 || !strings.Contains(body, "Alice") {
		t.Fatalf("alpha settings missing Alice: %d", code)
	}

	// Sign out and create a second family.
	ta.post("/logout", url.Values{})
	code, _, path = ta.postForm("/signup", url.Values{
		"email":       {"beta@example.com"},
		"password":    {"beta-pass-word"},
		"family_name": {"Beta Family"},
		"invite_code": {"secret-invite"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup beta: status=%d path=%s", code, path)
	}
	code, body = ta.get("/settings/people")
	if code != 200 {
		t.Fatalf("beta settings: %d", code)
	}
	if strings.Contains(body, "Alice") {
		t.Fatal("beta family can see alpha's kid — tenancy broken")
	}

	// Wrong invite blocked.
	ta.post("/logout", url.Values{})
	code, body, _ = ta.postForm("/signup", url.Values{
		"email":       {"gamma@example.com"},
		"password":    {"gamma-pass-word"},
		"family_name": {"Gamma"},
		"invite_code": {"wrong"},
	})
	if code != 403 && !strings.Contains(body, "invite") {
		t.Fatalf("expected invite failure, got %d %q", code, body)
	}

	// Login works for alpha.
	code, _, path = ta.postForm("/login", url.Values{
		"email":    {"alpha@example.com"},
		"password": {"alpha-pass-word"},
	})
	if path != "/" {
		t.Fatalf("alpha login path=%s code=%d", path, code)
	}
	code, body = ta.get("/settings/people")
	if !strings.Contains(body, "Alice") {
		t.Fatalf("alpha lost Alice after re-login")
	}

	// Per-family dirs exist.
	entries, _ := os.ReadDir(filepath.Join(ta.dataRoot, familiesDirName))
	if len(entries) < 2 {
		t.Fatalf("expected >=2 family dirs, got %d", len(entries))
	}
}

func TestHostedFamilyExportImport(t *testing.T) {
	ta := newHostedTestApp(t, "")
	ta.postForm("/signup", url.Values{
		"email":       {"exp@example.com"},
		"password":    {"export-pass-word"},
		"family_name": {"Export Family"},
	})
	ta.post("/settings/kids", url.Values{
		"name":  {"Eve"},
		"grade": {"2"},
		"color": {"#112233"},
	})

	resp, err := ta.client.Get(ta.server.URL + "/settings/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("export status %d", resp.StatusCode)
	}
	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(zipBytes) < 100 {
		t.Fatalf("export too small: %d", len(zipBytes))
	}

	// Change data then import archive back.
	ta.post("/settings/kids", url.Values{
		"name":  {"Other"},
		"grade": {"3"},
		"color": {"#445566"},
	})

	code, body := ta.postFile("/settings/import", "archive", "family.zip", zipBytes, url.Values{
		"mode":            {"replace"},
		"confirm_replace": {"REPLACE"},
	})
	if code != 200 {
		t.Fatalf("import status %d body=%s", code, body)
	}
	_, body = ta.get("/settings/people")
	if !strings.Contains(body, "Eve") {
		t.Fatal("import did not restore Eve")
	}
}

func TestHostedAdoptsLegacySchoolDB(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, dbFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting("probe", "legacy"); err != nil {
		t.Fatal(err)
	}
	store.Close()

	app, err := NewHostedApp(HostedConfig{DataRoot: dir, CookieSecure: false})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.closeTenants)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	ta := &testApp{App: app, server: server, client: &http.Client{Jar: jar}, t: t}

	ta.postForm("/signup", url.Values{
		"email":       {"legacy@example.com"},
		"password":    {"legacy-pass-word"},
		"family_name": {"Legacy"},
	})

	if _, err := os.Stat(filepath.Join(dir, dbFileName)); !os.IsNotExist(err) {
		t.Fatalf("legacy db should have moved, err=%v", err)
	}
	// Find family dir and confirm setting survived.
	found := false
	filepath.Walk(filepath.Join(dir, familiesDirName), func(path string, info os.FileInfo, err error) error {
		if info != nil && info.Name() == dbFileName {
			s, err := OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			v, _ := s.Setting("probe")
			if v == "legacy" {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Fatal("legacy setting not found in family db")
	}
}

func TestHostedPINLoginAndOwnerGates(t *testing.T) {
	ta := newHostedTestApp(t, "")
	code, _, path := ta.postForm("/signup", url.Values{
		"email":       {"owner@example.com"},
		"password":    {"owner-pass-word"},
		"family_name": {"Pin Family"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup: status=%d path=%s", code, path)
	}

	fams := mustListFamilies(t, ta)
	if len(fams) != 1 {
		t.Fatalf("expected 1 family, got %d", len(fams))
	}
	fam := &fams[0]

	code, _ = ta.post("/settings/kids", url.Values{
		"name":  {"Sam"},
		"grade": {"3"},
		"color": {"#aabbcc"},
	})
	if code != 200 {
		t.Fatalf("add kid: %d", code)
	}
	_, settingsBody := ta.get("/settings/access")
	if !strings.Contains(settingsBody, "Household logins") {
		t.Fatalf("settings missing household panel")
	}
	kidID := firstKidID(t, ta)

	code, body, path := ta.postForm("/settings/pins", url.Values{
		"display_name": {"Sam"},
		"username":     {"sam"},
		"pin":          {"1234"},
		"role":         {"kid"},
		"kid_id":       {strconv.FormatInt(kidID, 10)},
	})
	if code != 200 || path != "/settings/access" || !strings.Contains(body, "PIN shown once") {
		t.Fatalf("create kid pin: status=%d path=%s", code, path)
	}

	code, body, path = ta.postForm("/settings/pins", url.Values{
		"display_name": {"Helper"},
		"username":     {"helper"},
		"pin":          {"5678"},
		"role":         {"caregiver"},
	})
	if code != 200 || !strings.Contains(body, "PIN shown once") {
		t.Fatalf("create caregiver pin: status=%d path=%s", code, path)
	}

	ta.post("/logout", url.Values{})

	code, _, path = ta.postForm("/login", url.Values{
		"method":      {"pin"},
		"family_slug": {fam.Slug},
		"username":    {"sam"},
		"pin":         {"1234"},
	})
	if path != "/" {
		t.Fatalf("kid pin login path=%s code=%d", path, code)
	}
	code, body = ta.get("/")
	if code != 200 || !strings.Contains(body, "Sam") {
		t.Fatalf("kid today missing Sam: %d", code)
	}
	code, _ = ta.get("/settings")
	if code != 403 {
		t.Fatalf("kid settings want 403 got %d", code)
	}
	code, _ = ta.get("/settings/export")
	if code != 403 {
		t.Fatalf("kid export want 403 got %d", code)
	}
	code, _ = ta.get("/history")
	if code != 403 {
		t.Fatalf("kid history want 403 got %d", code)
	}

	ta.post("/logout", url.Values{})
	code, _, path = ta.postForm("/login", url.Values{
		"method":      {"pin"},
		"family_slug": {fam.Slug},
		"username":    {"helper"},
		"pin":         {"5678"},
	})
	if path != "/" {
		t.Fatalf("caregiver login path=%s code=%d", path, code)
	}
	code, _ = ta.get("/settings")
	if code != 403 {
		t.Fatalf("caregiver settings want 403 got %d", code)
	}
	code, _ = ta.get("/curriculum")
	if code != 403 {
		t.Fatalf("caregiver curriculum want 403 got %d", code)
	}
	code, _ = ta.get("/history")
	if code != 403 {
		t.Fatalf("caregiver history want 403 got %d", code)
	}
	code, _ = ta.get("/")
	if code != 200 {
		t.Fatalf("caregiver today want 200 got %d", code)
	}
	code, _ = ta.get("/planner")
	if code != 200 {
		t.Fatalf("caregiver planner want 200 got %d", code)
	}
	code, _ = ta.get("/attendance")
	if code != 200 {
		t.Fatalf("caregiver attendance want 200 got %d", code)
	}

	ta.post("/logout", url.Values{})
	ta.postForm("/login", url.Values{
		"email":    {"owner@example.com"},
		"password": {"owner-pass-word"},
	})
	members, err := ta.control.ListMemberships(fam.ID)
	if err != nil {
		t.Fatal(err)
	}
	var kidMemID int64
	for _, m := range members {
		if m.Role == roleKid {
			kidMemID = m.ID
			break
		}
	}
	if kidMemID == 0 {
		t.Fatal("kid membership missing")
	}
	code, _, _ = ta.postForm("/settings/pins/"+strconv.FormatInt(kidMemID, 10)+"/revoke", url.Values{})
	if code != 200 {
		t.Fatalf("revoke: %d", code)
	}

	ta.post("/logout", url.Values{})
	code, body, path = ta.postForm("/login", url.Values{
		"method":      {"pin"},
		"family_slug": {fam.Slug},
		"username":    {"sam"},
		"pin":         {"1234"},
	})
	if path == "/" {
		t.Fatalf("revoked kid should not land on home; code=%d body=%q", code, body)
	}
}

func TestHostedCaregiverDayOpsAndKidOwnStatus(t *testing.T) {
	ta := newHostedTestApp(t, "")
	code, _, path := ta.postForm("/signup", url.Values{
		"email":       {"owner2@example.com"},
		"password":    {"owner-pass-word"},
		"family_name": {"Ops Family"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup: status=%d path=%s", code, path)
	}
	fams := mustListFamilies(t, ta)
	fam := &fams[0]
	tenant, err := ta.tenantApp(fam.ID)
	if err != nil {
		t.Fatal(err)
	}

	code, _ = ta.post("/settings/kids", url.Values{
		"name": {"Sam"}, "grade": {"3"}, "color": {"#aabbcc"},
	})
	if code != 200 {
		t.Fatalf("add Sam: %d", code)
	}
	code, _ = ta.post("/settings/kids", url.Values{
		"name": {"Riley"}, "grade": {"1"}, "color": {"#ccddee"},
	})
	if code != 200 {
		t.Fatalf("add Riley: %d", code)
	}
	kids, err := tenant.store.Kids(false)
	if err != nil || len(kids) < 2 {
		t.Fatalf("kids: %v len=%d", err, len(kids))
	}
	var sam, riley Kid
	for _, k := range kids {
		switch k.Name {
		case "Sam":
			sam = k
		case "Riley":
			riley = k
		}
	}
	if sam.ID == 0 || riley.ID == 0 {
		t.Fatalf("missing kids: sam=%d riley=%d", sam.ID, riley.ID)
	}
	subjects, err := tenant.store.Subjects(false)
	if err != nil || len(subjects) == 0 {
		t.Fatalf("subjects: %v", err)
	}
	subjectID := subjects[0].ID
	today := time.Now().Format("2006-01-02")

	code, _ = ta.post("/lessons", url.Values{
		"kid_id":       {strconv.FormatInt(sam.ID, 10)},
		"subject_id":   {strconv.FormatInt(subjectID, 10)},
		"scheduled_on": {today},
		"title":        {"Sam math"},
		"minutes":      {"30"},
	})
	if code != 200 {
		t.Fatalf("create Sam lesson: %d", code)
	}
	code, _ = ta.post("/lessons", url.Values{
		"kid_id":       {strconv.FormatInt(riley.ID, 10)},
		"subject_id":   {strconv.FormatInt(subjectID, 10)},
		"scheduled_on": {today},
		"title":        {"Riley reading"},
		"minutes":      {"20"},
	})
	if code != 200 {
		t.Fatalf("create Riley lesson: %d", code)
	}
	lessons, err := tenant.store.LessonsBetween(today, today, 0)
	if err != nil {
		t.Fatal(err)
	}
	var samLesson, rileyLesson Lesson
	for _, l := range lessons {
		switch l.KidID {
		case sam.ID:
			samLesson = l
		case riley.ID:
			rileyLesson = l
		}
	}
	if samLesson.ID == 0 || rileyLesson.ID == 0 {
		t.Fatalf("lessons missing: sam=%d riley=%d", samLesson.ID, rileyLesson.ID)
	}

	// Attach a file to Riley's lesson for sibling-file deny checks.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("owner_type", OwnerLesson)
	_ = writer.WriteField("lesson_id", strconv.FormatInt(rileyLesson.ID, 10))
	part, err := writer.CreateFormFile("file", "riley-notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("riley only"))
	_ = writer.Close()
	resp, err := ta.client.Post(ta.server.URL+"/files", writer.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	rileyFiles, err := tenant.store.AttachmentsForLesson(rileyLesson.ID)
	if err != nil || len(rileyFiles) == 0 {
		t.Fatalf("riley file: %v len=%d", err, len(rileyFiles))
	}
	rileyFileID := rileyFiles[0].ID

	adults, err := tenant.store.Adults(false)
	if err != nil || len(adults) == 0 {
		t.Fatalf("adults: %v", err)
	}
	adultID := adults[0].ID

	code, body, _ := ta.postForm("/settings/pins", url.Values{
		"display_name": {"Sam"},
		"username":     {"sam"},
		"pin":          {"1234"},
		"role":         {"kid"},
		"kid_id":       {strconv.FormatInt(sam.ID, 10)},
	})
	if code != 200 || !strings.Contains(body, "PIN shown once") {
		t.Fatalf("create kid pin: status=%d", code)
	}
	code, body, _ = ta.postForm("/settings/pins", url.Values{
		"display_name": {"Helper"},
		"username":     {"helper"},
		"pin":          {"5678"},
		"role":         {"caregiver"},
	})
	if code != 200 || !strings.Contains(body, "PIN shown once") {
		t.Fatalf("create caregiver pin: status=%d", code)
	}

	// --- Caregiver day-ops ---
	ta.post("/logout", url.Values{})
	code, _, path = ta.postForm("/login", url.Values{
		"method": {"pin"}, "family_slug": {fam.Slug},
		"username": {"helper"}, "pin": {"5678"},
	})
	if path != "/" {
		t.Fatalf("caregiver login path=%s code=%d", path, code)
	}
	for _, p := range []string{"/", "/planner", "/attendance",
		"/kids/" + strconv.FormatInt(sam.ID, 10),
		"/lessons/" + strconv.FormatInt(samLesson.ID, 10)} {
		if code, _ = ta.get(p); code != 200 {
			t.Fatalf("caregiver GET %s want 200 got %d", p, code)
		}
	}
	code, _ = ta.postHTMX("/lessons/"+strconv.FormatInt(samLesson.ID, 10)+"/status", url.Values{
		"status": {StatusDone}, "minutes": {"25"},
	})
	if code != 200 {
		t.Fatalf("caregiver lesson status want 200 got %d", code)
	}
	code, _ = ta.post("/attendance", url.Values{
		"kid_id": {strconv.FormatInt(sam.ID, 10)}, "attended_on": {today}, "status": {"present"},
	})
	if code != 200 {
		t.Fatalf("caregiver attendance POST want 200 got %d", code)
	}
	for _, check := range []struct {
		name string
		fn   func() int
	}{
		{"settings POST", func() int {
			c, _ := ta.post("/settings/kids", url.Values{"name": {"Nope"}, "grade": {"1"}, "color": {"#111111"}})
			return c
		}},
		{"curriculum GET", func() int { c, _ := ta.get("/curriculum"); return c }},
		{"history GET", func() int { c, _ := ta.get("/history"); return c }},
		{"lesson delete", func() int {
			c, _ := ta.post("/lessons/"+strconv.FormatInt(rileyLesson.ID, 10)+"/delete", url.Values{})
			return c
		}},
		{"adult event", func() int {
			c, _ := ta.post("/adults/"+strconv.FormatInt(adultID, 10)+"/events", url.Values{
				"title": {"Blocked"}, "starts_on": {today},
			})
			return c
		}},
		{"file delete", func() int {
			c, _ := ta.post("/files/"+strconv.FormatInt(rileyFileID, 10)+"/delete", url.Values{})
			return c
		}},
	} {
		if c := check.fn(); c != 403 {
			t.Fatalf("caregiver %s want 403 got %d", check.name, c)
		}
	}

	// --- Kid own-status ---
	ta.post("/logout", url.Values{})
	code, _, path = ta.postForm("/login", url.Values{
		"method": {"pin"}, "family_slug": {fam.Slug},
		"username": {"sam"}, "pin": {"1234"},
	})
	if path != "/" {
		t.Fatalf("kid login path=%s code=%d", path, code)
	}
	if code, _ = ta.get("/kids/" + strconv.FormatInt(sam.ID, 10)); code != 200 {
		t.Fatalf("kid own page want 200 got %d", code)
	}
	if code, _ = ta.get("/lessons/" + strconv.FormatInt(samLesson.ID, 10)); code != 200 {
		t.Fatalf("kid own lesson want 200 got %d", code)
	}
	code, _ = ta.postHTMX("/lessons/"+strconv.FormatInt(samLesson.ID, 10)+"/status", url.Values{
		"status": {StatusPlanned}, "minutes": {"30"},
	})
	if code != 200 {
		t.Fatalf("kid own status want 200 got %d", code)
	}
	for _, check := range []struct {
		name string
		fn   func() int
	}{
		{"sibling kid", func() int {
			c, _ := ta.get("/kids/" + strconv.FormatInt(riley.ID, 10))
			return c
		}},
		{"sibling lesson", func() int {
			c, _ := ta.get("/lessons/" + strconv.FormatInt(rileyLesson.ID, 10))
			return c
		}},
		{"sibling subject", func() int {
			c, _ := ta.get("/kids/" + strconv.FormatInt(riley.ID, 10) + "/subjects/" + strconv.FormatInt(subjectID, 10))
			return c
		}},
		{"attendance POST", func() int {
			c, _ := ta.post("/attendance", url.Values{
				"kid_id": {strconv.FormatInt(sam.ID, 10)}, "attended_on": {today}, "status": {"present"},
			})
			return c
		}},
		{"settings", func() int { c, _ := ta.get("/settings"); return c }},
		{"curriculum", func() int { c, _ := ta.get("/curriculum"); return c }},
		{"adult page", func() int {
			c, _ := ta.get("/adults/" + strconv.FormatInt(adultID, 10))
			return c
		}},
		{"other kid file", func() int {
			c, _ := ta.get("/files/" + strconv.FormatInt(rileyFileID, 10))
			return c
		}},
	} {
		if c := check.fn(); c != 403 {
			t.Fatalf("kid %s want 403 got %d", check.name, c)
		}
	}

	// Owner happy paths still work.
	ta.post("/logout", url.Values{})
	code, _, path = ta.postForm("/login", url.Values{
		"email": {"owner2@example.com"}, "password": {"owner-pass-word"},
	})
	if path != "/" {
		t.Fatalf("owner login path=%s code=%d", path, code)
	}
	if code, _ = ta.get("/curriculum"); code != 200 {
		t.Fatalf("owner curriculum want 200 got %d", code)
	}
	if code, _ = ta.get("/settings/people"); code != 200 {
		t.Fatalf("owner settings want 200 got %d", code)
	}
}

func TestSVGDownloadForcesAttachment(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("owner_type", OwnerResource)
	_ = writer.WriteField("kid_id", itoa64(kid))
	_ = writer.WriteField("subject_id", itoa64(subject))
	part, err := writer.CreateFormFile("file", "icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	_ = writer.Close()
	resp, err := ta.client.Post(ta.server.URL+"/files", writer.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	files, err := ta.store.ResourceAttachments(kid, subject)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected 1 file, got %d err=%v", len(files), err)
	}
	req, err := http.NewRequest(http.MethodGet, ta.server.URL+"/files/"+itoa64(files[0].ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = ta.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	disp := resp.Header.Get("Content-Disposition")
	if !strings.HasPrefix(disp, "attachment") {
		t.Fatalf("SVG must download as attachment, got %q", disp)
	}
}

func TestHostedPasswordChangeKillsOtherSessions(t *testing.T) {
	ta := newHostedTestApp(t, "")
	code, _, path := ta.postForm("/signup", url.Values{
		"email":       {"pw@example.com"},
		"password":    {"first-pass-word"},
		"family_name": {"PW Family"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup: %d %s", code, path)
	}

	otherJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	other := &testApp{App: ta.App, server: ta.server, client: &http.Client{Jar: otherJar}, t: t}
	code, _, path = other.postForm("/login", url.Values{
		"email": {"pw@example.com"}, "password": {"first-pass-word"},
	})
	if path != "/" {
		t.Fatalf("second session login path=%s code=%d", path, code)
	}
	if code, _ = other.get("/settings/people"); code != 200 {
		t.Fatalf("second session settings want 200 got %d", code)
	}

	code, _, path = ta.postForm("/settings/account-password", url.Values{
		"password": {"second-pass-word"},
	})
	if code != 200 || path != "/settings/access" {
		t.Fatalf("password change: status=%d path=%s", code, path)
	}
	// Changing password keeps the current browser logged in.
	if code, _ = ta.get("/settings/people"); code != 200 {
		t.Fatalf("current session should survive password change, got %d", code)
	}
	// Other sessions are killed and must re-authenticate.
	code, body := other.get("/settings/people")
	if code == 200 && strings.Contains(body, "People") {
		t.Fatal("other session still has access after password change")
	}
}

func mustListFamilies(t *testing.T, ta *testApp) []Family {
	t.Helper()
	rows, err := ta.control.pool.Query(`SELECT id, name, slug, created_at FROM families`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []Family
	for rows.Next() {
		var f Family
		var created string
		if err := rows.Scan(&f.ID, &f.Name, &f.Slug, &created); err != nil {
			t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

func firstKidID(t *testing.T, ta *testApp) int64 {
	t.Helper()
	// After signup the jar is on the owner session; open tenant via control family.
	fams := mustListFamilies(t, ta)
	if len(fams) == 0 {
		t.Fatal("no family")
	}
	tenant, err := ta.tenantApp(fams[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	kids, err := tenant.store.Kids(false)
	if err != nil || len(kids) == 0 {
		t.Fatalf("kids: %v len=%d", err, len(kids))
	}
	return kids[0].ID
}
