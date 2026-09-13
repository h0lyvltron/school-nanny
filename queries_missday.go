package main

import (
	"fmt"
	"time"
)

// lessonWeekdays returns the school-day mask for shifting a lesson: the
// assignment's weekdays when it is part of a plan, otherwise Mon–Fri.
func (s *Store) lessonWeekdays(lesson Lesson) (string, error) {
	if lesson.AssignmentID == 0 {
		return defaultWeekdays(), nil
	}
	asg, err := s.Assignment(lesson.AssignmentID)
	if err != nil {
		return "", err
	}
	return asg.Weekdays, nil
}

// DoubleUpLesson piles one planned lesson onto the next school day without
// moving anything else — intentional stacking when life ate the day.
func (s *Store) DoubleUpLesson(lesson Lesson, today string) error {
	if lesson.Status != StatusPlanned {
		return fmt.Errorf("only a planned lesson can be doubled up")
	}
	weekdays, err := s.lessonWeekdays(lesson)
	if err != nil {
		return err
	}
	if today == "" {
		today = time.Now().Format(dateLayout)
	}
	base := maxDate(today, lesson.ScheduledOn)
	next := nextMatchingWeekdayOnOrAfter(addDays(base, 1), parseWeekdays(weekdays))
	return s.RescheduleLesson(lesson.ID, next)
}

// ShiftLessonForward moves this planned lesson one school day later.
// Assignment siblings are left alone.
func (s *Store) ShiftLessonForward(lesson Lesson) error {
	if lesson.Status != StatusPlanned {
		return fmt.Errorf("only a planned lesson can be shifted")
	}
	weekdays, err := s.lessonWeekdays(lesson)
	if err != nil {
		return err
	}
	next := nextMatchingWeekdayOnOrAfter(addDays(lesson.ScheduledOn, 1), parseWeekdays(weekdays))
	return s.RescheduleLesson(lesson.ID, next)
}

// ShiftAssignmentForward moves this planned lesson and every later planned
// lesson in its plan one school day later, keeping their order.
func (s *Store) ShiftAssignmentForward(lesson Lesson) error {
	if lesson.AssignmentID == 0 {
		return fmt.Errorf("that lesson is not part of a scheduled plan")
	}
	if lesson.Status != StatusPlanned {
		return fmt.Errorf("only a planned lesson can be shifted")
	}
	asg, err := s.Assignment(lesson.AssignmentID)
	if err != nil {
		return err
	}
	resume := nextMatchingWeekdayOnOrAfter(addDays(lesson.ScheduledOn, 1), parseWeekdays(asg.Weekdays))
	return s.RelayoutPlannedFrom(asg, resume, lesson.Sequence)
}

// RelayoutPlannedOnOrAfter moves planned lessons on or after fromDate onto
// consecutive school days beginning at resumeOn.
func (s *Store) RelayoutPlannedOnOrAfter(asg PlanAssignment, fromDate, resumeOn string) error {
	rows, err := s.db().Query(lessonSelect+`
		WHERE l.assignment_id = ? AND l.status = ? AND l.scheduled_on >= ?
		ORDER BY l.sequence, l.id`, asg.ID, StatusPlanned, fromDate)
	if err != nil {
		return err
	}
	planned, err := scanLessons(rows)
	if err != nil {
		return err
	}
	return s.relayoutLessons(asg, resumeOn, planned)
}

// VacationShiftAssignment treats [from,to] as no-school and lays planned work
// that had been on or after from onto school days after to.
func (s *Store) VacationShiftAssignment(assignmentID int64, from, to string) error {
	if from == "" || to == "" {
		return fmt.Errorf("vacation needs a start and end date")
	}
	if to < from {
		return fmt.Errorf("vacation end must be on or after the start")
	}
	asg, err := s.Assignment(assignmentID)
	if err != nil {
		return err
	}
	resume := nextMatchingWeekdayOnOrAfter(addDays(to, 1), parseWeekdays(asg.Weekdays))
	return s.RelayoutPlannedOnOrAfter(asg, from, resume)
}
