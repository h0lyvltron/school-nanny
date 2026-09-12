package main

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
	code, body = ta.get("/settings")
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
	code, body = ta.get("/settings")
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
	code, body = ta.get("/settings")
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

	code, _ := ta.postFile("/settings/import", "archive", "family.zip", zipBytes)
	if code != 200 {
		t.Fatalf("import status %d", code)
	}
	_, body := ta.get("/settings")
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
