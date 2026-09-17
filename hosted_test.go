package main

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
