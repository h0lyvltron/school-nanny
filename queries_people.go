package main

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"
)

const kidSelect = `SELECT id, name, grade, color, avatar_path, sort_order, archived FROM kids`

func (s *Store) Kids(includeArchived bool) ([]Kid, error) {
	q := kidSelect
	if !includeArchived {
		q += ` WHERE archived = 0`
	}
	q += ` ORDER BY sort_order, name`

	rows, err := s.db().Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var kids []Kid
	for rows.Next() {
		var k Kid
		if err := rows.Scan(&k.ID, &k.Name, &k.Grade, &k.Color, &k.AvatarPath, &k.SortOrder, &k.Archived); err != nil {
			return nil, err
		}
		kids = append(kids, k)
	}
	return kids, rows.Err()
}

func (s *Store) Kid(id int64) (Kid, error) {
	var k Kid
	err := s.db().QueryRow(kidSelect+` WHERE id = ?`, id).
		Scan(&k.ID, &k.Name, &k.Grade, &k.Color, &k.AvatarPath, &k.SortOrder, &k.Archived)
	return k, err
}

// SetKidAvatar records (or clears) a child's photo and returns the path of the
// photo it replaced, so the caller can delete the file it no longer needs.
func (s *Store) SetKidAvatar(id int64, storedPath string) (string, error) {
	var previous string
	err := s.db().QueryRow(`SELECT avatar_path FROM kids WHERE id = ?`, id).Scan(&previous)
	if err != nil {
		return "", err
	}
	if _, err := s.db().Exec(`UPDATE kids SET avatar_path = ? WHERE id = ?`, storedPath, id); err != nil {
		return "", err
	}
	return previous, nil
}

func (s *Store) CreateKid(name, grade, color string) (int64, error) {
	var next int
	if err := s.db().QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM kids`).Scan(&next); err != nil {
		return 0, err
	}
	res, err := s.db().Exec(`INSERT INTO kids (name, grade, color, sort_order) VALUES (?, ?, ?, ?)`,
		name, grade, color, next)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateKid(id int64, name, grade, color string, archived bool) error {
	_, err := s.db().Exec(`UPDATE kids SET name = ?, grade = ?, color = ?, archived = ? WHERE id = ?`,
		name, grade, color, archived, id)
	return err
}

// DeleteKid removes a child and, through ON DELETE CASCADE, their lessons,
// tests, notes, and attachment records.
func (s *Store) DeleteKid(id int64) error {
	_, err := s.db().Exec(`DELETE FROM kids WHERE id = ?`, id)
	return err
}

const subjectSelect = `SELECT id, name, slug, color, sort_order, archived FROM subjects`

func (s *Store) Subjects(includeArchived bool) ([]Subject, error) {
	q := subjectSelect
	if !includeArchived {
		q += ` WHERE archived = 0`
	}
	q += ` ORDER BY sort_order, name`

	rows, err := s.db().Query(q)
	if err != nil {
		return nil, err
	}
	return scanSubjects(rows)
}

func (s *Store) Subject(id int64) (Subject, error) {
	var sub Subject
	err := s.db().QueryRow(subjectSelect+` WHERE id = ?`, id).
		Scan(&sub.ID, &sub.Name, &sub.Slug, &sub.Color, &sub.SortOrder, &sub.Archived)
	return sub, err
}

func (s *Store) CreateSubject(name, color string) (int64, error) {
	var next int
	if err := s.db().QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM subjects`).Scan(&next); err != nil {
		return 0, err
	}
	slug := uniqueSlug(s, slugify(name))
	res, err := s.db().Exec(`INSERT INTO subjects (name, slug, color, sort_order) VALUES (?, ?, ?, ?)`,
		name, slug, color, next)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateSubject(id int64, name, color string, archived bool) error {
	_, err := s.db().Exec(`UPDATE subjects SET name = ?, color = ?, archived = ? WHERE id = ?`,
		name, color, archived, id)
	return err
}

// BackfillSubjectColors gives a colour to every subject that does not have one
// yet: the ones that existed before subjects were coloured, and any that were
// somehow saved blank. Walking the whole list rather than only the blank ones
// keeps the colours spread across the palette instead of piling onto its first
// entries.
func (s *Store) BackfillSubjectColors() error {
	rows, err := s.db().Query(subjectSelect + ` ORDER BY sort_order, id`)
	if err != nil {
		return err
	}
	subjects, err := scanSubjects(rows)
	if err != nil {
		return err
	}

	for i, sub := range subjects {
		if sub.Color != "" {
			continue
		}
		color := subjectPalette[i%len(subjectPalette)]
		if _, err := s.db().Exec(`UPDATE subjects SET color = ? WHERE id = ?`, color, sub.ID); err != nil {
			return err
		}
	}
	return nil
}

func scanSubjects(rows *sql.Rows) ([]Subject, error) {
	defer rows.Close()
	var subjects []Subject
	for rows.Next() {
		var sub Subject
		if err := rows.Scan(&sub.ID, &sub.Name, &sub.Slug, &sub.Color, &sub.SortOrder, &sub.Archived); err != nil {
			return nil, err
		}
		subjects = append(subjects, sub)
	}
	return subjects, rows.Err()
}

func (s *Store) DeleteSubject(id int64) error {
	_, err := s.db().Exec(`DELETE FROM subjects WHERE id = ?`, id)
	return err
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	slug := nonSlug.ReplaceAllString(strings.ToLower(name), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "subject"
	}
	return slug
}

func uniqueSlug(s *Store, base string) string {
	slug := base
	for i := 2; ; i++ {
		var exists int
		err := s.db().QueryRow(`SELECT COUNT(*) FROM subjects WHERE slug = ?`, slug).Scan(&exists)
		if err != nil || exists == 0 {
			return slug
		}
		slug = base + "-" + strconv.Itoa(i)
	}
}

func (s *Store) SchoolYears() ([]SchoolYear, error) {
	rows, err := s.db().Query(`SELECT id, name, starts_on, ends_on, is_current
		FROM school_years ORDER BY starts_on DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var years []SchoolYear
	for rows.Next() {
		var y SchoolYear
		if err := rows.Scan(&y.ID, &y.Name, &y.StartsOn, &y.EndsOn, &y.IsCurrent); err != nil {
			return nil, err
		}
		years = append(years, y)
	}
	return years, rows.Err()
}

func (s *Store) SchoolYear(id int64) (SchoolYear, error) {
	var y SchoolYear
	err := s.db().QueryRow(`SELECT id, name, starts_on, ends_on, is_current
		FROM school_years WHERE id = ?`, id).
		Scan(&y.ID, &y.Name, &y.StartsOn, &y.EndsOn, &y.IsCurrent)
	return y, err
}

// CurrentSchoolYear returns the year marked current, or a zero value when the
// family has not set one up yet.
func (s *Store) CurrentSchoolYear() (SchoolYear, error) {
	var y SchoolYear
	err := s.db().QueryRow(`SELECT id, name, starts_on, ends_on, is_current
		FROM school_years WHERE is_current = 1 ORDER BY starts_on DESC LIMIT 1`).
		Scan(&y.ID, &y.Name, &y.StartsOn, &y.EndsOn, &y.IsCurrent)
	if err == sql.ErrNoRows {
		return SchoolYear{}, nil
	}
	return y, err
}

func (s *Store) CreateSchoolYear(name, startsOn, endsOn string, current bool) (int64, error) {
	tx, err := s.db().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if current {
		if _, err := tx.Exec(`UPDATE school_years SET is_current = 0`); err != nil {
			return 0, err
		}
	}
	res, err := tx.Exec(`INSERT INTO school_years (name, starts_on, ends_on, is_current) VALUES (?, ?, ?, ?)`,
		name, startsOn, endsOn, current)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) UpdateSchoolYear(id int64, name, startsOn, endsOn string, current bool) error {
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if current {
		if _, err := tx.Exec(`UPDATE school_years SET is_current = 0`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE school_years SET name = ?, starts_on = ?, ends_on = ?, is_current = ? WHERE id = ?`,
		name, startsOn, endsOn, current, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteSchoolYear(id int64) error {
	_, err := s.db().Exec(`DELETE FROM school_years WHERE id = ?`, id)
	return err
}
