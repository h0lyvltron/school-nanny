package main

import (
	"database/sql"
	"time"
)

const seriesSelect = `SELECT sr.id, sr.kid_id, sr.subject_id, COALESCE(sr.school_year_id, 0),
		sr.title, sr.minutes, sr.notes, sr.weekdays, sr.starts_on, sr.ends_on,
		sr.occurrence_count, sr.created_at, k.name, k.color, s.name
	FROM lesson_series sr
	JOIN kids k ON k.id = sr.kid_id
	JOIN subjects s ON s.id = sr.subject_id`

func scanSeries(rows *sql.Rows) ([]LessonSeries, error) {
	defer rows.Close()
	var out []LessonSeries
	for rows.Next() {
		var sr LessonSeries
		if err := rows.Scan(&sr.ID, &sr.KidID, &sr.SubjectID, &sr.SchoolYearID,
			&sr.Title, &sr.Minutes, &sr.Notes, &sr.Weekdays, &sr.StartsOn, &sr.EndsOn,
			&sr.OccurrenceCount, &sr.CreatedAt, &sr.KidName, &sr.KidColor, &sr.SubjectName); err != nil {
			return nil, err
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

func (s *Store) Series(id int64) (LessonSeries, error) {
	rows, err := s.db().Query(seriesSelect+` WHERE sr.id = ?`, id)
	if err != nil {
		return LessonSeries{}, err
	}
	list, err := scanSeries(rows)
	if err != nil {
		return LessonSeries{}, err
	}
	if len(list) == 0 {
		return LessonSeries{}, sql.ErrNoRows
	}
	return list[0], nil
}

func (s *Store) LessonsForSeries(seriesID int64) ([]Lesson, error) {
	rows, err := s.db().Query(lessonSelect+` WHERE l.series_id = ? ORDER BY l.scheduled_on, l.id`, seriesID)
	if err != nil {
		return nil, err
	}
	return scanLessons(rows)
}

func (s *Store) CreateSeries(sr LessonSeries, dates []string) (int64, error) {
	yearID, err := s.currentYearID()
	if err != nil {
		return 0, err
	}

	tx, err := s.db().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT INTO lesson_series
		(kid_id, subject_id, school_year_id, title, minutes, notes, weekdays, starts_on, ends_on,
		 occurrence_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sr.KidID, sr.SubjectID, nullableID(yearID), sr.Title, sr.Minutes, sr.Notes, sr.Weekdays,
		sr.StartsOn, sr.EndsOn, sr.OccurrenceCount, now)
	if err != nil {
		return 0, err
	}
	seriesID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, date := range dates {
		if _, err := tx.Exec(`INSERT INTO lessons
			(kid_id, subject_id, school_year_id, series_id, scheduled_on, status, title, minutes, notes,
			 completed_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
			sr.KidID, sr.SubjectID, nullableID(yearID), seriesID, date, StatusPlanned,
			sr.Title, sr.Minutes, sr.Notes, now); err != nil {
			return 0, err
		}
	}
	return seriesID, tx.Commit()
}

func (s *Store) UpdateSeries(id int64, sr LessonSeries) error {
	_, err := s.db().Exec(`UPDATE lesson_series
		SET kid_id = ?, subject_id = ?, title = ?, minutes = ?, notes = ?, weekdays = ?,
			starts_on = ?, ends_on = ?, occurrence_count = ?
		WHERE id = ?`,
		sr.KidID, sr.SubjectID, sr.Title, sr.Minutes, sr.Notes, sr.Weekdays,
		sr.StartsOn, sr.EndsOn, sr.OccurrenceCount, id)
	return err
}

func (s *Store) UpdateFutureSeriesLessons(seriesID int64, sr LessonSeries, from string) error {
	_, err := s.db().Exec(`UPDATE lessons
		SET kid_id = ?, subject_id = ?, title = ?, minutes = ?, notes = ?
		WHERE series_id = ? AND status = ? AND scheduled_on >= ?`,
		sr.KidID, sr.SubjectID, sr.Title, sr.Minutes, sr.Notes,
		seriesID, StatusPlanned, from)
	return err
}

func (s *Store) PlannedSeriesIDsFrom(seriesID int64, from string) ([]int64, error) {
	rows, err := s.db().Query(`SELECT id FROM lessons
		WHERE series_id = ? AND status = ? AND scheduled_on >= ?`,
		seriesID, StatusPlanned, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) DeletePlannedSeriesFrom(seriesID int64, from string) error {
	_, err := s.db().Exec(`DELETE FROM lessons
		WHERE series_id = ? AND status = ? AND scheduled_on >= ?`,
		seriesID, StatusPlanned, from)
	return err
}

func (s *Store) RematerializeSeries(sr LessonSeries, from string) error {
	existing, err := s.LessonsForSeries(sr.ID)
	if err != nil {
		return err
	}
	taken := map[string]bool{}
	for _, l := range existing {
		if l.Status != StatusPlanned || l.ScheduledOn < from {
			taken[l.ScheduledOn] = true
		}
	}

	dates, err := occurrenceDates(sr.StartsOn, sr.EndsOn, sr.OccurrenceCount, parseWeekdays(sr.Weekdays))
	if err != nil {
		return err
	}

	yearID, err := s.currentYearID()
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)

	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, date := range dates {
		if date < from || taken[date] {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO lessons
			(kid_id, subject_id, school_year_id, series_id, scheduled_on, status, title, minutes, notes,
			 completed_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
			sr.KidID, sr.SubjectID, nullableID(yearID), sr.ID, date, StatusPlanned,
			sr.Title, sr.Minutes, sr.Notes, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ApplyCurriculum(plan CurriculumPlan, kidID int64, dates []string) error {
	if len(plan.Items) == 0 {
		return nil
	}
	yearID, err := s.currentYearID()
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)

	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	n := len(plan.Items)
	if len(dates) < n {
		n = len(dates)
	}
	for i := 0; i < n; i++ {
		it := plan.Items[i]
		if _, err := tx.Exec(`INSERT INTO lessons
			(kid_id, subject_id, school_year_id, series_id, scheduled_on, status, title, minutes, notes,
			 completed_at, created_at)
			VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, NULL, ?)`,
			kidID, plan.SubjectID, nullableID(yearID), dates[i], StatusPlanned,
			it.Title, it.Minutes, it.Notes, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
