package main

import (
	"database/sql"
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

// EnsureDefaultAdult gives the family one grown-up to work with, so her pages
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
		"Mom", "Mom", "#8d78e0")
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
		e.starts_on, e.ends_on, e.title, e.body, e.created_at,
		COALESCE(l.name, ''), COALESCE(l.color, '')
	FROM adult_events e
	LEFT JOIN adult_event_labels l ON l.id = e.label_id`

func scanAdultEvents(rows *sql.Rows) ([]AdultEvent, error) {
	defer rows.Close()
	var events []AdultEvent
	for rows.Next() {
		var e AdultEvent
		if err := rows.Scan(&e.ID, &e.AdultID, &e.LabelID, &e.StartsOn, &e.EndsOn,
			&e.Title, &e.Body, &e.CreatedAt, &e.LabelName, &e.LabelColor); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// AdultEventsOverlapping returns every event touching the range, not only the
// ones starting inside it: a trip that began last month is still happening
// during the days this month shows.
func (s *Store) AdultEventsOverlapping(adultID int64, from, to string) ([]AdultEvent, error) {
	rows, err := s.db().Query(adultEventSelect+` WHERE e.adult_id = ?
		AND e.starts_on <= ? AND e.ends_on >= ?
		ORDER BY e.starts_on, e.ends_on, e.id`, adultID, to, from)
	if err != nil {
		return nil, err
	}
	return scanAdultEvents(rows)
}

func (s *Store) AdultEvent(id int64) (AdultEvent, error) {
	var e AdultEvent
	err := s.db().QueryRow(adultEventSelect+` WHERE e.id = ?`, id).
		Scan(&e.ID, &e.AdultID, &e.LabelID, &e.StartsOn, &e.EndsOn,
			&e.Title, &e.Body, &e.CreatedAt, &e.LabelName, &e.LabelColor)
	return e, err
}

func (s *Store) CreateAdultEvent(e AdultEvent) (int64, error) {
	res, err := s.db().Exec(`INSERT INTO adult_events
		(adult_id, label_id, starts_on, ends_on, title, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.AdultID, nullableID(e.LabelID), e.StartsOn, e.EndsOn, e.Title, e.Body,
		time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetAdultEventLabel pins a colour tag onto an event, or clears it when
// labelID is zero. The adult id keeps one profile from retagging another's.
func (s *Store) SetAdultEventLabel(adultID, eventID, labelID int64) error {
	_, err := s.db().Exec(`UPDATE adult_events SET label_id = ?
		WHERE id = ? AND adult_id = ?`, nullableID(labelID), eventID, adultID)
	return err
}

// DeleteAdultEvent names the adult as well as the event so one profile can
// never delete something off another's calendar.
func (s *Store) DeleteAdultEvent(adultID, id int64) error {
	_, err := s.db().Exec(`DELETE FROM adult_events WHERE id = ? AND adult_id = ?`, id, adultID)
	return err
}

// Event labels ------------------------------------------------------------

const adultLabelSelect = `SELECT id, adult_id, name, color, sort_order, created_at
	FROM adult_event_labels`

func scanAdultLabels(rows *sql.Rows) ([]AdultEventLabel, error) {
	defer rows.Close()
	var out []AdultEventLabel
	for rows.Next() {
		var l AdultEventLabel
		if err := rows.Scan(&l.ID, &l.AdultID, &l.Name, &l.Color, &l.SortOrder, &l.CreatedAt); err != nil {
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
		Scan(&l.ID, &l.AdultID, &l.Name, &l.Color, &l.SortOrder, &l.CreatedAt)
	return l, err
}

func (s *Store) CreateAdultEventLabel(l AdultEventLabel) (int64, error) {
	var next int
	if err := s.db().QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1
		FROM adult_event_labels WHERE adult_id = ?`, l.AdultID).Scan(&next); err != nil {
		return 0, err
	}
	res, err := s.db().Exec(`INSERT INTO adult_event_labels
		(adult_id, name, color, sort_order, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		l.AdultID, l.Name, l.Color, next, time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteAdultEventLabel removes a colour tag. Events that wore it keep their
// dates and wording; they simply become unlabeled.
func (s *Store) DeleteAdultEventLabel(adultID, id int64) error {
	_, err := s.db().Exec(`DELETE FROM adult_event_labels WHERE id = ? AND adult_id = ?`, id, adultID)
	return err
}

// MoveAdultCard swaps a card with its neighbour, which is all the ordering a
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
