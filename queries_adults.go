package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const adultSelect = `SELECT id, name, role, color, avatar_path, sort_order, archived FROM adults`

func scanAdults(rows *sql.Rows) ([]Adult, error) {
	defer rows.Close()
	var adults []Adult
	for rows.Next() {
		var a Adult
		if err := rows.Scan(&a.ID, &a.Name, &a.Role, &a.Color, &a.AvatarPath,
			&a.SortOrder, &a.Archived); err != nil {
			return nil, err
		}
		adults = append(adults, a)
	}
	return adults, rows.Err()
}

func (s *Store) Adults(includeArchived bool) ([]Adult, error) {
	q := adultSelect
	if !includeArchived {
		q += ` WHERE archived = 0`
	}
	q += ` ORDER BY sort_order, name`

	rows, err := s.db().Query(q)
	if err != nil {
		return nil, err
	}
	return scanAdults(rows)
}

func (s *Store) Adult(id int64) (Adult, error) {
	var a Adult
	err := s.db().QueryRow(adultSelect+` WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.Role, &a.Color, &a.AvatarPath, &a.SortOrder, &a.Archived)
	return a, err
}

func (s *Store) UpdateAdult(id int64, name, role, color string, archived bool) error {
	_, err := s.db().Exec(`UPDATE adults SET name = ?, role = ?, color = ?, archived = ? WHERE id = ?`,
		name, role, color, archived, id)
	return err
}

func (s *Store) SetAdultAvatar(id int64, storedPath string) (string, error) {
	var previous string
	if err := s.db().QueryRow(`SELECT avatar_path FROM adults WHERE id = ?`, id).Scan(&previous); err != nil {
		return "", err
	}
	if _, err := s.db().Exec(`UPDATE adults SET avatar_path = ? WHERE id = ?`, storedPath, id); err != nil {
		return "", err
	}
	return previous, nil
}

// EnsureDefaultAdult gives the family one grown-up to work with, so adult pages
// exist without anyone having to set them up first. The name is only a
// starting point; Settings can change it.
func (s *Store) EnsureDefaultAdult() error {
	var count int
	if err := s.db().QueryRow(`SELECT COUNT(*) FROM adults`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.db().Exec(`INSERT INTO adults (name, role, color, sort_order) VALUES (?, ?, ?, 1)`,
		"Parent", "Parent", "#8d78e0")
	return err
}

// Pinboard cards ---------------------------------------------------------

const cardSelect = `SELECT id, adult_id, title, body, pinned, sort_order, created_at, updated_at
	FROM adult_cards`

// AdultCards lists the pinboard, pinned cards first, then in the order the
// parent arranged them.
func (s *Store) AdultCards(adultID int64) ([]AdultCard, error) {
	rows, err := s.db().Query(cardSelect+` WHERE adult_id = ?
		ORDER BY pinned DESC, sort_order, id`, adultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []AdultCard
	for rows.Next() {
		var c AdultCard
		if err := rows.Scan(&c.ID, &c.AdultID, &c.Title, &c.Body, &c.Pinned,
			&c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func (s *Store) AdultCard(id int64) (AdultCard, error) {
	var c AdultCard
	err := s.db().QueryRow(cardSelect+` WHERE id = ?`, id).
		Scan(&c.ID, &c.AdultID, &c.Title, &c.Body, &c.Pinned, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) CreateAdultCard(c AdultCard) (int64, error) {
	var next int
	if err := s.db().QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM adult_cards WHERE adult_id = ?`,
		c.AdultID).Scan(&next); err != nil {
		return 0, err
	}
	now := time.Now().Format(time.RFC3339)
	res, err := s.db().Exec(`INSERT INTO adult_cards
		(adult_id, title, body, pinned, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.AdultID, c.Title, c.Body, c.Pinned, next, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAdultCard(id int64, title, body string, pinned bool) error {
	_, err := s.db().Exec(`UPDATE adult_cards SET title = ?, body = ?, pinned = ?, updated_at = ?
		WHERE id = ?`, title, body, pinned, time.Now().Format(time.RFC3339), id)
	return err
}

func (s *Store) DeleteAdultCard(id int64) error {
	_, err := s.db().Exec(`DELETE FROM adult_cards WHERE id = ?`, id)
	return err
}

// Calendar events --------------------------------------------------------

const adultEventSelect = `SELECT e.id, e.adult_id, COALESCE(e.label_id, 0),
		e.uid, e.starts_on, e.ends_on, e.all_day, e.start_at, e.end_at,
		e.title, e.body, e.location, e.rrule, e.exdates, e.sequence,
		e.created_at, e.modified_at,
		COALESCE(l.name, ''), COALESCE(l.color, ''), COALESCE(l.emoji, '')
	FROM adult_events e
	LEFT JOIN adult_event_labels l ON l.id = e.label_id`

func scanAdultEvent(scanner interface {
	Scan(dest ...any) error
}) (AdultEvent, error) {
	var e AdultEvent
	var allDay int
	err := scanner.Scan(&e.ID, &e.AdultID, &e.LabelID, &e.UID, &e.StartsOn, &e.EndsOn,
		&allDay, &e.StartAt, &e.EndAt, &e.Title, &e.Body, &e.Location, &e.RRule, &e.ExDates,
		&e.Sequence, &e.CreatedAt, &e.ModifiedAt, &e.LabelName, &e.LabelColor, &e.LabelEmoji)
	e.AllDay = allDay != 0
	return e, err
}

func scanAdultEvents(rows *sql.Rows) ([]AdultEvent, error) {
	defer rows.Close()
	var events []AdultEvent
	for rows.Next() {
		e, err := scanAdultEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func ensureEventUID(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid != "" {
		return uid
	}
	return newEventUID()
}

func newEventUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("adult-event-%d@school-nanny", time.Now().UnixNano())
	}
	return hex.EncodeToString(b) + "@school-nanny"
}

func allDayInt(allDay bool) int {
	if allDay {
		return 1
	}
	return 0
}

func bumpAdultCalendarSync(tx DBTX, adultID int64) error {
	_, err := tx.Exec(`INSERT INTO adult_calendar_meta (adult_id, sync_token) VALUES (?, 1)
		ON CONFLICT(adult_id) DO UPDATE SET sync_token = sync_token + 1`, adultID)
	return err
}

func (s *Store) AdultCalendarSyncToken(adultID int64) (int64, error) {
	var token int64
	err := s.db().QueryRow(`SELECT sync_token FROM adult_calendar_meta WHERE adult_id = ?`, adultID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db().Exec(`INSERT INTO adult_calendar_meta (adult_id, sync_token) VALUES (?, 1)`, adultID)
		if err != nil {
			return 0, err
		}
		return 1, nil
	}
	return token, err
}

// AdultEventsOverlapping returns every event touching the range, expanding
// yearly rules into the days that fall inside it. Masters without a yearly
// rule are returned as stored.
func (s *Store) AdultEventsOverlapping(adultID int64, from, to string) ([]AdultEvent, error) {
	rows, err := s.db().Query(adultEventSelect+` WHERE e.adult_id = ?
		AND (
			(e.rrule = '' AND e.starts_on <= ? AND e.ends_on >= ?)
			OR e.rrule != ''
		)
		ORDER BY e.starts_on, e.ends_on, e.id`, adultID, to, from)
	if err != nil {
		return nil, err
	}
	masters, err := scanAdultEvents(rows)
	if err != nil {
		return nil, err
	}
	return ExpandAdultEvents(masters, from, to), nil
}

// AdultEventMasters returns the stored rows for an adult without expanding
// recurrence. CalDAV serves masters; the phone expands them itself.
func (s *Store) AdultEventMasters(adultID int64) ([]AdultEvent, error) {
	rows, err := s.db().Query(adultEventSelect+` WHERE e.adult_id = ?
		ORDER BY e.starts_on, e.ends_on, e.id`, adultID)
	if err != nil {
		return nil, err
	}
	return scanAdultEvents(rows)
}

func (s *Store) AdultEventByUID(adultID int64, uid string) (AdultEvent, error) {
	return scanAdultEvent(s.db().QueryRow(adultEventSelect+` WHERE e.adult_id = ? AND e.uid = ?`, adultID, uid))
}

func (s *Store) AdultEvent(id int64) (AdultEvent, error) {
	return scanAdultEvent(s.db().QueryRow(adultEventSelect+` WHERE e.id = ?`, id))
}

func (s *Store) AdultEventTombstones(adultID int64, since string) ([]string, error) {
	rows, err := s.db().Query(`SELECT uid FROM adult_event_tombstones
		WHERE adult_id = ? AND deleted_at > ?
		ORDER BY deleted_at, uid`, adultID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

func (s *Store) CreateAdultEvent(e AdultEvent) (int64, error) {
	tx, err := s.db().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	if e.CreatedAt == "" {
		e.CreatedAt = now
	}
	if e.ModifiedAt == "" {
		e.ModifiedAt = now
	}
	if !e.AllDay && e.StartAt == "" {
		e.AllDay = true
	}
	uid := ensureEventUID(e.UID)
	res, err := tx.Exec(`INSERT INTO adult_events
		(adult_id, label_id, uid, starts_on, ends_on, all_day, start_at, end_at,
		 title, body, location, rrule, exdates, sequence, created_at, modified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.AdultID, nullableID(e.LabelID), uid, e.StartsOn, e.EndsOn, allDayInt(e.AllDay),
		e.StartAt, e.EndAt, e.Title, e.Body, e.Location, e.RRule, e.ExDates, e.Sequence,
		e.CreatedAt, e.ModifiedAt)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := bumpAdultCalendarSync(tx, e.AdultID); err != nil {
		return 0, err
	}
	_, _ = tx.Exec(`DELETE FROM adult_event_tombstones WHERE adult_id = ? AND uid = ?`, e.AdultID, uid)
	return id, tx.Commit()
}

func (s *Store) CreateAdultEvents(events []AdultEvent) error {
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	adults := map[int64]bool{}
	for _, e := range events {
		uid := ensureEventUID(e.UID)
		allDay := e.AllDay
		if !allDay && e.StartAt == "" {
			allDay = true
		}
		if _, err := tx.Exec(`INSERT INTO adult_events
			(adult_id, label_id, uid, starts_on, ends_on, all_day, start_at, end_at,
			 title, body, location, rrule, exdates, sequence, created_at, modified_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.AdultID, nullableID(e.LabelID), uid, e.StartsOn, e.EndsOn, allDayInt(allDay),
			e.StartAt, e.EndAt, e.Title, e.Body, e.Location, e.RRule, e.ExDates, e.Sequence,
			now, now); err != nil {
			return err
		}
		adults[e.AdultID] = true
		_, _ = tx.Exec(`DELETE FROM adult_event_tombstones WHERE adult_id = ? AND uid = ?`, e.AdultID, uid)
	}
	for adultID := range adults {
		if err := bumpAdultCalendarSync(tx, adultID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateAdultEvent rewrites an existing event in place: title, notes, dates,
// and which label it wears. That is how a note written last month still gets a
// birthday cake when she invents the label later.
func (s *Store) UpdateAdultEvent(adultID, id int64, e AdultEvent) error {
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(`UPDATE adult_events
		SET label_id = ?, starts_on = ?, ends_on = ?, all_day = ?, start_at = ?, end_at = ?,
		    title = ?, body = ?, location = ?, rrule = ?, exdates = ?,
		    sequence = sequence + 1, modified_at = ?
		WHERE id = ? AND adult_id = ?`,
		nullableID(e.LabelID), e.StartsOn, e.EndsOn, allDayInt(e.AllDay || e.StartAt == ""),
		e.StartAt, e.EndAt, e.Title, e.Body, e.Location, e.RRule, e.ExDates, now, id, adultID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	if err := bumpAdultCalendarSync(tx, adultID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertAdultEventByUID creates or replaces the event the phone addressed.
// IfMatch is the ETag the client sent; empty means create-only or overwrite.
func (s *Store) UpsertAdultEventByUID(e AdultEvent, ifMatch string) (AdultEvent, bool, error) {
	tx, err := s.db().Begin()
	if err != nil {
		return AdultEvent{}, false, err
	}
	defer tx.Rollback()

	existing, err := scanAdultEvent(tx.QueryRow(adultEventSelect+` WHERE e.adult_id = ? AND e.uid = ?`, e.AdultID, e.UID))
	now := time.Now().UTC().Format(time.RFC3339)
	created := false
	if errors.Is(err, sql.ErrNoRows) {
		if ifMatch != "" && ifMatch != "*" {
			return AdultEvent{}, false, errCalDAVPrecondition
		}
		e.CreatedAt = now
		e.ModifiedAt = now
		e.Sequence = 0
		if !e.AllDay && e.StartAt == "" {
			e.AllDay = true
		}
		res, err := tx.Exec(`INSERT INTO adult_events
			(adult_id, label_id, uid, starts_on, ends_on, all_day, start_at, end_at,
			 title, body, location, rrule, exdates, sequence, created_at, modified_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.AdultID, nullableID(e.LabelID), e.UID, e.StartsOn, e.EndsOn, allDayInt(e.AllDay),
			e.StartAt, e.EndAt, e.Title, e.Body, e.Location, e.RRule, e.ExDates, e.Sequence,
			e.CreatedAt, e.ModifiedAt)
		if err != nil {
			return AdultEvent{}, false, err
		}
		e.ID, err = res.LastInsertId()
		if err != nil {
			return AdultEvent{}, false, err
		}
		created = true
		_, _ = tx.Exec(`DELETE FROM adult_event_tombstones WHERE adult_id = ? AND uid = ?`, e.AdultID, e.UID)
	} else if err != nil {
		return AdultEvent{}, false, err
	} else {
		if ifMatch != "" && ifMatch != existing.ETag() {
			return AdultEvent{}, false, errCalDAVPrecondition
		}
		e.ID = existing.ID
		e.LabelID = existing.LabelID
		e.CreatedAt = existing.CreatedAt
		e.Sequence = existing.Sequence + 1
		e.ModifiedAt = now
		if !e.AllDay && e.StartAt == "" {
			e.AllDay = true
		}
		_, err = tx.Exec(`UPDATE adult_events
			SET starts_on = ?, ends_on = ?, all_day = ?, start_at = ?, end_at = ?,
			    title = ?, body = ?, location = ?, rrule = ?, exdates = ?,
			    sequence = ?, modified_at = ?
			WHERE id = ? AND adult_id = ?`,
			e.StartsOn, e.EndsOn, allDayInt(e.AllDay), e.StartAt, e.EndAt,
			e.Title, e.Body, e.Location, e.RRule, e.ExDates, e.Sequence, e.ModifiedAt,
			e.ID, e.AdultID)
		if err != nil {
			return AdultEvent{}, false, err
		}
	}
	if err := bumpAdultCalendarSync(tx, e.AdultID); err != nil {
		return AdultEvent{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return AdultEvent{}, false, err
	}
	return e, created, nil
}

var errCalDAVPrecondition = errors.New("caldav precondition failed")

// SetAdultEventLabel pins a color tag onto an event, or clears it when
// labelID is zero. The adult id keeps one profile from retagging another's.
func (s *Store) SetAdultEventLabel(adultID, eventID, labelID int64) error {
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(`UPDATE adult_events SET label_id = ?, sequence = sequence + 1, modified_at = ?
		WHERE id = ? AND adult_id = ?`, nullableID(labelID), now, eventID, adultID)
	if err != nil {
		return err
	}
	if err := bumpAdultCalendarSync(tx, adultID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteAdultEvent names the adult as well as the event so one profile can
// never delete something off another's calendar.
func (s *Store) DeleteAdultEvent(adultID, id int64) error {
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var uid string
	err = tx.QueryRow(`SELECT uid FROM adult_events WHERE id = ? AND adult_id = ?`, id, adultID).Scan(&uid)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM adult_events WHERE id = ? AND adult_id = ?`, id, adultID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT INTO adult_event_tombstones (adult_id, uid, deleted_at)
		VALUES (?, ?, ?)
		ON CONFLICT(adult_id, uid) DO UPDATE SET deleted_at = excluded.deleted_at`,
		adultID, uid, now); err != nil {
		return err
	}
	if err := bumpAdultCalendarSync(tx, adultID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteAdultEventByUID(adultID int64, uid, ifMatch string) error {
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing, err := scanAdultEvent(tx.QueryRow(adultEventSelect+` WHERE e.adult_id = ? AND e.uid = ?`, adultID, uid))
	if err != nil {
		return err
	}
	if ifMatch != "" && ifMatch != existing.ETag() {
		return errCalDAVPrecondition
	}
	if _, err := tx.Exec(`DELETE FROM adult_events WHERE id = ? AND adult_id = ?`, existing.ID, adultID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT INTO adult_event_tombstones (adult_id, uid, deleted_at)
		VALUES (?, ?, ?)
		ON CONFLICT(adult_id, uid) DO UPDATE SET deleted_at = excluded.deleted_at`,
		adultID, uid, now); err != nil {
		return err
	}
	if err := bumpAdultCalendarSync(tx, adultID); err != nil {
		return err
	}
	return tx.Commit()
}

// Event labels ------------------------------------------------------------

const adultLabelSelect = `SELECT id, adult_id, name, color, emoji, sort_order, created_at
	FROM adult_event_labels`

func scanAdultLabels(rows *sql.Rows) ([]AdultEventLabel, error) {
	defer rows.Close()
	var out []AdultEventLabel
	for rows.Next() {
		var l AdultEventLabel
		if err := rows.Scan(&l.ID, &l.AdultID, &l.Name, &l.Color, &l.Emoji, &l.SortOrder, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) AdultEventLabels(adultID int64) ([]AdultEventLabel, error) {
	rows, err := s.db().Query(adultLabelSelect+`
		WHERE adult_id = ? ORDER BY sort_order, id`, adultID)
	if err != nil {
		return nil, err
	}
	return scanAdultLabels(rows)
}

func (s *Store) AdultEventLabel(id int64) (AdultEventLabel, error) {
	var l AdultEventLabel
	err := s.db().QueryRow(adultLabelSelect+` WHERE id = ?`, id).
		Scan(&l.ID, &l.AdultID, &l.Name, &l.Color, &l.Emoji, &l.SortOrder, &l.CreatedAt)
	return l, err
}

func (s *Store) CreateAdultEventLabel(l AdultEventLabel) (int64, error) {
	var next int
	if err := s.db().QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1
		FROM adult_event_labels WHERE adult_id = ?`, l.AdultID).Scan(&next); err != nil {
		return 0, err
	}
	res, err := s.db().Exec(`INSERT INTO adult_event_labels
		(adult_id, name, color, emoji, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		l.AdultID, l.Name, l.Color, strings.TrimSpace(l.Emoji), next, time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAdultEventLabel(adultID, id int64, name, color, emoji string) error {
	_, err := s.db().Exec(`UPDATE adult_event_labels
		SET name = ?, color = ?, emoji = ?
		WHERE id = ? AND adult_id = ?`,
		name, color, strings.TrimSpace(emoji), id, adultID)
	return err
}

// DeleteAdultEventLabel removes a color tag. Events that wore it keep their
// dates and wording; they simply become unlabeled.
func (s *Store) DeleteAdultEventLabel(adultID, id int64) error {
	_, err := s.db().Exec(`DELETE FROM adult_event_labels WHERE id = ? AND adult_id = ?`, id, adultID)
	return err
}

// Holiday notes -----------------------------------------------------------

const holidayNoteSelect = `SELECT n.id, n.adult_id, n.observed_on, n.holiday_name,
		n.emoji, n.notes, COALESCE(n.label_id, 0),
		COALESCE(l.name, ''), COALESCE(l.color, ''), COALESCE(l.emoji, '')
	FROM adult_holiday_notes n
	LEFT JOIN adult_event_labels l ON l.id = n.label_id`

type holidayNoteRow struct {
	AdultHolidayNote
	LabelName, LabelColor, LabelEmoji string
}

func (s *Store) HolidayNotesOverlapping(adultID int64, from, to string) ([]holidayNoteRow, error) {
	rows, err := s.db().Query(holidayNoteSelect+`
		WHERE n.adult_id = ? AND n.observed_on BETWEEN ? AND ?
		ORDER BY n.observed_on, n.holiday_name`, adultID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []holidayNoteRow
	for rows.Next() {
		var r holidayNoteRow
		if err := rows.Scan(&r.ID, &r.AdultID, &r.ObservedOn, &r.HolidayName,
			&r.Emoji, &r.Notes, &r.LabelID, &r.LabelName, &r.LabelColor, &r.LabelEmoji); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertHolidayNote writes her personalization of a computed holiday. Empty
// emoji/notes with no label clears the override entirely so the default icon
// comes back.
func (s *Store) UpsertHolidayNote(n AdultHolidayNote) error {
	emoji := strings.TrimSpace(n.Emoji)
	notes := strings.TrimSpace(n.Notes)
	if emoji == "" && notes == "" && n.LabelID == 0 {
		_, err := s.db().Exec(`DELETE FROM adult_holiday_notes
			WHERE adult_id = ? AND observed_on = ? AND holiday_name = ?`,
			n.AdultID, n.ObservedOn, n.HolidayName)
		return err
	}
	_, err := s.db().Exec(`INSERT INTO adult_holiday_notes
		(adult_id, observed_on, holiday_name, emoji, notes, label_id)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(adult_id, observed_on, holiday_name) DO UPDATE SET
			emoji = excluded.emoji,
			notes = excluded.notes,
			label_id = excluded.label_id`,
		n.AdultID, n.ObservedOn, n.HolidayName, emoji, notes, nullableID(n.LabelID))
	return err
}

// MoveAdultCard swaps a card with its neighbor, which is all the ordering a
// short pinboard needs.
func (s *Store) MoveAdultCard(id int64, up bool) error {
	card, err := s.AdultCard(id)
	if err != nil {
		return err
	}
	cards, err := s.AdultCards(card.AdultID)
	if err != nil {
		return err
	}
	// Reordering runs over the list as it is displayed, so a pinned card only
	// ever trades places with another pinned one.
	at := -1
	for i, c := range cards {
		if c.ID == id {
			at = i
		}
	}
	swap := at + 1
	if up {
		swap = at - 1
	}
	if at < 0 || swap < 0 || swap >= len(cards) || cards[swap].Pinned != card.Pinned {
		return nil
	}

	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE adult_cards SET sort_order = ? WHERE id = ?`,
		cards[swap].SortOrder, card.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE adult_cards SET sort_order = ? WHERE id = ?`,
		card.SortOrder, cards[swap].ID); err != nil {
		return err
	}
	return tx.Commit()
}
