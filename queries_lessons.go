package main

import (
	"database/sql"
	"strings"
	"time"
)

const lessonSelect = `SELECT l.id, l.kid_id, l.subject_id, COALESCE(l.school_year_id, 0),
		l.scheduled_on, l.status, l.title, l.minutes, l.notes,
		COALESCE(l.completed_at, ''), l.created_at,
		k.name, k.color, s.name
	FROM lessons l
	JOIN kids k ON k.id = l.kid_id
	JOIN subjects s ON s.id = l.subject_id`

func scanLessons(rows *sql.Rows) ([]Lesson, error) {
	defer rows.Close()
	var lessons []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.KidID, &l.SubjectID, &l.SchoolYearID,
			&l.ScheduledOn, &l.Status, &l.Title, &l.Minutes, &l.Notes,
			&l.CompletedAt, &l.CreatedAt,
			&l.KidName, &l.KidColor, &l.SubjectName); err != nil {
			return nil, err
		}
		lessons = append(lessons, l)
	}
	return lessons, rows.Err()
}

func (s *Store) Lesson(id int64) (Lesson, error) {
	rows, err := s.db.Query(lessonSelect+` WHERE l.id = ?`, id)
	if err != nil {
		return Lesson{}, err
	}
	lessons, err := scanLessons(rows)
	if err != nil {
		return Lesson{}, err
	}
	if len(lessons) == 0 {
		return Lesson{}, sql.ErrNoRows
	}
	return lessons[0], nil
}

// LessonsBetween returns everything scheduled in a date range, optionally for
// one child. kidID of 0 means every child.
func (s *Store) LessonsBetween(from, to string, kidID int64) ([]Lesson, error) {
	q := lessonSelect + ` WHERE l.scheduled_on BETWEEN ? AND ?`
	args := []any{from, to}
	if kidID > 0 {
		q += ` AND l.kid_id = ?`
		args = append(args, kidID)
	}
	q += ` ORDER BY l.scheduled_on, k.sort_order, s.sort_order, l.id`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	return scanLessons(rows)
}

// LessonsOverdue lists planned lessons whose day has passed, oldest first.
func (s *Store) LessonsOverdue(before string, limit int) ([]Lesson, error) {
	rows, err := s.db.Query(lessonSelect+` WHERE l.status = ? AND l.scheduled_on < ?
		ORDER BY l.scheduled_on, k.sort_order LIMIT ?`, StatusPlanned, before, limit)
	if err != nil {
		return nil, err
	}
	return scanLessons(rows)
}

// LessonsForSubject lists a child's work in one subject, newest first.
func (s *Store) LessonsForSubject(kidID, subjectID int64, limit int) ([]Lesson, error) {
	rows, err := s.db.Query(lessonSelect+` WHERE l.kid_id = ? AND l.subject_id = ?
		ORDER BY l.scheduled_on DESC, l.id DESC LIMIT ?`, kidID, subjectID, limit)
	if err != nil {
		return nil, err
	}
	return scanLessons(rows)
}

func (s *Store) CreateLesson(l Lesson) (int64, error) {
	yearID, err := s.currentYearID()
	if err != nil {
		return 0, err
	}
	var completedAt any
	if l.Status == StatusDone {
		completedAt = time.Now().Format(time.RFC3339)
	}
	res, err := s.db.Exec(`INSERT INTO lessons
		(kid_id, subject_id, school_year_id, scheduled_on, status, title, minutes, notes, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.KidID, l.SubjectID, nullableID(yearID), l.ScheduledOn, l.Status, l.Title, l.Minutes, l.Notes,
		completedAt, time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateLesson(id int64, l Lesson) error {
	_, err := s.db.Exec(`UPDATE lessons
		SET kid_id = ?, subject_id = ?, scheduled_on = ?, title = ?, minutes = ?, notes = ?
		WHERE id = ?`,
		l.KidID, l.SubjectID, l.ScheduledOn, l.Title, l.Minutes, l.Notes, id)
	return err
}

// SetLessonStatus moves a lesson between planned, done, and skipped, stamping
// or clearing the completion time to match.
func (s *Store) SetLessonStatus(id int64, status string) error {
	var completedAt any
	if status == StatusDone {
		completedAt = time.Now().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`UPDATE lessons SET status = ?, completed_at = ? WHERE id = ?`,
		status, completedAt, id)
	return err
}

func (s *Store) RescheduleLesson(id int64, date string) error {
	_, err := s.db.Exec(`UPDATE lessons SET scheduled_on = ? WHERE id = ?`, date, id)
	return err
}

func (s *Store) DeleteLesson(id int64) error {
	_, err := s.db.Exec(`DELETE FROM lessons WHERE id = ?`, id)
	return err
}

// ProgressBetween totals lesson statuses and minutes for a date range,
// optionally narrowed to one child and one subject.
func (s *Store) ProgressBetween(from, to string, kidID, subjectID int64) (Progress, error) {
	q := `SELECT
			COALESCE(SUM(status = 'planned'), 0),
			COALESCE(SUM(status = 'done'), 0),
			COALESCE(SUM(status = 'skipped'), 0),
			COALESCE(SUM(CASE WHEN status = 'done' THEN minutes ELSE 0 END), 0)
		FROM lessons WHERE scheduled_on BETWEEN ? AND ?`
	args := []any{from, to}
	if kidID > 0 {
		q += ` AND kid_id = ?`
		args = append(args, kidID)
	}
	if subjectID > 0 {
		q += ` AND subject_id = ?`
		args = append(args, subjectID)
	}

	var p Progress
	err := s.db.QueryRow(q, args...).Scan(&p.Planned, &p.Done, &p.Skipped, &p.Minutes)
	return p, err
}

// ProgressBySubject totals a child's lessons per subject in one pass, so a
// dashboard with a card per subject stays a single query.
func (s *Store) ProgressBySubject(from, to string, kidID int64) (map[int64]Progress, error) {
	rows, err := s.db.Query(`SELECT subject_id,
			COALESCE(SUM(status = 'planned'), 0),
			COALESCE(SUM(status = 'done'), 0),
			COALESCE(SUM(status = 'skipped'), 0),
			COALESCE(SUM(CASE WHEN status = 'done' THEN minutes ELSE 0 END), 0)
		FROM lessons
		WHERE kid_id = ? AND scheduled_on BETWEEN ? AND ?
		GROUP BY subject_id`, kidID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]Progress{}
	for rows.Next() {
		var subjectID int64
		var p Progress
		if err := rows.Scan(&subjectID, &p.Planned, &p.Done, &p.Skipped, &p.Minutes); err != nil {
			return nil, err
		}
		out[subjectID] = p
	}
	return out, rows.Err()
}

// LessonsForKidByStatus splits a child's subject work into what is still
// coming up and what has already happened.
func (s *Store) LessonsForKidSubjectSplit(kidID, subjectID int64, limit int) (upcoming, past []Lesson, err error) {
	lessons, err := s.LessonsForSubject(kidID, subjectID, limit)
	if err != nil {
		return nil, nil, err
	}
	now := today()
	for _, l := range lessons {
		if l.Status == StatusPlanned && l.ScheduledOn >= now {
			upcoming = append(upcoming, l)
		} else {
			past = append(past, l)
		}
	}
	// Upcoming reads better soonest-first; the query returned newest-first.
	for i, j := 0, len(upcoming)-1; i < j; i, j = i+1, j-1 {
		upcoming[i], upcoming[j] = upcoming[j], upcoming[i]
	}
	return upcoming, past, nil
}

func (s *Store) currentYearID() (int64, error) {
	year, err := s.CurrentSchoolYear()
	if err != nil {
		return 0, err
	}
	return year.ID, nil
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// weekStart snaps a date to the Monday of its week, which is how the planner
// grid is laid out.
func weekStart(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, t.Location())
}

// parseDate accepts a YYYY-MM-DD string, falling back to today when it is
// missing or malformed so a bad URL never breaks a page.
func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(dateLayout, value); err == nil {
		return t
	}
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
