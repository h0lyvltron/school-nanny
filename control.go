package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

const (
	controlDBName   = "control.db"
	hostedSIDCookie = "school_nanny_sid"
	familiesDirName = "families"

	accountOwnerEmail  = "owner_email"
	accountPINPrincipal = "pin_principal"

	roleOwner     = "owner"
	roleCoParent  = "co_parent"
	roleTeacher   = "teacher"
	roleCaregiver = "caregiver"
	roleKid       = "kid"

	membershipActive  = "active"
	membershipRevoked = "revoked"

	authPassword = "password"
	authPIN      = "pin"

	minPINLen = 4
	maxPINLen = 8
	maxPINFails = 8
	pinLockMinutes = 15
)

var (
	errEmailTaken     = errors.New("email already registered")
	errBadCredentials = errors.New("bad credentials")
	errInviteRequired = errors.New("invite code required")
	errBadInvite      = errors.New("invite code did not match")
	errNoSession      = errors.New("no session")
	errPINLocked      = errors.New("pin locked")
	errBadPIN         = errors.New("bad pin")
	errUsernameTaken  = errors.New("username taken")
)

// ControlStore is the hosted multi-tenant control plane.
type ControlStore struct {
	pool *sql.DB
	path string
}

type Family struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
}

type Account struct {
	ID           int64
	Kind         string
	Email        string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time
}

// User is kept as an alias shape for older call sites; ID is the account id.
type User struct {
	ID           int64
	FamilyID     string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Membership struct {
	ID                 int64
	AccountID          int64
	FamilyID           string
	Role               string
	KidID              int64
	AdultID            int64
	CanManageKidLogins bool
	Status             string
	Username           string
	DisplayName        string
	CreatedAt          time.Time
}

type Session struct {
	ID                 string
	AccountID          int64
	UserID             int64 // same as AccountID; kept for call-site compatibility
	MembershipID       int64
	FamilyID           string
	AuthMethod         string
	Role               string
	KidID              int64
	CanManageKidLogins bool
	Email              string
	DisplayName        string
	ExpiresAt          time.Time
}

func (s *Session) IsOwner() bool { return s != nil && s.Role == roleOwner }
func (s *Session) IsKid() bool   { return s != nil && s.Role == roleKid }

func OpenControlStore(dbPath string) (*ControlStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + url.PathEscape(dbPath) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening control db: %w", err)
	}
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	if err := pool.Ping(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("opening control db: %w", err)
	}
	c := &ControlStore{pool: pool, path: dbPath}
	if err := c.migrate(); err != nil {
		pool.Close()
		return nil, err
	}
	return c, nil
}

func (c *ControlStore) Close() error {
	return c.pool.Close()
}

func (c *ControlStore) migrate() error {
	_, err := c.pool.Exec(`
CREATE TABLE IF NOT EXISTS families (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  email TEXT NOT NULL COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(email)
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL,
  family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expires_at ON sessions(expires_at);
`)
	if err != nil {
		return err
	}
	return c.migrateRBAC()
}

func (c *ControlStore) migrateRBAC() error {
	if _, err := c.pool.Exec(`ALTER TABLE families ADD COLUMN slug TEXT`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		return err
	}
	_, err := c.pool.Exec(`
CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  email TEXT COLLATE NOCASE,
  password_hash TEXT,
  display_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  UNIQUE(email)
);
CREATE TABLE IF NOT EXISTS memberships (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  kid_id INTEGER,
  adult_id INTEGER,
  can_manage_kid_logins INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active',
  created_by_account_id INTEGER,
  created_at TEXT NOT NULL,
  UNIQUE(account_id, family_id)
);
CREATE TABLE IF NOT EXISTS pin_credentials (
  account_id INTEGER PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  username TEXT NOT NULL COLLATE NOCASE,
  pin_hash TEXT NOT NULL,
  must_rotate INTEGER NOT NULL DEFAULT 0,
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  locked_until TEXT,
  UNIQUE(family_id, username)
);
CREATE INDEX IF NOT EXISTS memberships_family ON memberships(family_id);
CREATE INDEX IF NOT EXISTS pin_credentials_family ON pin_credentials(family_id);
`)
	if err != nil {
		return err
	}
	if err := c.ensureFamilySlugs(); err != nil {
		return err
	}
	if err := c.backfillAccountsFromUsers(); err != nil {
		return err
	}
	return c.migrateSessionsTable()
}

func (c *ControlStore) ensureFamilySlugs() error {
	rows, err := c.pool.Query(`SELECT id, name, COALESCE(slug, '') FROM families`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct{ id, name, slug string }
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name, &r.slug); err != nil {
			return err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range list {
		if r.slug != "" {
			continue
		}
		slug, err := c.uniqueFamilySlug(r.name)
		if err != nil {
			return err
		}
		if _, err := c.pool.Exec(`UPDATE families SET slug = ? WHERE id = ?`, slug, r.id); err != nil {
			return err
		}
	}
	_, _ = c.pool.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS families_slug ON families(slug)`)
	return nil
}

func (c *ControlStore) backfillAccountsFromUsers() error {
	var n int
	if err := c.pool.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	rows, err := c.pool.Query(`SELECT id, family_id, email, password_hash, created_at FROM users`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID int64
		var familyID, email, hash, created string
		if err := rows.Scan(&userID, &familyID, &email, &hash, &created); err != nil {
			return err
		}
		res, err := c.pool.Exec(
			`INSERT INTO accounts (kind, email, password_hash, display_name, created_at) VALUES (?, ?, ?, ?, ?)`,
			accountOwnerEmail, email, hash, email, created,
		)
		if err != nil {
			return err
		}
		accountID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := c.pool.Exec(
			`INSERT INTO memberships (account_id, family_id, role, status, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			accountID, familyID, roleOwner, membershipActive, created,
		); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (c *ControlStore) migrateSessionsTable() error {
	var hasAccountID int
	err := c.pool.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'account_id'`).Scan(&hasAccountID)
	if err != nil {
		return err
	}
	if hasAccountID > 0 {
		return nil
	}
	_, err = c.pool.Exec(`
CREATE TABLE sessions_v2 (
  id TEXT PRIMARY KEY,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  membership_id INTEGER NOT NULL REFERENCES memberships(id) ON DELETE CASCADE,
  auth_method TEXT NOT NULL DEFAULT 'password',
  expires_at TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	// Best-effort copy: map old user_id via email to new account/membership.
	_, _ = c.pool.Exec(`
INSERT INTO sessions_v2 (id, account_id, family_id, membership_id, auth_method, expires_at)
SELECT s.id, a.id, s.family_id, m.id, 'password', s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN accounts a ON lower(a.email) = lower(u.email)
JOIN memberships m ON m.account_id = a.id AND m.family_id = s.family_id
`)
	_, err = c.pool.Exec(`DROP TABLE sessions`)
	if err != nil {
		return err
	}
	_, err = c.pool.Exec(`ALTER TABLE sessions_v2 RENAME TO sessions`)
	if err != nil {
		return err
	}
	_, err = c.pool.Exec(`CREATE INDEX IF NOT EXISTS sessions_expires_at ON sessions(expires_at)`)
	return err
}

func newFamilyID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

var slugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

func slugifyFamilyName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugCleaner.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "family"
	}
	if len(s) > 32 {
		s = s[:32]
		s = strings.Trim(s, "-")
	}
	return s
}

func (c *ControlStore) uniqueFamilySlug(name string) (string, error) {
	base := slugifyFamilyName(name)
	for i := 0; i < 20; i++ {
		candidate := base
		if i > 0 {
			suf := make([]byte, 2)
			if _, err := rand.Read(suf); err != nil {
				return "", err
			}
			candidate = fmt.Sprintf("%s-%s", base, hex.EncodeToString(suf))
		}
		var n int
		if err := c.pool.QueryRow(`SELECT COUNT(*) FROM families WHERE slug = ?`, candidate).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate a family slug")
}

func (c *ControlStore) Signup(email, password, familyName, inviteGot, inviteExpected string) (*User, *Family, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	familyName = strings.TrimSpace(familyName)
	if email == "" || password == "" || familyName == "" {
		return nil, nil, fmt.Errorf("email, password, and family name are required")
	}
	if len(password) < 10 {
		return nil, nil, fmt.Errorf("password must be at least 10 characters")
	}
	if inviteExpected != "" {
		if inviteGot == "" {
			return nil, nil, errInviteRequired
		}
		if inviteGot != inviteExpected {
			return nil, nil, errBadInvite
		}
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, nil, err
	}
	familyID, err := newFamilyID()
	if err != nil {
		return nil, nil, err
	}
	slug, err := c.uniqueFamilySlug(familyName)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := c.pool.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO families (id, name, slug, created_at) VALUES (?, ?, ?, ?)`,
		familyID, familyName, slug, now,
	); err != nil {
		return nil, nil, err
	}
	res, err := tx.Exec(
		`INSERT INTO accounts (kind, email, password_hash, display_name, created_at) VALUES (?, ?, ?, ?, ?)`,
		accountOwnerEmail, email, hash, email, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, nil, errEmailTaken
		}
		return nil, nil, err
	}
	accountID, err := res.LastInsertId()
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(
		`INSERT INTO memberships (account_id, family_id, role, status, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, familyID, roleOwner, membershipActive, now,
	); err != nil {
		return nil, nil, err
	}
	// Keep legacy users row for any leftover tooling; best-effort.
	_, _ = tx.Exec(
		`INSERT INTO users (family_id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		familyID, email, hash, now,
	)
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return &User{ID: accountID, FamilyID: familyID, Email: email, PasswordHash: hash},
		&Family{ID: familyID, Name: familyName, Slug: slug}, nil
}

func (c *ControlStore) Authenticate(email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var accountID int64
	var hash, display, created string
	err := c.pool.QueryRow(
		`SELECT id, password_hash, display_name, created_at FROM accounts
		 WHERE kind = ? AND email = ?`,
		accountOwnerEmail, email,
	).Scan(&accountID, &hash, &display, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errBadCredentials
	}
	if err != nil {
		return nil, err
	}
	if !checkPassword(hash, password) {
		return nil, errBadCredentials
	}
	var familyID string
	err = c.pool.QueryRow(
		`SELECT family_id FROM memberships WHERE account_id = ? AND role = ? AND status = ?`,
		accountID, roleOwner, membershipActive,
	).Scan(&familyID)
	if err != nil {
		return nil, errBadCredentials
	}
	return &User{ID: accountID, FamilyID: familyID, Email: email, PasswordHash: hash}, nil
}

func (c *ControlStore) ownerMembership(accountID int64, familyID string) (int64, error) {
	var id int64
	err := c.pool.QueryRow(
		`SELECT id FROM memberships WHERE account_id = ? AND family_id = ? AND status = ?`,
		accountID, familyID, membershipActive,
	).Scan(&id)
	return id, err
}

func (c *ControlStore) CreateSession(accountID int64, familyID string, lifetime time.Duration) (*Session, error) {
	membershipID, err := c.ownerMembership(accountID, familyID)
	if err != nil {
		return nil, err
	}
	return c.createSession(accountID, membershipID, familyID, authPassword, lifetime)
}

func (c *ControlStore) createSession(accountID, membershipID int64, familyID, method string, lifetime time.Duration) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(lifetime)
	_, err = c.pool.Exec(
		`INSERT INTO sessions (id, account_id, family_id, membership_id, auth_method, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, accountID, familyID, membershipID, method, expires.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	return c.Session(id)
}

func (c *ControlStore) Session(id string) (*Session, error) {
	if id == "" {
		return nil, errNoSession
	}
	var s Session
	var expires string
	var canManage int
	err := c.pool.QueryRow(`
SELECT s.id, s.account_id, s.family_id, s.membership_id, s.auth_method, s.expires_at,
       m.role, COALESCE(m.kid_id, 0), m.can_manage_kid_logins,
       COALESCE(a.email, ''), a.display_name
FROM sessions s
JOIN memberships m ON m.id = s.membership_id
JOIN accounts a ON a.id = s.account_id
WHERE s.id = ? AND m.status = ?`, id, membershipActive,
	).Scan(&s.ID, &s.AccountID, &s.FamilyID, &s.MembershipID, &s.AuthMethod, &expires,
		&s.Role, &s.KidID, &canManage, &s.Email, &s.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoSession
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(t) {
		_ = c.RevokeSession(id)
		return nil, errNoSession
	}
	s.ExpiresAt = t
	s.CanManageKidLogins = canManage != 0
	s.UserID = s.AccountID
	return &s, nil
}

func (c *ControlStore) RevokeSession(id string) error {
	_, err := c.pool.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (c *ControlStore) KillSessionsForAccount(accountID int64) error {
	_, err := c.pool.Exec(`DELETE FROM sessions WHERE account_id = ?`, accountID)
	return err
}

func (c *ControlStore) UpdatePassword(accountID int64, password string) error {
	if len(password) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = c.pool.Exec(`UPDATE accounts SET password_hash = ? WHERE id = ? AND kind = ?`,
		hash, accountID, accountOwnerEmail)
	return err
}

func (c *ControlStore) User(id int64) (*User, error) {
	var u User
	var created string
	err := c.pool.QueryRow(
		`SELECT a.id, m.family_id, COALESCE(a.email, ''), COALESCE(a.password_hash, ''), a.created_at
		 FROM accounts a
		 JOIN memberships m ON m.account_id = a.id AND m.role = ? AND m.status = ?
		 WHERE a.id = ?`,
		roleOwner, membershipActive, id,
	).Scan(&u.ID, &u.FamilyID, &u.Email, &u.PasswordHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		// Fall back: any membership for this account.
		err = c.pool.QueryRow(
			`SELECT a.id, m.family_id, COALESCE(a.email, ''), COALESCE(a.password_hash, ''), a.created_at
			 FROM accounts a
			 JOIN memberships m ON m.account_id = a.id AND m.status = ?
			 WHERE a.id = ? LIMIT 1`,
			membershipActive, id,
		).Scan(&u.ID, &u.FamilyID, &u.Email, &u.PasswordHash, &created)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &u, err
}

func (c *ControlStore) Family(id string) (*Family, error) {
	var f Family
	var created string
	err := c.pool.QueryRow(
		`SELECT id, name, COALESCE(slug, ''), created_at FROM families WHERE id = ?`, id,
	).Scan(&f.ID, &f.Name, &f.Slug, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &f, err
}

func (c *ControlStore) FamilyBySlug(slug string) (*Family, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	var f Family
	var created string
	err := c.pool.QueryRow(
		`SELECT id, name, COALESCE(slug, ''), created_at FROM families WHERE slug = ?`, slug,
	).Scan(&f.ID, &f.Name, &f.Slug, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &f, err
}

func (c *ControlStore) FamilyCount() (int, error) {
	var n int
	err := c.pool.QueryRow(`SELECT COUNT(*) FROM families`).Scan(&n)
	return n, err
}

func familyDataDir(dataRoot, familyID string) string {
	return filepath.Join(dataRoot, familiesDirName, familyID)
}

func validatePIN(pin string) error {
	if len(pin) < minPINLen || len(pin) > maxPINLen {
		return fmt.Errorf("PIN must be %d–%d digits", minPINLen, maxPINLen)
	}
	for _, r := range pin {
		if !unicode.IsDigit(r) {
			return fmt.Errorf("PIN must be digits only")
		}
	}
	return nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func validUsername(username string) bool {
	if username == "" || len(username) > 32 {
		return false
	}
	for _, r := range username {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validRole(role string) bool {
	switch role {
	case roleCoParent, roleTeacher, roleCaregiver, roleKid:
		return true
	}
	return false
}
