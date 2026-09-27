package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	tokenConfirm = "confirm"
	tokenReset   = "reset"
	tokenSignin  = "signin"

	confirmTokenTTL = 48 * time.Hour
	resetTokenTTL   = time.Hour
	signinCodeTTL   = 10 * time.Minute

	resetGuessLimit = 5
	signinCodeLimit = 5
	resendMinGap    = time.Minute
)

var errEmailToken = errors.New("that link did not work")

func emailTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomSigninCode() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	n := int(buf[0])<<16 | int(buf[1])<<8 | int(buf[2])
	return fmt.Sprintf("%06d", n%1000000), nil
}

func (c *ControlStore) issueEmailToken(accountID int64, purpose, raw, challengeID string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, _ = c.pool.Exec(
		`UPDATE email_tokens SET used_at = ? WHERE account_id = ? AND purpose = ? AND used_at IS NULL`,
		now.Format(time.RFC3339), accountID, purpose,
	)
	_, err := c.pool.Exec(
		`INSERT INTO email_tokens
		 (account_id, purpose, token_hash, challenge_id, expires_at, failed_attempts, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		accountID, purpose, emailTokenHash(raw), nullIfEmpty(challengeID),
		now.Add(ttl).Format(time.RFC3339), now.Format(time.RFC3339),
	)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type emailTokenRow struct {
	id      int64
	account int64
	hash    string
	expires time.Time
	fails   int
}

func (c *ControlStore) findEmailToken(purpose, raw string) (*emailTokenRow, error) {
	var row emailTokenRow
	var expires string
	err := c.pool.QueryRow(
		`SELECT id, account_id, token_hash, expires_at, failed_attempts
		 FROM email_tokens
		 WHERE purpose = ? AND token_hash = ? AND used_at IS NULL`,
		purpose, emailTokenHash(raw),
	).Scan(&row.id, &row.account, &row.hash, &expires, &row.fails)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errEmailToken
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, expires)
	if err != nil || !time.Now().UTC().Before(t) {
		return nil, errEmailToken
	}
	return &row, nil
}

func (c *ControlStore) findSigninChallenge(challengeID string) (*emailTokenRow, error) {
	if challengeID == "" {
		return nil, errEmailToken
	}
	var row emailTokenRow
	var expires string
	err := c.pool.QueryRow(
		`SELECT id, account_id, token_hash, expires_at, failed_attempts
		 FROM email_tokens
		 WHERE purpose = ? AND challenge_id = ? AND used_at IS NULL`,
		tokenSignin, challengeID,
	).Scan(&row.id, &row.account, &row.hash, &expires, &row.fails)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errEmailToken
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, expires)
	if err != nil || !time.Now().UTC().Before(t) {
		return nil, errEmailToken
	}
	return &row, nil
}

func (c *ControlStore) consumeEmailToken(id int64) error {
	_, err := c.pool.Exec(
		`UPDATE email_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

func (c *ControlStore) markEmailVerified(accountID int64) error {
	_, err := c.pool.Exec(
		`UPDATE accounts SET email_verified_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), accountID,
	)
	return err
}

func (c *ControlStore) emailVerified(accountID int64) (bool, error) {
	var verified sql.NullString
	err := c.pool.QueryRow(
		`SELECT email_verified_at FROM accounts WHERE id = ?`, accountID,
	).Scan(&verified)
	if err != nil {
		return false, err
	}
	return verified.Valid && verified.String != "", nil
}

func (c *ControlStore) setEmail2FA(accountID int64, on bool) error {
	val := 0
	if on {
		val = 1
	}
	_, err := c.pool.Exec(`UPDATE accounts SET email_2fa = ? WHERE id = ? AND kind = ?`, val, accountID, accountOwnerEmail)
	return err
}

func (c *ControlStore) email2FAOn(accountID int64) (bool, error) {
	var n int
	err := c.pool.QueryRow(`SELECT email_2fa FROM accounts WHERE id = ?`, accountID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n != 0, nil
}

func (c *ControlStore) ownerFamily(accountID int64) (familyID, email string, err error) {
	err = c.pool.QueryRow(
		`SELECT m.family_id, COALESCE(a.email, '')
		 FROM accounts a
		 JOIN memberships m ON m.account_id = a.id AND m.role = ? AND m.status = ?
		 WHERE a.id = ?`,
		roleOwner, membershipActive, accountID,
	).Scan(&familyID, &email)
	return familyID, email, err
}

func (c *ControlStore) ownerByEmail(email string) (int64, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var id int64
	var familyID string
	err := c.pool.QueryRow(
		`SELECT a.id, m.family_id FROM accounts a
		 JOIN memberships m ON m.account_id = a.id AND m.role = ? AND m.status = ?
		 WHERE a.kind = ? AND a.email = ?`,
		roleOwner, membershipActive, accountOwnerEmail, email,
	).Scan(&id, &familyID)
	return id, familyID, err
}

// noteResetFailure counts a bad submission against a still-valid reset token.
// The fifth failure burns the token.
func (c *ControlStore) noteResetFailure(id int64, fails int) error {
	fails++
	if fails >= resetGuessLimit {
		return c.consumeEmailToken(id)
	}
	_, err := c.pool.Exec(`UPDATE email_tokens SET failed_attempts = ? WHERE id = ?`, fails, id)
	return err
}

func (c *ControlStore) noteSigninFailure(row *emailTokenRow) (burned bool, err error) {
	row.fails++
	if row.fails >= signinCodeLimit {
		if err := c.consumeEmailToken(row.id); err != nil {
			return true, err
		}
		return true, nil
	}
	_, err = c.pool.Exec(`UPDATE email_tokens SET failed_attempts = ? WHERE id = ?`, row.fails, row.id)
	return false, err
}

func signinCodeMatches(row *emailTokenRow, code string) bool {
	got := emailTokenHash(strings.TrimSpace(code))
	return subtle.ConstantTimeCompare([]byte(got), []byte(row.hash)) == 1
}

func (c *ControlStore) recentConfirmToken(accountID int64) (time.Time, bool) {
	var created string
	err := c.pool.QueryRow(
		`SELECT created_at FROM email_tokens
		 WHERE account_id = ? AND purpose = ?
		 ORDER BY id DESC LIMIT 1`,
		accountID, tokenConfirm,
	).Scan(&created)
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func linkURL(base, path, token string) string {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return path + "?token=" + token
	}
	return base + path + "?token=" + token
}
