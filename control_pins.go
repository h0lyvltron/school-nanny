package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// IssuePIN creates a PIN principal + membership. Returns the plaintext PIN for
// one-time display (caller must not log it).
func (c *ControlStore) IssuePIN(familyID string, issuedBy int64, username, pin, role, displayName string, kidID, adultID int64, canManageKids bool) (*Membership, error) {
	username = normalizeUsername(username)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}
	if !validUsername(username) {
		return nil, fmt.Errorf("username must be letters, numbers, - or _")
	}
	if !validRole(role) {
		return nil, fmt.Errorf("unknown role")
	}
	if err := validatePIN(pin); err != nil {
		return nil, err
	}
	if role == roleKid && kidID == 0 {
		return nil, fmt.Errorf("a kid login needs a child")
	}
	if role != roleKid {
		kidID = 0
	}
	if role != roleCoParent {
		canManageKids = false
	}

	hash, err := hashPassword(pin)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := c.pool.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO accounts (kind, email, password_hash, display_name, created_at)
		 VALUES (?, NULL, NULL, ?, ?)`,
		accountPINPrincipal, displayName, now,
	)
	if err != nil {
		return nil, err
	}
	accountID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`INSERT INTO pin_credentials (account_id, family_id, username, pin_hash)
		 VALUES (?, ?, ?, ?)`,
		accountID, familyID, username, hash,
	); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errUsernameTaken
		}
		return nil, err
	}
	var kid any
	if kidID > 0 {
		kid = kidID
	}
	var adult any
	if adultID > 0 {
		adult = adultID
	}
	manage := 0
	if canManageKids {
		manage = 1
	}
	res, err = tx.Exec(
		`INSERT INTO memberships
		 (account_id, family_id, role, kid_id, adult_id, can_manage_kid_logins, status, created_by_account_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, familyID, role, kid, adult, manage, membershipActive, issuedBy, now,
	)
	if err != nil {
		return nil, err
	}
	membershipID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Membership{
		ID:                 membershipID,
		AccountID:          accountID,
		FamilyID:           familyID,
		Role:               role,
		KidID:              kidID,
		AdultID:            adultID,
		CanManageKidLogins: canManageKids,
		Status:             membershipActive,
		Username:           username,
		DisplayName:        displayName,
	}, nil
}

func (c *ControlStore) ResetPIN(membershipID int64, familyID, pin string) error {
	if err := validatePIN(pin); err != nil {
		return err
	}
	hash, err := hashPassword(pin)
	if err != nil {
		return err
	}
	var accountID int64
	err = c.pool.QueryRow(
		`SELECT account_id FROM memberships WHERE id = ? AND family_id = ? AND status = ?`,
		membershipID, familyID, membershipActive,
	).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("that login was not found")
	}
	if err != nil {
		return err
	}
	_, err = c.pool.Exec(
		`UPDATE pin_credentials SET pin_hash = ?, failed_attempts = 0, locked_until = NULL WHERE account_id = ?`,
		hash, accountID,
	)
	if err != nil {
		return err
	}
	return c.KillSessionsForAccount(accountID)
}

func (c *ControlStore) RevokeMembership(membershipID int64, familyID string) error {
	var accountID int64
	var role string
	err := c.pool.QueryRow(
		`SELECT account_id, role FROM memberships WHERE id = ? AND family_id = ?`,
		membershipID, familyID,
	).Scan(&accountID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("that login was not found")
	}
	if err != nil {
		return err
	}
	if role == roleOwner {
		return fmt.Errorf("cannot revoke the owner")
	}
	_, err = c.pool.Exec(
		`UPDATE memberships SET status = ? WHERE id = ?`, membershipRevoked, membershipID,
	)
	if err != nil {
		return err
	}
	return c.KillSessionsForAccount(accountID)
}

func (c *ControlStore) AuthenticatePIN(familySlug, username, pin string) (*Session, error) {
	family, err := c.FamilyBySlug(familySlug)
	if err != nil {
		return nil, errBadCredentials
	}
	username = normalizeUsername(username)
	var accountID, membershipID int64
	var pinHash string
	var fails int
	var locked sql.NullString
	var role string
	var kidID int64
	var canManage int
	var display string
	err = c.pool.QueryRow(`
SELECT a.id, m.id, p.pin_hash, p.failed_attempts, p.locked_until,
       m.role, COALESCE(m.kid_id, 0), m.can_manage_kid_logins, a.display_name
FROM pin_credentials p
JOIN accounts a ON a.id = p.account_id
JOIN memberships m ON m.account_id = a.id AND m.family_id = p.family_id
WHERE p.family_id = ? AND p.username = ? AND m.status = ?`,
		family.ID, username, membershipActive,
	).Scan(&accountID, &membershipID, &pinHash, &fails, &locked, &role, &kidID, &canManage, &display)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errBadCredentials
	}
	if err != nil {
		return nil, err
	}
	if locked.Valid && locked.String != "" {
		if t, err := time.Parse(time.RFC3339, locked.String); err == nil && time.Now().UTC().Before(t) {
			return nil, errPINLocked
		}
	}
	if !checkPassword(pinHash, pin) {
		fails++
		var lockedUntil any
		if fails >= maxPINFails {
			lockedUntil = time.Now().UTC().Add(pinLockMinutes * time.Minute).Format(time.RFC3339)
		}
		_, _ = c.pool.Exec(
			`UPDATE pin_credentials SET failed_attempts = ?, locked_until = ? WHERE account_id = ?`,
			fails, lockedUntil, accountID,
		)
		if fails >= maxPINFails {
			return nil, errPINLocked
		}
		return nil, errBadPIN
	}
	_, _ = c.pool.Exec(
		`UPDATE pin_credentials SET failed_attempts = 0, locked_until = NULL WHERE account_id = ?`,
		accountID,
	)
	return c.createSession(accountID, membershipID, family.ID, authPIN, sessionLifetime)
}

func (c *ControlStore) ListMemberships(familyID string) ([]Membership, error) {
	rows, err := c.pool.Query(`
SELECT m.id, m.account_id, m.family_id, m.role, COALESCE(m.kid_id, 0), COALESCE(m.adult_id, 0),
       m.can_manage_kid_logins, m.status, a.display_name, COALESCE(p.username, ''), m.created_at
FROM memberships m
JOIN accounts a ON a.id = m.account_id
LEFT JOIN pin_credentials p ON p.account_id = a.id
WHERE m.family_id = ?
ORDER BY CASE m.role
  WHEN 'owner' THEN 0 WHEN 'co_parent' THEN 1 WHEN 'teacher' THEN 2
  WHEN 'caregiver' THEN 3 WHEN 'kid' THEN 4 ELSE 5 END, a.display_name`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Membership
	for rows.Next() {
		var m Membership
		var created string
		var manage int
		if err := rows.Scan(&m.ID, &m.AccountID, &m.FamilyID, &m.Role, &m.KidID, &m.AdultID,
			&manage, &m.Status, &m.DisplayName, &m.Username, &created); err != nil {
			return nil, err
		}
		m.CanManageKidLogins = manage != 0
		m.CreatedAt = time.Time{}
		out = append(out, m)
	}
	return out, rows.Err()
}
