package main

import (
	"database/sql"
	"time"
)

const assessmentSelect = `SELECT a.id, a.kid_id, a.subject_id, COALESCE(a.lesson_id, 0),
		COALESCE(a.school_year_id, 0), a.given_on, a.name, a.score, a.max_score,
		a.letter, a.notes, a.created_at, k.name, k.color, s.name
	FROM assessments a
	JOIN kids k ON k.id = a.kid_id
	JOIN subjects s ON s.id = a.subject_id`

func scanAssessments(rows *sql.Rows) ([]Assessment, error) {
	defer rows.Close()
	var out []Assessment
	for rows.Next() {
		var a Assessment
		var score, maxScore sql.NullFloat64
		if err := rows.Scan(&a.ID, &a.KidID, &a.SubjectID, &a.LessonID, &a.SchoolYearID,
			&a.GivenOn, &a.Name, &score, &maxScore, &a.Letter, &a.Notes, &a.CreatedAt,
			&a.KidName, &a.KidColor, &a.SubjectName); err != nil {
			return nil, err
		}
		if score.Valid {
			a.Score = &score.Float64
		}
		if maxScore.Valid {
			a.MaxScore = &maxScore.Float64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Assessment(id int64) (Assessment, error) {
	rows, err := s.db.Query(assessmentSelect+` WHERE a.id = ?`, id)
	if err != nil {
		return Assessment{}, err
	}
	list, err := scanAssessments(rows)
	if err != nil {
		return Assessment{}, err
	}
	if len(list) == 0 {
		return Assessment{}, sql.ErrNoRows
	}
	return list[0], nil
}

// Assessments lists tests for a child, newest first, optionally in one subject.
func (s *Store) Assessments(kidID, subjectID int64, limit int) ([]Assessment, error) {
	q := assessmentSelect + ` WHERE a.kid_id = ?`
	args := []any{kidID}
	if subjectID > 0 {
		q += ` AND a.subject_id = ?`
		args = append(args, subjectID)
	}
	q += ` ORDER BY a.given_on DESC, a.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	return scanAssessments(rows)
}

func (s *Store) AssessmentsForLesson(lessonID int64) ([]Assessment, error) {
	rows, err := s.db.Query(assessmentSelect+` WHERE a.lesson_id = ? ORDER BY a.given_on DESC`, lessonID)
	if err != nil {
		return nil, err
	}
	return scanAssessments(rows)
}

func (s *Store) CreateAssessment(a Assessment) (int64, error) {
	yearID, err := s.currentYearID()
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO assessments
		(kid_id, subject_id, lesson_id, school_year_id, given_on, name, score, max_score, letter, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.KidID, a.SubjectID, nullableID(a.LessonID), nullableID(yearID), a.GivenOn, a.Name,
		nullableFloat(a.Score), nullableFloat(a.MaxScore), a.Letter, a.Notes,
		time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAssessment(id int64, a Assessment) error {
	_, err := s.db.Exec(`UPDATE assessments
		SET subject_id = ?, given_on = ?, name = ?, score = ?, max_score = ?, letter = ?, notes = ?
		WHERE id = ?`,
		a.SubjectID, a.GivenOn, a.Name, nullableFloat(a.Score), nullableFloat(a.MaxScore),
		a.Letter, a.Notes, id)
	return err
}

func (s *Store) DeleteAssessment(id int64) error {
	_, err := s.db.Exec(`DELETE FROM assessments WHERE id = ?`, id)
	return err
}

func (s *Store) Notes(kidID, subjectID int64, limit int) ([]Note, error) {
	q := `SELECT n.id, n.kid_id, COALESCE(n.subject_id, 0), n.noted_on, n.body, n.created_at,
			COALESCE(s.name, '')
		FROM notes n
		LEFT JOIN subjects s ON s.id = n.subject_id
		WHERE n.kid_id = ?`
	args := []any{kidID}
	if subjectID > 0 {
		q += ` AND n.subject_id = ?`
		args = append(args, subjectID)
	}
	q += ` ORDER BY n.noted_on DESC, n.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.KidID, &n.SubjectID, &n.NotedOn, &n.Body, &n.CreatedAt, &n.SubjectName); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) CreateNote(n Note) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO notes (kid_id, subject_id, noted_on, body, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		n.KidID, nullableID(n.SubjectID), n.NotedOn, n.Body, time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteNote(id int64) error {
	_, err := s.db.Exec(`DELETE FROM notes WHERE id = ?`, id)
	return err
}

const attachmentSelect = `SELECT id, owner_type, COALESCE(lesson_id, 0), COALESCE(assessment_id, 0),
		COALESCE(kid_id, 0), COALESCE(subject_id, 0), original_name, stored_path,
		size_bytes, content_type, created_at
	FROM attachments`

func scanAttachments(rows *sql.Rows) ([]Attachment, error) {
	defer rows.Close()
	var out []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.OwnerType, &a.LessonID, &a.AssessmentID, &a.KidID,
			&a.SubjectID, &a.OriginalName, &a.StoredPath, &a.SizeBytes, &a.ContentType,
			&a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Attachment(id int64) (Attachment, error) {
	rows, err := s.db.Query(attachmentSelect+` WHERE id = ?`, id)
	if err != nil {
		return Attachment{}, err
	}
	list, err := scanAttachments(rows)
	if err != nil {
		return Attachment{}, err
	}
	if len(list) == 0 {
		return Attachment{}, sql.ErrNoRows
	}
	return list[0], nil
}

func (s *Store) AttachmentsForLesson(lessonID int64) ([]Attachment, error) {
	rows, err := s.db.Query(attachmentSelect+` WHERE owner_type = ? AND lesson_id = ? ORDER BY id`,
		OwnerLesson, lessonID)
	if err != nil {
		return nil, err
	}
	return scanAttachments(rows)
}

func (s *Store) AttachmentsForAssessment(assessmentID int64) ([]Attachment, error) {
	rows, err := s.db.Query(attachmentSelect+` WHERE owner_type = ? AND assessment_id = ? ORDER BY id`,
		OwnerAssessment, assessmentID)
	if err != nil {
		return nil, err
	}
	return scanAttachments(rows)
}

// ResourceAttachments returns the reusable file pile for a child and subject:
// curriculum PDFs, worksheets, anything worth keeping around.
func (s *Store) ResourceAttachments(kidID, subjectID int64) ([]Attachment, error) {
	rows, err := s.db.Query(attachmentSelect+` WHERE owner_type = ? AND kid_id = ? AND subject_id = ?
		ORDER BY id DESC`, OwnerResource, kidID, subjectID)
	if err != nil {
		return nil, err
	}
	return scanAttachments(rows)
}

func (s *Store) CreateAttachment(a Attachment) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO attachments
		(owner_type, lesson_id, assessment_id, kid_id, subject_id, original_name, stored_path,
		 size_bytes, content_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.OwnerType, nullableID(a.LessonID), nullableID(a.AssessmentID), nullableID(a.KidID),
		nullableID(a.SubjectID), a.OriginalName, a.StoredPath, a.SizeBytes, a.ContentType,
		time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteAttachment(id int64) error {
	_, err := s.db.Exec(`DELETE FROM attachments WHERE id = ?`, id)
	return err
}

func nullableFloat(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
