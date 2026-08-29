package main

import (
	"database/sql"
	"time"
)

const attendanceSelect = `SELECT a.id, a.kid_id, a.attended_on, a.status, a.notes, a.created_at,
		k.name, k.color
	FROM attendance a
	JOIN kids k ON k.id = a.kid_id`

func scanAttendance(rows *sql.Rows) ([]Attendance, error) {
	defer rows.Close()
	var out []Attendance
	for rows.Next() {
		var a Attendance
		if err := rows.Scan(&a.ID, &a.KidID, &a.AttendedOn, &a.Status, &a.Notes, &a.CreatedAt,
			&a.KidName, &a.KidColor); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AttendanceOnDate(date string) (map[int64]Attendance, error) {
	rows, err := s.db().Query(attendanceSelect+` WHERE a.attended_on = ?`, date)
	if err != nil {
		return nil, err
	}
	list, err := scanAttendance(rows)
	if err != nil {
		return nil, err
	}
	out := map[int64]Attendance{}
	for _, a := range list {
		out[a.KidID] = a
	}
	return out, nil
}

func (s *Store) AttendanceBetween(from, to string, kidID int64) ([]Attendance, error) {
	q := attendanceSelect + ` WHERE a.attended_on BETWEEN ? AND ?`
	args := []any{from, to}
	if kidID > 0 {
		q += ` AND a.kid_id = ?`
		args = append(args, kidID)
	}
	q += ` ORDER BY a.attended_on, k.sort_order`
	rows, err := s.db().Query(q, args...)
	if err != nil {
		return nil, err
	}
	return scanAttendance(rows)
}

func (s *Store) AttendanceForKidOn(kidID int64, date string) (Attendance, error) {
	rows, err := s.db().Query(attendanceSelect+` WHERE a.kid_id = ? AND a.attended_on = ?`, kidID, date)
	if err != nil {
		return Attendance{}, err
	}
	list, err := scanAttendance(rows)
	if err != nil {
		return Attendance{}, err
	}
	if len(list) == 0 {
		return Attendance{KidID: kidID, AttendedOn: date}, nil
	}
	return list[0], nil
}

func (s *Store) UpsertAttendance(kidID int64, date, status, notes string) error {
	_, err := s.db().Exec(`INSERT INTO attendance (kid_id, attended_on, status, notes, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(kid_id, attended_on) DO UPDATE SET
			status = excluded.status,
			notes = excluded.notes`,
		kidID, date, status, notes, time.Now().Format(time.RFC3339))
	return err
}

func (s *Store) DeleteAttendance(kidID int64, date string) error {
	_, err := s.db().Exec(`DELETE FROM attendance WHERE kid_id = ? AND attended_on = ?`, kidID, date)
	return err
}

// AttendanceTotalsBetween counts marked days and, for unmarked, the Mon–Fri
// dates in the range that have no row at all.
func (s *Store) AttendanceTotalsBetween(from, to string, kidID int64) (AttendanceTotals, error) {
	var t AttendanceTotals
	if from == "" || to == "" || to < from {
		return t, nil
	}

	q := `SELECT
			COALESCE(SUM(a.status = 'present'), 0),
			COALESCE(SUM(a.status = 'absent'), 0),
			COALESCE(SUM(a.status = 'excused'), 0),
			COALESCE(SUM(CASE WHEN strftime('%w', a.attended_on) NOT IN ('0', '6') THEN 1 ELSE 0 END), 0)
		FROM attendance a
		JOIN kids k ON k.id = a.kid_id
		WHERE a.attended_on BETWEEN ? AND ?`
	args := []any{from, to}
	if kidID > 0 {
		q += ` AND a.kid_id = ?`
		args = append(args, kidID)
	} else {
		q += ` AND k.archived = 0`
	}

	var weekdayMarked int
	if err := s.db().QueryRow(q, args...).Scan(&t.Present, &t.Absent, &t.Excused, &weekdayMarked); err != nil {
		return t, err
	}

	weekdays := weekdayCountMonFri(from, to)
	if kidID == 0 {
		kids, err := s.Kids(false)
		if err != nil {
			return t, err
		}
		n := len(kids)
		if n == 0 {
			t.Unmarked = 0
			return t, nil
		}
		t.Unmarked = weekdays*n - weekdayMarked
	} else {
		t.Unmarked = weekdays - weekdayMarked
	}
	if t.Unmarked < 0 {
		t.Unmarked = 0
	}
	return t, nil
}
