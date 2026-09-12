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
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	controlDBName   = "control.db"
	hostedSIDCookie = "school_nanny_sid"
	familiesDirName = "families"
)

var (
	errEmailTaken     = errors.New("email already registered")
	errBadCredentials = errors.New("bad credentials")
	errInviteRequired = errors.New("invite code required")
	errBadInvite      = errors.New("invite code did not match")
	errNoSession      = errors.New("no session")
)

// ControlStore is the hosted multi-tenant control plane: users, families, sessions.
type ControlStore struct {
	pool *sql.DB
	path string
}

type Family struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type User struct {
	ID           int64
	FamilyID     string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    int64
	FamilyID  string
	ExpiresAt time.Time
}

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
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  family_id TEXT NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expires_at ON sessions(expires_at);
`)
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

// Signup creates a family + parent user. inviteRequired/inviteExpected gate new accounts.
func (c *ControlStore) Signup(email, password, familyName, inviteGot, inviteExpected string) (*User, *Family, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	familyName = strings.TrimSpace(familyName)
	if email == "" || password == "" || familyName == "" {
		return nil, nil, fmt.Errorf("email, password, and family name are required")
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
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := c.pool.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO families (id, name, created_at) VALUES (?, ?, ?)`,
		familyID, familyName, now,
	); err != nil {
		return nil, nil, err
	}
	res, err := tx.Exec(
		`INSERT INTO users (family_id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		familyID, email, hash, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, nil, errEmailTaken
		}
		return nil, nil, err
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return &User{ID: userID, FamilyID: familyID, Email: email, PasswordHash: hash},
		&Family{ID: familyID, Name: familyName}, nil
}

func (c *ControlStore) Authenticate(email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u User
	var created string
	err := c.pool.QueryRow(
		`SELECT id, family_id, email, password_hash, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&u.ID, &u.FamilyID, &u.Email, &u.PasswordHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errBadCredentials
	}
	if err != nil {
		return nil, err
	}
	if !checkPassword(u.PasswordHash, password) {
		return nil, errBadCredentials
	}
	return &u, nil
}

func (c *ControlStore) CreateSession(userID int64, familyID string, lifetime time.Duration) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(lifetime)
	_, err = c.pool.Exec(
		`INSERT INTO sessions (id, user_id, family_id, expires_at) VALUES (?, ?, ?, ?)`,
		id, userID, familyID, expires.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	return &Session{ID: id, UserID: userID, FamilyID: familyID, ExpiresAt: expires}, nil
}

func (c *ControlStore) Session(id string) (*Session, error) {
	if id == "" {
		return nil, errNoSession
	}
	var s Session
	var expires string
	err := c.pool.QueryRow(
		`SELECT id, user_id, family_id, expires_at FROM sessions WHERE id = ?`,
		id,
	).Scan(&s.ID, &s.UserID, &s.FamilyID, &expires)
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
	return &s, nil
}

func (c *ControlStore) RevokeSession(id string) error {
	_, err := c.pool.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (c *ControlStore) UpdatePassword(userID int64, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = c.pool.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, userID)
	return err
}

func (c *ControlStore) User(id int64) (*User, error) {
	var u User
	var created string
	err := c.pool.QueryRow(
		`SELECT id, family_id, email, password_hash, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.FamilyID, &u.Email, &u.PasswordHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return &u, err
}

func (c *ControlStore) FamilyCount() (int, error) {
	var n int
	err := c.pool.QueryRow(`SELECT COUNT(*) FROM families`).Scan(&n)
	return n, err
}

func familyDataDir(dataRoot, familyID string) string {
	return filepath.Join(dataRoot, familiesDirName, familyID)
}
