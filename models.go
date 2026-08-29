package main

import (
	"fmt"
	"time"
)

// Lesson statuses. A lesson is the single record for both "we plan to do this"
// and "we did this", which is what keeps planning and logging in one system.
const (
	StatusPlanned = "planned"
	StatusDone    = "done"
	StatusSkipped = "skipped"
)

// Attachment owner kinds.
const (
	OwnerLesson     = "lesson"
	OwnerAssessment = "assessment"
	OwnerResource   = "resource"
	OwnerCurriculum = "curriculum"
)

const (
	AttendancePresent = "present"
	AttendanceAbsent  = "absent"
	AttendanceExcused = "excused"
)

const (
	PlanAuthored = "authored"
	PlanFromYear = "from_year"
)

type Kid struct {
	ID        int64
	Name      string
	Grade     string
	Color     string
	SortOrder int
	Archived  bool
}

type Subject struct {
	ID        int64
	Name      string
	Slug      string
	SortOrder int
	Archived  bool
}

type SchoolYear struct {
	ID        int64
	Name      string
	StartsOn  string
	EndsOn    string
	IsCurrent bool
}

type Lesson struct {
	ID           int64
	KidID        int64
	SubjectID    int64
	SchoolYearID int64
	SeriesID     int64
	ScheduledOn  string
	Status       string
	Title        string
	Minutes      int
	Notes        string
	CompletedAt  string
	CreatedAt    string

	KidName     string
	KidColor    string
	SubjectName string

	Attachments []Attachment
	Assessments []Assessment
}

func (l Lesson) IsDone() bool    { return l.Status == StatusDone }
func (l Lesson) IsPlanned() bool { return l.Status == StatusPlanned }
func (l Lesson) IsSkipped() bool { return l.Status == StatusSkipped }

// Overdue reports a lesson still planned on a day that has already passed.
func (l Lesson) Overdue() bool {
	return l.Status == StatusPlanned && l.ScheduledOn < today()
}

type Assessment struct {
	ID           int64
	KidID        int64
	SubjectID    int64
	LessonID     int64
	SchoolYearID int64
	GivenOn      string
	Name         string
	Score        *float64
	MaxScore     *float64
	Letter       string
	Notes        string
	CreatedAt    string

	KidName     string
	KidColor    string
	SubjectName string

	Attachments []Attachment
}

// HasPercent reports whether a score out of a maximum was recorded, which is
// what makes a percentage meaningful.
func (a Assessment) HasPercent() bool {
	return a.Score != nil && a.MaxScore != nil && *a.MaxScore > 0
}

func (a Assessment) Percent() int {
	if !a.HasPercent() {
		return 0
	}
	return int((*a.Score / *a.MaxScore * 100) + 0.5)
}

// ScoreLabel renders whatever was recorded: a fraction, a raw score, a letter,
// or nothing at all.
func (a Assessment) ScoreLabel() string {
	switch {
	case a.HasPercent():
		return fmt.Sprintf("%s / %s (%d%%)", trimFloat(*a.Score), trimFloat(*a.MaxScore), a.Percent())
	case a.Score != nil:
		return trimFloat(*a.Score)
	case a.Letter != "":
		return a.Letter
	default:
		return ""
	}
}

type Note struct {
	ID        int64
	KidID     int64
	SubjectID int64
	NotedOn   string
	Body      string
	CreatedAt string

	SubjectName string
}

type Attachment struct {
	ID               int64
	OwnerType        string
	LessonID         int64
	AssessmentID     int64
	KidID            int64
	SubjectID        int64
	CurriculumPlanID int64
	OriginalName     string
	StoredPath       string
	SizeBytes        int64
	ContentType      string
	CreatedAt        string
}

func (a Attachment) SizeLabel() string { return sizeLabel(a.SizeBytes) }

// Progress is the derived view of how a stretch of time went: counts of
// lessons by status plus recorded minutes.
type Progress struct {
	Planned int
	Done    int
	Skipped int
	Minutes int
}

func (p Progress) Total() int { return p.Planned + p.Done + p.Skipped }

// PercentDone measures completion against everything scheduled except skips,
// so skipping a lesson does not permanently dent the number.
func (p Progress) PercentDone() int {
	base := p.Planned + p.Done
	if base == 0 {
		return 0
	}
	return int((float64(p.Done) / float64(base) * 100) + 0.5)
}

func (p Progress) HoursLabel() string {
	if p.Minutes == 0 {
		return "0h"
	}
	h := p.Minutes / 60
	m := p.Minutes % 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

type Attendance struct {
	ID         int64
	KidID      int64
	AttendedOn string
	Status     string
	Notes      string
	CreatedAt  string

	KidName  string
	KidColor string
}

func (a Attendance) IsPresent() bool { return a.Status == AttendancePresent }
func (a Attendance) IsAbsent() bool  { return a.Status == AttendanceAbsent }
func (a Attendance) IsExcused() bool { return a.Status == AttendanceExcused }
func (a Attendance) Marked() bool    { return a.Status != "" }

func (a Attendance) Label() string {
	switch a.Status {
	case AttendancePresent:
		return "Present"
	case AttendanceAbsent:
		return "Absent"
	case AttendanceExcused:
		return "Excused"
	default:
		return "Not marked"
	}
}

type AttendanceTotals struct {
	Present  int
	Absent   int
	Excused  int
	Unmarked int
}

func (t AttendanceTotals) Recorded() int {
	return t.Present + t.Absent + t.Excused
}

type CurriculumPlan struct {
	ID           int64
	Name         string
	SubjectID    int64
	Kind         string
	SourceKidID  int64
	SourceYearID int64
	Notes        string
	CreatedAt    string

	SubjectName string
	ItemCount   int
	Items       []CurriculumItem
	Attachments []Attachment
}

func (p CurriculumPlan) KindLabel() string {
	if p.Kind == PlanFromYear {
		return "Saved from a year"
	}
	return "Written"
}

func (p CurriculumPlan) FromYear() bool {
	return p.Kind == PlanFromYear
}

type CurriculumItem struct {
	ID         int64
	PlanID     int64
	SortOrder  int
	Title      string
	Notes      string
	Minutes    int
	WeekNumber int
	CreatedAt  string
}

type LessonSeries struct {
	ID              int64
	KidID           int64
	SubjectID       int64
	SchoolYearID    int64
	Title           string
	Minutes         int
	Notes           string
	Weekdays        string
	StartsOn        string
	EndsOn          string
	OccurrenceCount int
	CreatedAt       string

	KidName     string
	KidColor    string
	SubjectName string
}

func today() string {
	return time.Now().Format(dateLayout)
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
