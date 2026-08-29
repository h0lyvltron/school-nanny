package main

import (
	"database/sql"
	"fmt"
	"time"
)

const planSelect = `SELECT p.id, p.name, p.subject_id, p.kind, COALESCE(p.source_kid_id, 0),
		COALESCE(p.source_year_id, 0), p.notes, p.created_at, s.name,
		(SELECT COUNT(*) FROM curriculum_items i WHERE i.plan_id = p.id)
	FROM curriculum_plans p
	JOIN subjects s ON s.id = p.subject_id`

func scanPlans(rows *sql.Rows) ([]CurriculumPlan, error) {
	defer rows.Close()
	var out []CurriculumPlan
	for rows.Next() {
		var p CurriculumPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.SubjectID, &p.Kind, &p.SourceKidID,
			&p.SourceYearID, &p.Notes, &p.CreatedAt, &p.SubjectName, &p.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CurriculumPlans() ([]CurriculumPlan, error) {
	rows, err := s.db().Query(planSelect + ` ORDER BY s.sort_order, p.name, p.id`)
	if err != nil {
		return nil, err
	}
	return scanPlans(rows)
}

func (s *Store) CurriculumPlan(id int64) (CurriculumPlan, error) {
	rows, err := s.db().Query(planSelect+` WHERE p.id = ?`, id)
	if err != nil {
		return CurriculumPlan{}, err
	}
	list, err := scanPlans(rows)
	if err != nil {
		return CurriculumPlan{}, err
	}
	if len(list) == 0 {
		return CurriculumPlan{}, sql.ErrNoRows
	}
	plan := list[0]
	plan.Items, err = s.CurriculumItems(plan.ID)
	if err != nil {
		return CurriculumPlan{}, err
	}
	plan.Attachments, err = s.AttachmentsForPlan(plan.ID)
	return plan, err
}

func (s *Store) CreateCurriculumPlan(p CurriculumPlan) (int64, error) {
	if p.Kind == "" {
		p.Kind = PlanAuthored
	}
	res, err := s.db().Exec(`INSERT INTO curriculum_plans
		(name, subject_id, kind, source_kid_id, source_year_id, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.SubjectID, p.Kind, nullableID(p.SourceKidID), nullableID(p.SourceYearID),
		p.Notes, time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateCurriculumPlan(id int64, p CurriculumPlan) error {
	_, err := s.db().Exec(`UPDATE curriculum_plans
		SET name = ?, subject_id = ?, notes = ? WHERE id = ?`,
		p.Name, p.SubjectID, p.Notes, id)
	return err
}

func (s *Store) DeleteCurriculumPlan(id int64) error {
	_, err := s.db().Exec(`DELETE FROM curriculum_plans WHERE id = ?`, id)
	return err
}

func (s *Store) CurriculumItems(planID int64) ([]CurriculumItem, error) {
	rows, err := s.db().Query(`SELECT id, plan_id, sort_order, title, notes, minutes,
			COALESCE(week_number, 0), created_at
		FROM curriculum_items WHERE plan_id = ? ORDER BY sort_order, id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CurriculumItem
	for rows.Next() {
		var it CurriculumItem
		if err := rows.Scan(&it.ID, &it.PlanID, &it.SortOrder, &it.Title, &it.Notes,
			&it.Minutes, &it.WeekNumber, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) CreateCurriculumItem(it CurriculumItem) (int64, error) {
	var next int
	if err := s.db().QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM curriculum_items WHERE plan_id = ?`,
		it.PlanID).Scan(&next); err != nil {
		return 0, err
	}
	res, err := s.db().Exec(`INSERT INTO curriculum_items
		(plan_id, sort_order, title, notes, minutes, week_number, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		it.PlanID, next, it.Title, it.Notes, it.Minutes, nullableWeek(it.WeekNumber),
		time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateCurriculumItem(id int64, it CurriculumItem) error {
	_, err := s.db().Exec(`UPDATE curriculum_items
		SET title = ?, notes = ?, minutes = ?, week_number = ? WHERE id = ?`,
		it.Title, it.Notes, it.Minutes, nullableWeek(it.WeekNumber), id)
	return err
}

func (s *Store) DeleteCurriculumItem(id int64) error {
	_, err := s.db().Exec(`DELETE FROM curriculum_items WHERE id = ?`, id)
	return err
}

func (s *Store) MoveCurriculumItem(id int64, dir int) error {
	var planID int64
	var order int
	err := s.db().QueryRow(`SELECT plan_id, sort_order FROM curriculum_items WHERE id = ?`, id).
		Scan(&planID, &order)
	if err != nil {
		return err
	}

	q := `SELECT id, sort_order FROM curriculum_items WHERE plan_id = ? AND sort_order `
	if dir < 0 {
		q += `< ? ORDER BY sort_order DESC, id DESC LIMIT 1`
	} else {
		q += `> ? ORDER BY sort_order, id LIMIT 1`
	}

	var otherID int64
	var otherOrder int
	err = s.db().QueryRow(q, planID, order).Scan(&otherID, &otherOrder)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE curriculum_items SET sort_order = ? WHERE id = ?`, otherOrder, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE curriculum_items SET sort_order = ? WHERE id = ?`, order, otherID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AttachmentsForPlan(planID int64) ([]Attachment, error) {
	rows, err := s.db().Query(attachmentSelect+` WHERE owner_type = ? AND curriculum_plan_id = ? ORDER BY id`,
		OwnerCurriculum, planID)
	if err != nil {
		return nil, err
	}
	return scanAttachments(rows)
}

func (s *Store) CreatePlanFromLessons(name string, subjectID, sourceKidID, sourceYearID int64, lessons []Lesson) (int64, error) {
	tx, err := s.db().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT INTO curriculum_plans
		(name, subject_id, kind, source_kid_id, source_year_id, notes, created_at)
		VALUES (?, ?, ?, ?, ?, '', ?)`,
		name, subjectID, PlanFromYear, nullableID(sourceKidID), nullableID(sourceYearID), now)
	if err != nil {
		return 0, err
	}
	planID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, l := range lessons {
		if _, err := tx.Exec(`INSERT INTO curriculum_items
			(plan_id, sort_order, title, notes, minutes, week_number, created_at)
			VALUES (?, ?, ?, ?, ?, NULL, ?)`,
			planID, i+1, l.Title, l.Notes, l.Minutes, now); err != nil {
			return 0, err
		}
	}
	return planID, tx.Commit()
}

func (s *Store) ImportCurriculum(plans []CurriculumPlan) (int, error) {
	if len(plans) == 0 {
		return 0, fmt.Errorf("that file has no plans in it")
	}
	tx, err := s.db().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	for _, p := range plans {
		res, err := tx.Exec(`INSERT INTO curriculum_plans
			(name, subject_id, kind, source_kid_id, source_year_id, notes, created_at)
			VALUES (?, ?, ?, NULL, NULL, ?, ?)`,
			p.Name, p.SubjectID, PlanAuthored, p.Notes, now)
		if err != nil {
			return 0, err
		}
		planID, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		for i, it := range p.Items {
			order := it.SortOrder
			if order <= 0 {
				order = i + 1
			}
			if _, err := tx.Exec(`INSERT INTO curriculum_items
				(plan_id, sort_order, title, notes, minutes, week_number, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				planID, order, it.Title, it.Notes, it.Minutes, nullableWeek(it.WeekNumber), now); err != nil {
				return 0, err
			}
		}
	}
	return len(plans), tx.Commit()
}

func nullableWeek(n int) any {
	if n <= 0 {
		return nil
	}
	return n
}
