package main

import (
	"archive/zip"
	"errors"
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
	"time"
)

func TestContainedPath(t *testing.T) {
	root := t.TempDir()
	got, ok := containedPath(root, "lesson..2.pdf")
	if !ok || filepath.Base(got) != "lesson..2.pdf" {
		t.Fatalf("lesson..2.pdf: %q ok=%v", got, ok)
	}
	if _, ok := containedPath(root, filepath.Join("..", "..", "control.db")); ok {
		t.Fatal("parent path was accepted")
	}
	if _, ok := containedPath(root, "/etc/passwd"); ok {
		t.Fatal("absolute path was accepted")
	}
	if _, ok := containedPath(root, string(os.PathSeparator)+"outside"); ok {
		t.Fatal("leading separator was accepted")
	}
}

func TestZipBudgetRatio(t *testing.T) {
	bomb := &zipBudget{compressed: 1000}
	if err := bomb.addBytes(zipBombFloor + 1); err == nil {
		t.Fatal("expected a bomb past the floor")
	}
	honest := &zipBudget{compressed: (zipBombFloor + 1) / 2}
	if err := honest.addBytes(zipBombFloor + 1); err != nil {
		t.Fatal(err)
	}
	underFloor := &zipBudget{compressed: 1}
	if err := underFloor.addBytes(1 << 20); err != nil {
		t.Fatal(err)
	}
}

func TestZipBombDoesNotLand(t *testing.T) {
	ta := newTestApp(t)
	if err := os.MkdirAll(ta.uploadDir, dirPerm); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(ta.uploadDir, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), filePerm); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "bomb.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	db, err := zw.Create("school.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Write([]byte("not a database")); err != nil {
		t.Fatal(err)
	}
	hdr := &zip.FileHeader{Name: "uploads/bomb.bin", Method: zip.Deflate}
	member, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(member, zeroReader{}, zipBombFloor+(1<<20)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	err = ta.importFamilyArchive(zipPath)
	if !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ta.uploadDir, "bomb.bin")); !os.IsNotExist(err) {
		t.Fatalf("bomb file left behind: %v", err)
	}
	body, err := os.ReadFile(keep)
	if err != nil || string(body) != "keep" {
		t.Fatalf("live upload changed: %v %q", err, body)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestFamilyExportIsOneCompleteZip(t *testing.T) {
	ta := newTestApp(t)
	if err := os.MkdirAll(ta.uploadDir, dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ta.uploadDir, "worksheet.txt"), []byte("page"), filePerm); err != nil {
		t.Fatal(err)
	}
	resp, err := ta.client.Get(ta.server.URL + "/settings/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("export status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(strings.NewReader(string(raw)), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["school.db"] || !names["uploads/worksheet.txt"] {
		t.Fatalf("archive names: %v", names)
	}
	yaml := false
	for name := range names {
		if strings.Contains(name, "curriculum/") && strings.HasSuffix(name, ".yaml") {
			yaml = true
		}
	}
	if !yaml {
		t.Fatalf("curriculum yaml missing: %v", names)
	}
}

func TestSecurityHeaders(t *testing.T) {
	ta := newTestApp(t)
	for _, path := range []string{"/healthz", "/"} {
		resp, err := ta.client.Get(ta.server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("%s nosniff: %q", path, got)
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Fatalf("%s csp: %q", path, csp)
		}
	}
}

func TestEmailLoginLockoutAndStoredRounds(t *testing.T) {
	ta := newHostedTestApp(t, "secret-invite")
	code, _, path := ta.postForm("/signup", url.Values{
		"email":            {"lock@example.com"},
		"password":         {"correct-horse"},
		"password_confirm": {"correct-horse"},
		"family_name":      {"Lock Family"},
		"invite_code":      {"secret-invite"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup: %d %s", code, path)
	}
	ta.post("/logout", nil)
	for i := 0; i < maxPINFails; i++ {
		code, body, _ := ta.postForm("/login", url.Values{
			"email": {"lock@example.com"}, "password": {"wrong-password"},
		})
		if code != http.StatusUnauthorized || !strings.Contains(body, "did not match") {
			t.Fatalf("attempt %d: %d %s", i, code, body)
		}
	}
	code, body, _ := ta.postForm("/login", url.Values{
		"email": {"lock@example.com"}, "password": {"correct-horse"},
	})
	if code != http.StatusUnauthorized || !strings.Contains(body, "did not match") {
		t.Fatalf("locked login: %d %s", code, body)
	}
	_, err := ta.control.pool.Exec(
		`UPDATE accounts SET locked_until = ? WHERE email = ?`,
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), "lock@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	code, _, path = ta.postForm("/login", url.Values{
		"email": {"lock@example.com"}, "password": {"correct-horse"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("login after window: %d %s", code, path)
	}
	var rounds string
	if err := ta.control.pool.QueryRow(
		`SELECT password_hash FROM accounts WHERE email = ?`, "lock@example.com",
	).Scan(&rounds); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rounds, "pbkdf2$600000$") {
		t.Fatalf("new hash %s", rounds)
	}
	rehash, err := hashPasswordAt("correct-horse", 200_000)
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword(rehash, "correct-horse") {
		t.Fatal("stored iteration count was ignored")
	}
	if !strings.HasPrefix(rehash, "pbkdf2$200000$") {
		t.Fatal(rehash)
	}
}

func hashPasswordAt(password string, rounds int) (string, error) {
	salt := []byte("0123456789abcdef")
	key, err := deriveKey(password, salt, rounds)
	if err != nil {
		return "", err
	}
	return "pbkdf2$200000$30313233343536373839616263646566$" + hexEncode(key), nil
}

func hexEncode(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}

func TestClosedSignupAndConfirmPassword(t *testing.T) {
	ta := newHostedTestApp(t, "")
	code, body, _ := ta.postForm("/signup", url.Values{
		"email":            {"open@example.com"},
		"password":         {"correct-horse"},
		"password_confirm": {"correct-horse"},
		"family_name":      {"Closed"},
	})
	if code != http.StatusForbidden || !strings.Contains(body, "Signup is closed") {
		t.Fatalf("closed signup: %d %s", code, body)
	}

	ta = newHostedTestApp(t, "secret-invite")
	code, body, _ = ta.postForm("/signup", url.Values{
		"email":            {"mis@example.com"},
		"password":         {"correct-horse"},
		"password_confirm": {"other-horse-x"},
		"family_name":      {"Mismatch"},
		"invite_code":      {"secret-invite"},
	})
	if code != http.StatusBadRequest || !strings.Contains(body, "did not match") {
		t.Fatalf("mismatch: %d %s", code, body)
	}
	var n int
	if err := ta.control.pool.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("account created on mismatch: %d", n)
	}
}

func TestOpenSignupConfirmAndReset(t *testing.T) {
	dir := t.TempDir()
	mail := &memoryMailer{}
	app, err := NewHostedApp(HostedConfig{
		DataRoot:        dir,
		BaseURL:         "http://example.test",
		AllowOpenSignup: true,
		Mailer:          mail,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.closeTenants)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	ta := &testApp{App: app, server: server, client: &http.Client{Jar: jar}, t: t}

	code, body, path := ta.postForm("/signup", url.Values{
		"email":            {"new@example.com"},
		"password":         {"correct-horse"},
		"password_confirm": {"correct-horse"},
		"family_name":      {"New Family"},
	})
	if code != http.StatusAccepted || path != "/signup" || !strings.Contains(body, "Check your email") {
		t.Fatalf("open signup: %d %s %s", code, path, body)
	}
	code, body, path = ta.getFollow("/")
	if path == "/" && !strings.Contains(body, "Sign in") && !strings.Contains(body, "Check your email") {
		t.Fatal("session issued before confirm")
	}
	msg, ok := mail.last()
	if !ok {
		t.Fatal("no confirm mail")
	}
	token := mailToken(msg.Body)
	code, _, path = ta.getFollow("/verify-email?token=" + token)
	if code != 200 || path != "/" {
		t.Fatalf("verify: %d %s", code, path)
	}
	var verified string
	if err := app.control.pool.QueryRow(
		`SELECT COALESCE(email_verified_at, '') FROM accounts WHERE email = ?`, "new@example.com",
	).Scan(&verified); err != nil || verified == "" {
		t.Fatalf("verified %q err %v", verified, err)
	}

	ta.post("/logout", nil)
	_, unknown, _ := ta.postForm("/forgot-password", url.Values{"email": {"nobody@example.com"}})
	_, known, _ := ta.postForm("/forgot-password", url.Values{"email": {"new@example.com"}})
	want := "If that email is registered, we sent a reset link."
	if !strings.Contains(unknown, want) || !strings.Contains(known, want) {
		t.Fatalf("forgot messages:\nunknown %s\nknown %s", unknown, known)
	}
	msg, ok = mail.last()
	if !ok {
		t.Fatal("no reset mail")
	}
	token = mailToken(msg.Body)
	code, body, _ = ta.postForm("/reset-password", url.Values{
		"token":            {token},
		"password":         {"correct-horse"},
		"password_confirm": {"nope-nope-nope"},
	})
	if code != http.StatusBadRequest || !strings.Contains(body, "did not match") {
		t.Fatalf("reset mismatch: %d %s", code, body)
	}
	code, _, path = ta.postForm("/reset-password", url.Values{
		"token":            {token},
		"password":         {"brand-new-password"},
		"password_confirm": {"brand-new-password"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("reset: %d %s", code, path)
	}
	ta.post("/logout", nil)
	code, _, path = ta.postForm("/login", url.Values{
		"email": {"new@example.com"}, "password": {"brand-new-password"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("login with new password: %d %s", code, path)
	}
}

func TestEmailSigninCode(t *testing.T) {
	dir := t.TempDir()
	mail := &memoryMailer{}
	app, err := NewHostedApp(HostedConfig{
		DataRoot:   dir,
		BaseURL:    "http://example.test",
		InviteCode: "secret-invite",
		Mailer:     mail,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.closeTenants)
	server := httptest.NewServer(app.Routes())
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	ta := &testApp{App: app, server: server, client: &http.Client{Jar: jar}, t: t}
	code, _, path := ta.postForm("/signup", url.Values{
		"email":            {"two@example.com"},
		"password":         {"correct-horse"},
		"password_confirm": {"correct-horse"},
		"family_name":      {"Two Factor"},
		"invite_code":      {"secret-invite"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup: %d %s", code, path)
	}
	code, _, path = ta.postForm("/settings/email-2fa", url.Values{"email_2fa": {"1"}})
	if code != 200 || !strings.Contains(path, "/settings/access") {
		t.Fatalf("enable 2fa: %d %s", code, path)
	}
	ta.post("/logout", nil)
	code, body, path := ta.postForm("/login", url.Values{
		"email": {"two@example.com"}, "password": {"correct-horse"},
	})
	if path == "/" || !strings.Contains(body, "Sign-in code") {
		t.Fatalf("expected challenge, got %d %s %s", code, path, body)
	}
	code, body = ta.get("/planner")
	if !strings.Contains(body, "Sign in") && !strings.Contains(body, "Sign-in code") {
		t.Fatalf("planner without session: %d %s", code, body)
	}
	msg, ok := mail.last()
	if !ok || !strings.Contains(msg.Body, "sign-in code is ") {
		t.Fatalf("mail: %+v", msg)
	}
	sent := signinCodeFromMail(msg.Body)
	for i := 0; i < signinCodeLimit; i++ {
		code, _, _ = ta.postForm("/login/code", url.Values{"code": {"000000"}})
		if code != http.StatusUnauthorized {
			t.Fatalf("bad code %d: %d", i, code)
		}
	}
	code, _, path = ta.postForm("/login/code", url.Values{"code": {sent}})
	if path == "/" {
		t.Fatal("burned challenge still signed in")
	}

	code, body, _ = ta.postForm("/login", url.Values{
		"email": {"two@example.com"}, "password": {"correct-horse"},
	})
	msg, _ = mail.last()
	sent = signinCodeFromMail(msg.Body)
	code, _, path = ta.postForm("/login/code", url.Values{"code": {sent}})
	if code != 200 || path != "/" {
		t.Fatalf("good code: %d %s", code, path)
	}
}

func TestOwnersFromBeforeVerificationAreConfirmed(t *testing.T) {
	ta := newHostedTestApp(t, "secret-invite")
	code, _, path := ta.postForm("/signup", url.Values{
		"email":            {"wife@example.com"},
		"password":         {"correct-horse"},
		"password_confirm": {"correct-horse"},
		"family_name":      {"Already Here"},
		"invite_code":      {"secret-invite"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup: %d %s", code, path)
	}
	if _, err := ta.control.pool.Exec(
		`UPDATE accounts SET email_verified_at = NULL WHERE email = ?`, "wife@example.com",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.control.pool.Exec(`DELETE FROM email_tokens`); err != nil {
		t.Fatal(err)
	}
	if err := ta.control.grandfatherUnverifiedOwners(); err != nil {
		t.Fatal(err)
	}
	var verified string
	if err := ta.control.pool.QueryRow(
		`SELECT COALESCE(email_verified_at, '') FROM accounts WHERE email = ?`, "wife@example.com",
	).Scan(&verified); err != nil {
		t.Fatal(err)
	}
	if verified == "" {
		t.Fatal("owner from before verification mail was left unconfirmed")
	}

	raw, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := ta.control.pool.QueryRow(
		`SELECT id FROM accounts WHERE email = ?`, "wife@example.com",
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.control.pool.Exec(
		`UPDATE accounts SET email_verified_at = NULL WHERE id = ?`, id,
	); err != nil {
		t.Fatal(err)
	}
	if err := ta.control.issueEmailToken(id, tokenConfirm, raw, "", confirmTokenTTL); err != nil {
		t.Fatal(err)
	}
	if err := ta.control.grandfatherUnverifiedOwners(); err != nil {
		t.Fatal(err)
	}
	if err := ta.control.pool.QueryRow(
		`SELECT COALESCE(email_verified_at, '') FROM accounts WHERE id = ?`, id,
	).Scan(&verified); err != nil {
		t.Fatal(err)
	}
	if verified != "" {
		t.Fatal("an account with a confirm token should stay unverified")
	}
}

func TestKidAvatarScope(t *testing.T) {
	ta := newHostedTestApp(t, "secret-invite")
	code, _, path := ta.postForm("/signup", url.Values{
		"email":            {"kids@example.com"},
		"password":         {"correct-horse"},
		"password_confirm": {"correct-horse"},
		"family_name":      {"Kids"},
		"invite_code":      {"secret-invite"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("signup: %d %s", code, path)
	}
	for _, name := range []string{"Ada", "Bea"} {
		code, body := ta.post("/settings/kids", url.Values{"name": {name}, "color": {"#336699"}})
		if code >= 400 {
			t.Fatalf("add %s: %d %s", name, code, body)
		}
	}
	var familyID, slug string
	if err := ta.control.pool.QueryRow(`SELECT id, COALESCE(slug, '') FROM families`).Scan(&familyID, &slug); err != nil {
		t.Fatal(err)
	}
	tenant, err := ta.tenantApp(familyID)
	if err != nil {
		t.Fatal(err)
	}
	kids, err := tenant.store.Kids(false)
	if err != nil || len(kids) < 2 {
		t.Fatalf("kids: %v %d", err, len(kids))
	}
	code, body := ta.post("/settings/pins", url.Values{
		"display_name": {"Ada"},
		"username":     {"ada"},
		"pin":          {"1234"},
		"role":         {"kid"},
		"kid_id":       {strconv.FormatInt(kids[0].ID, 10)},
	})
	if code >= 400 {
		t.Fatalf("pin: %d %s", code, body)
	}
	jar, _ := cookiejar.New(nil)
	kid := &testApp{App: ta.App, server: ta.server, client: &http.Client{Jar: jar}, t: t}
	code, body, path = kid.postForm("/login", url.Values{
		"method":      {"pin"},
		"family_slug": {slug},
		"username":    {"ada"},
		"pin":         {"1234"},
	})
	if code != 200 || path != "/" {
		t.Fatalf("kid login: %d %s %s", code, path, body)
	}
	own := "/avatars/kids/" + strconv.FormatInt(kids[0].ID, 10)
	code, _ = kid.get(own)
	if code == http.StatusForbidden {
		t.Fatalf("own avatar forbidden")
	}
	other := "/avatars/kids/" + strconv.FormatInt(kids[1].ID, 10)
	code, _ = kid.get(other)
	if code != http.StatusForbidden {
		t.Fatalf("other kid avatar: %d", code)
	}
	code, _ = kid.get("/avatars/adults/1")
	if code != http.StatusForbidden {
		t.Fatalf("adult avatar: %d", code)
	}
}

func mailToken(body string) string {
	const key = "token="
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	rest := body[i+len(key):]
	if n := strings.IndexAny(rest, " \n\r"); n >= 0 {
		rest = rest[:n]
	}
	return rest
}

func signinCodeFromMail(body string) string {
	const key = "sign-in code is "
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	rest := body[i+len(key):]
	if len(rest) >= 6 {
		return rest[:6]
	}
	return rest
}

func (ta *testApp) getFollow(path string) (int, string, string) {
	ta.t.Helper()
	resp, err := ta.client.Get(ta.server.URL + path)
	if err != nil {
		ta.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Request.URL.Path
}
