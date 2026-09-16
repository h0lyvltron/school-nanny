package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var (
	errCurriculumLessonExists = errors.New("that curriculum lesson is already scheduled for that child on that date")
	errDeletedLessonExpired   = errors.New("that deleted lesson can no longer be restored")
)

const deletedLessonRetention = 7 * 24 * time.Hour

func (s *Store) ScheduleCurriculumItem(kidID, itemID int64, date string) (int64, error) {
	yearID, err := s.currentYearID()
	if err != nil {
		return 0, err
	}
	tx, err := s.db().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var item CurriculumItem
	var subjectID int64
	err = tx.QueryRow(`SELECT i.id, i.plan_id, i.sort_order, i.title, i.notes, i.minutes,
			COALESCE(i.week_number, 0), COALESCE(i.page_start, 0), COALESCE(i.page_end, 0),
			i.created_at, p.subject_id
		FROM curriculum_items i
		JOIN curriculum_plans p ON p.id = i.plan_id
		WHERE i.id = ?`, itemID).
		Scan(&item.ID, &item.PlanID, &item.SortOrder, &item.Title, &item.Notes,
			&item.Minutes, &item.WeekNumber, &item.PageStart, &item.PageEnd,
			&item.CreatedAt, &subjectID)
	if err != nil {
		return 0, err
	}
	var kidExists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM kids WHERE id = ? AND archived = 0`, kidID).Scan(&kidExists); err != nil {
		return 0, err
	}
	if kidExists == 0 {
		return 0, sql.ErrNoRows
	}
	var duplicate int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM lessons
		WHERE kid_id = ? AND curriculum_item_id = ? AND scheduled_on = ?`,
		kidID, itemID, date).Scan(&duplicate); err != nil {
		return 0, err
	}
	if duplicate > 0 {
		return 0, errCurriculumLessonExists
	}

	sequence := item.SortOrder
	var assignmentID int64
	_ = tx.QueryRow(`SELECT a.id
		FROM plan_assignments a
		WHERE a.kid_id = ? AND a.plan_id = ?
		  AND COALESCE(a.school_year_id, 0) = COALESCE(?, 0)
		  AND NOT EXISTS (
			SELECT 1 FROM lessons l
			WHERE l.assignment_id = a.id AND l.sequence = ?
		  )
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT 1`, kidID, item.PlanID, nullableID(yearID), sequence).Scan(&assignmentID)

	now := time.Now().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT INTO lessons
		(kid_id, adult_id, subject_id, school_year_id, series_id, assignment_id,
		 curriculum_item_id, sequence, scheduled_on, status, title, minutes, notes,
		 page_start, page_end, completed_at, created_at)
		VALUES (?, NULL, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		kidID, subjectID, nullableID(yearID), nullableID(assignmentID), item.ID, sequence,
		date, StatusPlanned, item.Title, item.Minutes, item.Notes,
		nullablePage(item.PageStart), nullablePage(item.PageEnd), now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) TrashLesson(id int64, now time.Time) (DeletedLesson, error) {
	tx, err := s.db().Begin()
	if err != nil {
		return DeletedLesson{}, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(lessonSelect+` WHERE l.id = ?`, id)
	if err != nil {
		return DeletedLesson{}, err
	}
	lessons, err := scanLessons(rows)
	if err != nil {
		return DeletedLesson{}, err
	}
	if len(lessons) == 0 {
		return DeletedLesson{}, sql.ErrNoRows
	}
	lesson := lessons[0]

	attachmentRows, err := tx.Query(attachmentSelect+` WHERE owner_type = ? AND lesson_id = ? ORDER BY id`,
		OwnerLesson, id)
	if err != nil {
		return DeletedLesson{}, err
	}
	attachments, err := scanAttachments(attachmentRows)
	if err != nil {
		return DeletedLesson{}, err
	}
	assessmentRows, err := tx.Query(`SELECT id FROM assessments WHERE lesson_id = ? ORDER BY id`, id)
	if err != nil {
		return DeletedLesson{}, err
	}
	var assessmentIDs []int64
	for assessmentRows.Next() {
		var assessmentID int64
		if err := assessmentRows.Scan(&assessmentID); err != nil {
			assessmentRows.Close()
			return DeletedLesson{}, err
		}
		assessmentIDs = append(assessmentIDs, assessmentID)
	}
	if err := assessmentRows.Close(); err != nil {
		return DeletedLesson{}, err
	}

	lessonJSON, err := json.Marshal(lesson)
	if err != nil {
		return DeletedLesson{}, err
	}
	attachmentsJSON, err := json.Marshal(attachments)
	if err != nil {
		return DeletedLesson{}, err
	}
	assessmentsJSON, err := json.Marshal(assessmentIDs)
	if err != nil {
		return DeletedLesson{}, err
	}
	token, err := lessonRestoreToken()
	if err != nil {
		return DeletedLesson{}, err
	}
	deletedAt := now.UTC()
	expiresAt := deletedAt.Add(deletedLessonRetention)
	deleted := DeletedLesson{
		Token: token, OriginalLessonID: id, Title: lesson.Title,
		ScheduledOn: lesson.ScheduledOn, PersonName: lesson.PersonName,
		DeletedAt: deletedAt.Format(time.RFC3339), ExpiresAt: expiresAt.Format(time.RFC3339),
	}
	if _, err := tx.Exec(`INSERT INTO deleted_lessons
		(token, original_lesson_id, title, scheduled_on, person_name, lesson_json,
		 attachments_json, assessment_ids_json, deleted_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		deleted.Token, deleted.OriginalLessonID, deleted.Title, deleted.ScheduledOn,
		deleted.PersonName, string(lessonJSON), string(attachmentsJSON),
		string(assessmentsJSON), deleted.DeletedAt, deleted.ExpiresAt); err != nil {
		return DeletedLesson{}, err
	}
	if _, err := tx.Exec(`DELETE FROM lessons WHERE id = ?`, id); err != nil {
		return DeletedLesson{}, err
	}
	return deleted, tx.Commit()
}

func (s *Store) RestoreLesson(token string, now time.Time) (Lesson, error) {
	tx, err := s.db().Begin()
	if err != nil {
		return Lesson{}, err
	}
	defer tx.Rollback()

	var lessonJSON, attachmentsJSON, assessmentsJSON, expires string
	err = tx.QueryRow(`SELECT lesson_json, attachments_json, assessment_ids_json, expires_at
		FROM deleted_lessons WHERE token = ?`, token).
		Scan(&lessonJSON, &attachmentsJSON, &assessmentsJSON, &expires)
	if err != nil {
		return Lesson{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339, expires)
	if err != nil || !now.UTC().Before(expiresAt) {
		return Lesson{}, errDeletedLessonExpired
	}
	var lesson Lesson
	var attachments []Attachment
	var assessmentIDs []int64
	if err := json.Unmarshal([]byte(lessonJSON), &lesson); err != nil {
		return Lesson{}, err
	}
	if err := json.Unmarshal([]byte(attachmentsJSON), &attachments); err != nil {
		return Lesson{}, err
	}
	if err := json.Unmarshal([]byte(assessmentsJSON), &assessmentIDs); err != nil {
		return Lesson{}, err
	}
	var completedAt any
	if lesson.CompletedAt != "" {
		completedAt = lesson.CompletedAt
	}
	if _, err := tx.Exec(`INSERT INTO lessons
		(id, kid_id, adult_id, subject_id, school_year_id, series_id, assignment_id,
		 curriculum_item_id, sequence, scheduled_on, status, title, minutes, notes,
		 page_start, page_end, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lesson.ID, nullableID(lesson.KidID), nullableID(lesson.AdultID), lesson.SubjectID,
		nullableID(lesson.SchoolYearID), nullableID(lesson.SeriesID), nullableID(lesson.AssignmentID),
		nullableID(lesson.CurriculumItemID), lesson.Sequence, lesson.ScheduledOn, lesson.Status,
		lesson.Title, lesson.Minutes, lesson.Notes, nullablePage(lesson.PageStart),
		nullablePage(lesson.PageEnd), completedAt, lesson.CreatedAt); err != nil {
		return Lesson{}, err
	}
	for _, attachment := range attachments {
		if _, err := tx.Exec(`INSERT INTO attachments
			(id, owner_type, lesson_id, assessment_id, kid_id, subject_id, curriculum_plan_id,
			 original_name, stored_path, size_bytes, content_type, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			attachment.ID, attachment.OwnerType, lesson.ID, nullableID(attachment.AssessmentID),
			nullableID(attachment.KidID), nullableID(attachment.SubjectID),
			nullableID(attachment.CurriculumPlanID), attachment.OriginalName,
			attachment.StoredPath, attachment.SizeBytes, attachment.ContentType,
			attachment.CreatedAt); err != nil {
			return Lesson{}, err
		}
	}
	for _, assessmentID := range assessmentIDs {
		if _, err := tx.Exec(`UPDATE assessments SET lesson_id = ?
			WHERE id = ? AND lesson_id IS NULL`, lesson.ID, assessmentID); err != nil {
			return Lesson{}, err
		}
	}
	if _, err := tx.Exec(`DELETE FROM deleted_lessons WHERE token = ?`, token); err != nil {
		return Lesson{}, err
	}
	return lesson, tx.Commit()
}

func (s *Store) DeletedLessons(now time.Time) ([]DeletedLesson, error) {
	rows, err := s.db().Query(`SELECT token, original_lesson_id, title, scheduled_on,
			person_name, deleted_at, expires_at
		FROM deleted_lessons WHERE expires_at > ?
		ORDER BY deleted_at DESC`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeletedLesson
	for rows.Next() {
		var deleted DeletedLesson
		if err := rows.Scan(&deleted.Token, &deleted.OriginalLessonID, &deleted.Title,
			&deleted.ScheduledOn, &deleted.PersonName, &deleted.DeletedAt,
			&deleted.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, deleted)
	}
	return out, rows.Err()
}

func (s *Store) DeletedLesson(token string, now time.Time) (DeletedLesson, error) {
	var deleted DeletedLesson
	err := s.db().QueryRow(`SELECT token, original_lesson_id, title, scheduled_on,
			person_name, deleted_at, expires_at
		FROM deleted_lessons WHERE token = ? AND expires_at > ?`,
		token, now.UTC().Format(time.RFC3339)).
		Scan(&deleted.Token, &deleted.OriginalLessonID, &deleted.Title,
			&deleted.ScheduledOn, &deleted.PersonName, &deleted.DeletedAt,
			&deleted.ExpiresAt)
	return deleted, err
}

func (s *Store) PurgeExpiredDeletedLessons(now time.Time) ([]string, error) {
	tx, err := s.db().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT attachments_json FROM deleted_lessons WHERE expires_at <= ?`,
		now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	var paths []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		var attachments []Attachment
		if json.Unmarshal([]byte(raw), &attachments) == nil {
			for _, attachment := range attachments {
				paths = append(paths, attachment.StoredPath)
			}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM deleted_lessons WHERE expires_at <= ?`,
		now.UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	return paths, tx.Commit()
}

func lessonRestoreToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
