package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const assignmentSelect = `SELECT a.id, a.kid_id, a.subject_id, COALESCE(a.school_year_id, 0),
		COALESCE(a.plan_id, 0), a.name, a.weekdays, a.starts_on, a.created_at,
		k.name, k.color, s.name
	FROM plan_assignments a
	JOIN kids k ON k.id = a.kid_id
	JOIN subjects s ON s.id = a.subject_id`

func scanAssignments(rows *sql.Rows) ([]PlanAssignment, error) {
	defer rows.Close()
	var out []PlanAssignment
	for rows.Next() {
		var a PlanAssignment
		if err := rows.Scan(&a.ID, &a.KidID, &a.SubjectID, &a.SchoolYearID,
			&a.PlanID, &a.Name, &a.Weekdays, &a.StartsOn, &a.CreatedAt,
			&a.KidName, &a.KidColor, &a.SubjectName); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Assignment(id int64) (PlanAssignment, error) {
	rows, err := s.db().Query(assignmentSelect+` WHERE a.id = ?`, id)
	if err != nil {
		return PlanAssignment{}, err
	}
	list, err := scanAssignments(rows)
	if err != nil {
		return PlanAssignment{}, err
	}
	if len(list) == 0 {
		return PlanAssignment{}, sql.ErrNoRows
	}
	return list[0], nil
}

func (s *Store) AssignmentsForPlan(planID int64) ([]PlanAssignment, error) {
	rows, err := s.db().Query(assignmentSelect+` WHERE a.plan_id = ? ORDER BY k.sort_order, a.starts_on, a.id`, planID)
	if err != nil {
		return nil, err
	}
	return scanAssignments(rows)
}

func (s *Store) LessonsForAssignment(assignmentID int64) ([]Lesson, error) {
	rows, err := s.db().Query(lessonSelect+` WHERE l.assignment_id = ? ORDER BY l.sequence, l.id`, assignmentID)
	if err != nil {
		return nil, err
	}
	return scanLessons(rows)
}

func (s *Store) ApplyCurriculum(plan CurriculumPlan, kidID int64, dates []string, weekdays string) error {
	if len(plan.Items) == 0 {
		return nil
	}
	if weekdays == "" {
		weekdays = defaultWeekdays()
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
	if n == 0 {
		return nil
	}

	res, err := tx.Exec(`INSERT INTO plan_assignments
		(kid_id, subject_id, school_year_id, plan_id, name, weekdays, starts_on, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		kidID, plan.SubjectID, nullableID(yearID), nullableID(plan.ID), plan.Name,
		weekdays, dates[0], now)
	if err != nil {
		return err
	}
	assignmentID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		it := plan.Items[i]
		seq := it.SortOrder
		if seq <= 0 {
			seq = i + 1
		}
		if _, err := tx.Exec(`INSERT INTO lessons
			(kid_id, subject_id, school_year_id, series_id, assignment_id, sequence,
			 scheduled_on, status, title, minutes, notes, completed_at, created_at)
			VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
			kidID, plan.SubjectID, nullableID(yearID), assignmentID, seq, dates[i], StatusPlanned,
			it.Title, it.Minutes, it.Notes, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateAssignment(id int64, name, weekdays string) error {
	_, err := s.db().Exec(`UPDATE plan_assignments SET name = ?, weekdays = ? WHERE id = ?`,
		name, weekdays, id)
	return err
}

func (s *Store) PlannedAssignmentIDsFrom(assignmentID int64, fromSequence int) ([]int64, error) {
	rows, err := s.db().Query(`SELECT id FROM lessons
		WHERE assignment_id = ? AND status = ? AND sequence >= ?`,
		assignmentID, StatusPlanned, fromSequence)
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

func (s *Store) DeletePlannedAssignmentFrom(assignmentID int64, from string) error {
	_, err := s.db().Exec(`DELETE FROM lessons
		WHERE assignment_id = ? AND status = ? AND scheduled_on >= ?`,
		assignmentID, StatusPlanned, from)
	return err
}

func (s *Store) DeletePlannedAssignmentFromSequence(assignmentID int64, fromSequence int) error {
	_, err := s.db().Exec(`DELETE FROM lessons
		WHERE assignment_id = ? AND status = ? AND sequence >= ?`,
		assignmentID, StatusPlanned, fromSequence)
	return err
}

func (s *Store) RelayoutPlannedFrom(asg PlanAssignment, resumeOn string, fromSequence int) error {
	rows, err := s.db().Query(lessonSelect+`
		WHERE l.assignment_id = ? AND l.status = ? AND l.sequence >= ?
		ORDER BY l.sequence, l.id`, asg.ID, StatusPlanned, fromSequence)
	if err != nil {
		return err
	}
	planned, err := scanLessons(rows)
	if err != nil {
		return err
	}
	return s.relayoutLessons(asg, resumeOn, planned)
}

// relayoutLessons drops the given lessons onto consecutive school days from
// resumeOn, in the order they arrive. Callers decide which lessons move and
// where the run begins; this only deals out the days.
func (s *Store) relayoutLessons(asg PlanAssignment, resumeOn string, planned []Lesson) error {
	if len(planned) == 0 {
		return nil
	}

	resumeOn = nextMatchingWeekdayOnOrAfter(resumeOn, parseWeekdays(asg.Weekdays))
	dates, err := occurrenceDates(resumeOn, "", len(planned), parseWeekdays(asg.Weekdays))
	if err != nil {
		return err
	}
	if len(dates) < len(planned) {
		return fmt.Errorf("not enough school days to shift %d remaining lessons", len(planned))
	}

	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, l := range planned {
		if _, err := tx.Exec(`UPDATE lessons SET scheduled_on = ? WHERE id = ?`, dates[i], l.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PushAssignmentLesson(lesson Lesson) error {
	if lesson.AssignmentID == 0 {
		return fmt.Errorf("that lesson is not part of a scheduled plan")
	}
	if lesson.Status != StatusPlanned {
		return fmt.Errorf("only a planned lesson can be pushed")
	}
	asg, err := s.Assignment(lesson.AssignmentID)
	if err != nil {
		return err
	}
	resume := pushResumeOn(today(), lesson.ScheduledOn, asg.Weekdays)
	return s.RelayoutPlannedFrom(asg, resume, lesson.Sequence)
}

// PullAssignmentLesson is Push read backwards: the children want to carry on,
// so tomorrow's lesson is brought onto today and the rest of the plan closes
// up behind it. Unlike Push there is no floor beyond today, because moving
// work earlier is the whole point.
func (s *Store) PullAssignmentLesson(lesson Lesson) error {
	if lesson.AssignmentID == 0 {
		return fmt.Errorf("that lesson is not part of a scheduled plan")
	}
	if lesson.Status != StatusPlanned {
		return fmt.Errorf("only a planned lesson can be pulled forward")
	}
	asg, err := s.Assignment(lesson.AssignmentID)
	if err != nil {
		return err
	}
	return s.RelayoutPlannedFrom(asg, today(), lesson.Sequence)
}

// RescheduleAssignmentLesson moves one lesson onto the day it was dropped on
// and brings the rest of its plan with it, in whichever direction it went. The
// dragged lesson lands exactly where it was let go, weekend or not, because
// that is what the parent just pointed at; the lessons behind it fall onto the
// school days that follow. It reports whether anything besides the dragged
// lesson moved, which is what tells the planner how much of the week to redraw.
func (s *Store) RescheduleAssignmentLesson(lesson Lesson, date string) (bool, error) {
	if err := s.RescheduleLesson(lesson.ID, date); err != nil {
		return false, err
	}
	return s.CascadeAssignmentAfterMove(lesson, date)
}

// CascadeAssignmentAfterMove reshuffles the rest of a plan once one lesson has
// already been moved onto date (by a drag, or by editing the date and saving).
// lesson must still describe the lesson as it was before the move.
func (s *Store) CascadeAssignmentAfterMove(lesson Lesson, date string) (bool, error) {
	if !lesson.HasAssignment() || lesson.Status != StatusPlanned ||
		lesson.Sequence == 0 || date == lesson.ScheduledOn {
		return false, nil
	}

	siblings, err := s.plannedSiblings(lesson)
	if err != nil {
		return false, err
	}
	follow := lessonsClosingUpBehind(lesson, siblings, date)
	if len(follow) == 0 {
		return false, nil
	}
	asg, err := s.Assignment(lesson.AssignmentID)
	if err != nil {
		return false, err
	}
	if err := s.relayoutLessons(asg, addDays(date, 1), follow); err != nil {
		return false, err
	}
	return true, nil
}

// plannedSiblings is the rest of a lesson's plan that is still to be done.
// Lessons already marked done or skipped are a record of what happened, so
// they are left out of any reshuffle.
func (s *Store) plannedSiblings(lesson Lesson) ([]Lesson, error) {
	all, err := s.LessonsForAssignment(lesson.AssignmentID)
	if err != nil {
		return nil, err
	}
	var out []Lesson
	for _, l := range all {
		if l.ID != lesson.ID && l.Status == StatusPlanned {
			out = append(out, l)
		}
	}
	return out, nil
}

// lessonsClosingUpBehind picks which of a plan's remaining lessons have to
// shuffle once one of them has been dropped on a new day, in the order they
// should be worked through.
//
// Dragged later, it is the lessons numbered after it that follow it down the
// calendar; the ones in front of it were done first and stay where they are.
//
// Dragged earlier she is pulling a lesson forward past work that is still
// pending, and that pending work cannot be left behind: it would double up on
// the days the rest of the plan is about to land on and the set would read out
// of order. So everything still sitting beyond the day she dropped on closes
// up behind it, lowest number first. Whatever is already on that day stays
// there and shares it with the lesson she just let go.
func lessonsClosingUpBehind(dragged Lesson, siblings []Lesson, date string) []Lesson {
	later := date > dragged.ScheduledOn
	var out []Lesson
	for _, l := range siblings {
		if later && l.Sequence > dragged.Sequence {
			out = append(out, l)
		} else if !later && l.ScheduledOn > date {
			out = append(out, l)
		}
	}
	return out
}

func (s *Store) PauseAssignmentUntil(assignmentID int64, resumeOn string) error {
	asg, err := s.Assignment(assignmentID)
	if err != nil {
		return err
	}
	return s.RelayoutPlannedFrom(asg, resumeOn, 0)
}

// BackfillPlanAssignments groups already-applied curriculum lessons that were
// inserted as independent rows (same child, subject, and created_at).
func (s *Store) BackfillPlanAssignments() error {
	rows, err := s.db().Query(`SELECT l.id, l.kid_id, l.subject_id, COALESCE(l.school_year_id, 0),
			l.scheduled_on, l.title, l.notes, l.created_at, s.name
		FROM lessons l
		JOIN subjects s ON s.id = l.subject_id
		WHERE l.assignment_id IS NULL AND l.series_id IS NULL AND l.kid_id IS NOT NULL
		ORDER BY l.kid_id, l.subject_id, l.created_at, l.scheduled_on, l.id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []assignmentBackfillRow
	for rows.Next() {
		var it assignmentBackfillRow
		if err := rows.Scan(&it.id, &it.kidID, &it.subjectID, &it.yearID,
			&it.scheduledOn, &it.title, &it.notes, &it.createdAt, &it.subjectName); err != nil {
			return err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	type bucket struct {
		key   string
		items []assignmentBackfillRow
	}
	var buckets []bucket
	for _, it := range items {
		key := fmt.Sprintf("%d/%d/%s", it.kidID, it.subjectID, it.createdAt)
		if n := len(buckets); n > 0 && buckets[n-1].key == key {
			buckets[n-1].items = append(buckets[n-1].items, it)
			continue
		}
		buckets = append(buckets, bucket{key: key, items: []assignmentBackfillRow{it}})
	}

	plansBySubject, err := s.plansBySubject()
	if err != nil {
		return err
	}

	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	for _, b := range buckets {
		if len(b.items) < 2 {
			continue
		}
		first := b.items[0]
		name := first.subjectName + " sequence"
		var planID int64
		if match, ok := matchingCurriculumPlan(plansBySubject[first.subjectID], b.items); ok {
			name = match.Name
			planID = match.ID
		} else if hint := assignmentNameFromNotes(b.items); hint != "" {
			name = hint
			if p, ok := planNamed(plansBySubject[first.subjectID], hint); ok {
				planID = p.ID
			}
		}
		weekdays := weekdaysFromDates(b.items)
		if weekdays == "" {
			weekdays = defaultWeekdays()
		}

		res, err := tx.Exec(`INSERT INTO plan_assignments
			(kid_id, subject_id, school_year_id, plan_id, name, weekdays, starts_on, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			first.kidID, first.subjectID, nullableID(first.yearID), nullableID(planID),
			name, weekdays, first.scheduledOn, now)
		if err != nil {
			return err
		}
		assignmentID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for i, it := range b.items {
			if _, err := tx.Exec(`UPDATE lessons SET assignment_id = ?, sequence = ? WHERE id = ?`,
				assignmentID, i+1, it.id); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.linkAssignmentsToPlans()
}

func (s *Store) linkAssignmentsToPlans() error {
	_, err := s.db().Exec(`UPDATE plan_assignments
		SET plan_id = (
			SELECT p.id FROM curriculum_plans p
			WHERE p.subject_id = plan_assignments.subject_id
			  AND p.name = plan_assignments.name
			LIMIT 1
		)
		WHERE plan_id IS NULL`)
	return err
}

func (s *Store) plansBySubject() (map[int64][]CurriculumPlan, error) {
	plans, err := s.CurriculumPlans()
	if err != nil {
		return nil, err
	}
	out := map[int64][]CurriculumPlan{}
	for _, p := range plans {
		items, err := s.CurriculumItems(p.ID)
		if err != nil {
			return nil, err
		}
		p.Items = items
		out[p.SubjectID] = append(out[p.SubjectID], p)
	}
	return out, nil
}

type assignmentBackfillRow struct {
	id, kidID, subjectID, yearID                      int64
	scheduledOn, title, notes, createdAt, subjectName string
}

func planNamed(plans []CurriculumPlan, name string) (CurriculumPlan, bool) {
	for _, p := range plans {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return CurriculumPlan{}, false
}

func matchingCurriculumPlan(plans []CurriculumPlan, items []assignmentBackfillRow) (CurriculumPlan, bool) {
	var best CurriculumPlan
	bestExtra := -1
	for _, p := range plans {
		if len(p.Items) < len(items) {
			continue
		}
		ok := true
		for i, it := range items {
			if p.Items[i].Title != it.title {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		extra := len(p.Items) - len(items)
		if extra == 0 {
			return p, true
		}
		if bestExtra < 0 || extra < bestExtra {
			best = p
			bestExtra = extra
		}
	}
	if bestExtra >= 0 {
		return best, true
	}
	return CurriculumPlan{}, false
}

func weekdaysFromDates(items []assignmentBackfillRow) string {
	var days []int
	for _, it := range items {
		t, err := time.Parse(dateLayout, it.scheduledOn)
		if err != nil {
			continue
		}
		days = append(days, isoWeekday(t))
	}
	return formatWeekdays(days)
}

func assignmentNameFromNotes(items []assignmentBackfillRow) string {
	counts := map[string]int{}
	for _, it := range items {
		prefix, _, ok := strings.Cut(strings.TrimSpace(it.notes), ":")
		if !ok {
			continue
		}
		prefix = strings.TrimSpace(prefix)
		if len(prefix) < 4 {
			continue
		}
		counts[prefix]++
	}
	best := ""
	n := 0
	for k, c := range counts {
		if c > n {
			best, n = k, c
		}
	}
	if n*2 >= len(items) {
		return best
	}
	return ""
}
