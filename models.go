package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
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
	ID         int64
	Name       string
	Grade      string
	Color      string
	AvatarPath string
	SortOrder  int
	Archived   bool
}

// HasPhoto reports whether this child has a photo to show instead of the
// fallback color dot.
func (k Kid) HasPhoto() bool { return k.AvatarPath != "" }

// AvatarURL is where the photo is served from, cache-busted by the stored
// path so replacing a photo shows up straight away.
func (k Kid) AvatarURL() string {
	return "/avatars/kids/" + fmt.Sprint(k.ID) + "?v=" + shortHash(k.AvatarPath)
}

// Adult is a grown-up in the family: a schedule, notes, and a pinboard, but no
// grades, attendance, or curriculum. V1 expects exactly one.
type Adult struct {
	ID         int64
	Name       string
	Role       string
	Color      string
	AvatarPath string
	SortOrder  int
	Archived   bool
}

func (a Adult) HasPhoto() bool { return a.AvatarPath != "" }

func (a Adult) URL() string { return "/adults/" + fmt.Sprint(a.ID) }

func (a Adult) AvatarURL() string {
	return "/avatars/adults/" + fmt.Sprint(a.ID) + "?v=" + shortHash(a.AvatarPath)
}

// AdultCard is one note on the pinboard: something worth keeping in view that
// is not pinned to a particular day.
type AdultCard struct {
	ID        int64
	AdultID   int64
	Title     string
	Body      string
	Pinned    bool
	SortOrder int
	CreatedAt string
	UpdatedAt string
}

// AdultEventLabel is a color tag she keeps on her own calendar so appointments
// of a kind can be told apart at a glance. An optional emoji rides along for
// the same reason a cake says "birthday" faster than the word does.
type AdultEventLabel struct {
	ID        int64
	AdultID   int64
	Name      string
	Color     string
	Emoji     string
	SortOrder int
	CreatedAt string
}

func (l AdultEventLabel) HasEmoji() bool { return strings.TrimSpace(l.Emoji) != "" }

// AdultEvent is something on her own calendar: an appointment, a trip, a week
// away. Both dates are inclusive, so a one-day event has the same date twice.
type AdultEvent struct {
	ID        int64
	AdultID   int64
	LabelID   int64
	StartsOn  string
	EndsOn    string
	Title     string
	Body      string
	CreatedAt string

	LabelName  string
	LabelColor string
	LabelEmoji string
}

func (e AdultEvent) Spans() bool { return e.EndsOn > e.StartsOn }

func (e AdultEvent) HasLabel() bool {
	return e.LabelID != 0 && hexColor.MatchString(e.LabelColor)
}

func (e AdultEvent) Icon() string {
	return strings.TrimSpace(e.LabelEmoji)
}

// Covers reports whether a day falls inside the event, which is what puts a
// mark on every cell of a run rather than only the day it started.
func (e AdultEvent) Covers(date string) bool {
	return date >= e.StartsOn && date <= e.EndsOn
}

// DateLabelOn names the stretch the way she would say it out loud, relative to
// the reader's own today so "Tomorrow" means theirs.
func (e AdultEvent) DateLabelOn(today string) string {
	if !e.Spans() {
		return prettyDateOn(e.StartsOn, today)
	}
	return prettyDateOn(e.StartsOn, today) + " - " + prettyDateOn(e.EndsOn, today)
}

// Holiday is a day the family keeps that nobody has to enter: the classic US
// holidays, worked out for whichever year is on screen rather than stored.
// Emoji is the default icon; an adult can override it (and add notes or a
// color label) without changing the holiday for anyone else.
type Holiday struct {
	Date  string
	Name  string
	Emoji string

	// Filled in when the calendar is drawn for one adult.
	Notes          string
	OverrideEmoji  string
	LabelID        int64
	LabelName      string
	LabelColor     string
	LabelEmoji     string
}

func (h Holiday) Icon() string {
	if e := strings.TrimSpace(h.OverrideEmoji); e != "" {
		return e
	}
	return strings.TrimSpace(h.Emoji)
}

func (h Holiday) HasLabel() bool {
	return h.LabelID != 0 && hexColor.MatchString(h.LabelColor)
}

// AdultHolidayNote is one adult's personalization of a computed holiday.
type AdultHolidayNote struct {
	ID          int64
	AdultID     int64
	ObservedOn  string
	HolidayName string
	Emoji       string
	Notes       string
	LabelID     int64
}

type Subject struct {
	ID        int64
	Name      string
	Slug      string
	Color     string
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
	AdultID      int64
	SubjectID    int64
	SchoolYearID int64
	SeriesID     int64
	AssignmentID int64
	Sequence     int
	ScheduledOn  string
	Status       string
	Title        string
	Minutes      int
	Notes        string
	CompletedAt  string
	CreatedAt    string

	// A lesson belongs to exactly one person: a child, or an adult with a
	// schedule of her own. These carry whichever it is.
	PersonName   string
	PersonColor  string
	PersonAvatar string
	SubjectName  string
	SubjectColor string

	Attachments []Attachment
	Assessments []Assessment
}

func (l Lesson) ForAdult() bool { return l.AdultID != 0 }

func (l Lesson) PersonHasPhoto() bool { return l.PersonAvatar != "" }

func (l Lesson) PersonURL() string {
	if l.ForAdult() {
		return "/adults/" + fmt.Sprint(l.AdultID)
	}
	return "/kids/" + fmt.Sprint(l.KidID)
}

func (l Lesson) PersonAvatarURL() string {
	if l.ForAdult() {
		return "/avatars/adults/" + fmt.Sprint(l.AdultID) + "?v=" + shortHash(l.PersonAvatar)
	}
	return "/avatars/kids/" + fmt.Sprint(l.KidID) + "?v=" + shortHash(l.PersonAvatar)
}

func (l Lesson) IsDone() bool        { return l.Status == StatusDone }
func (l Lesson) IsPlanned() bool     { return l.Status == StatusPlanned }
func (l Lesson) IsSkipped() bool     { return l.Status == StatusSkipped }
func (l Lesson) HasAssignment() bool { return l.AssignmentID != 0 }
func (l Lesson) HasSeries() bool     { return l.SeriesID != 0 }

// OverdueOn reports a lesson still planned on a day that has already passed.
// The day is passed in rather than read from the clock because "already
// passed" depends on which timezone the person reading is in.
func (l Lesson) OverdueOn(today string) bool {
	return l.Status == StatusPlanned && l.ScheduledOn < today
}

// AheadOf reports a lesson sitting on a later day, which is the only kind
// there is anything to gain by pulling forward.
func (l Lesson) AheadOf(today string) bool { return l.ScheduledOn > today }

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
	AdultID   int64
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

// PlanAssignment is one child's scheduled copy of a curriculum sequence.
type PlanAssignment struct {
	ID           int64
	KidID        int64
	SubjectID    int64
	SchoolYearID int64
	PlanID       int64
	Name         string
	Weekdays     string
	StartsOn     string
	CreatedAt    string

	KidName     string
	KidColor    string
	SubjectName string
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

// shortHash turns a stored path into a stable cache-busting token, so a
// replaced photo is not hidden behind the browser's copy of the old one.
func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:4])
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}
