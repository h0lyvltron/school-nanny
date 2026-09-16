package main

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func recoveryCurriculum(t *testing.T, ta *testApp) (CurriculumPlan, []CurriculumItem) {
	t.Helper()
	planID, err := ta.store.CreateCurriculumPlan(CurriculumPlan{
		Name: "Recoverable Math", SubjectID: ta.mathSubjectID(), Kind: PlanAuthored,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []CurriculumItem{
		{PlanID: planID, Title: "Lesson one", Minutes: 20, Notes: "Read together", PageStart: 9, PageEnd: 10},
		{PlanID: planID, Title: "Lesson two", Minutes: 30, Notes: "Use counters", PageStart: 11, PageEnd: 13},
	} {
		if _, err := ta.store.CreateCurriculumItem(item); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := ta.store.CurriculumPlan(planID)
	if err != nil {
		t.Fatal(err)
	}
	return plan, plan.Items
}

func TestScheduleOneCurriculumItemCopiesSourceAndRejectsDuplicate(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	_, items := recoveryCurriculum(t, ta)
	date := addDays(today(), 3)

	id, err := ta.store.ScheduleCurriculumItem(kid, items[1].ID, date)
	if err != nil {
		t.Fatal(err)
	}
	lesson, err := ta.store.Lesson(id)
	if err != nil {
		t.Fatal(err)
	}
	if lesson.CurriculumItemID != items[1].ID || lesson.AssignmentID != 0 ||
		lesson.Title != "Lesson two" || lesson.Minutes != 30 ||
		lesson.Notes != "Use counters" || lesson.PageStart != 11 || lesson.PageEnd != 13 {
		t.Fatalf("scheduled lesson: %+v", lesson)
	}
	if _, err := ta.store.ScheduleCurriculumItem(kid, items[1].ID, date); !errors.Is(err, errCurriculumLessonExists) {
		t.Fatalf("duplicate err=%v", err)
	}
}

func TestScheduleMissingCurriculumItemRejoinsAssignment(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	plan, items := recoveryCurriculum(t, ta)
	dates := []string{addDays(today(), 7), addDays(today(), 8)}
	if err := ta.store.ApplyCurriculum(plan, kid, dates, "1,2,3,4,5"); err != nil {
		t.Fatal(err)
	}
	lessons, err := ta.store.LessonsBetween(dates[0], dates[1], kid)
	if err != nil || len(lessons) != 2 {
		t.Fatalf("lessons=%d err=%v", len(lessons), err)
	}
	if lessons[0].CurriculumItemID != items[0].ID || lessons[1].CurriculumItemID != items[1].ID {
		t.Fatalf("applied curriculum provenance: %+v", lessons)
	}
	assignmentID := lessons[0].AssignmentID
	if _, err := ta.store.TrashLesson(lessons[1].ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	restoredID, err := ta.store.ScheduleCurriculumItem(kid, items[1].ID, dates[1])
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ta.store.Lesson(restoredID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.AssignmentID != assignmentID || restored.Sequence != items[1].SortOrder {
		t.Fatalf("restored assignment linkage: %+v", restored)
	}
}

func TestCurriculumSchedulePageAndPost(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	plan, items := recoveryCurriculum(t, ta)
	date := addDays(today(), 4)

	status, page := ta.get("/planner?week=" + weekStart(parseDate(date)).Format(dateLayout))
	if status != 200 || !strings.Contains(page, "Add from curriculum") {
		t.Fatalf("planner status=%d missing curriculum action", status)
	}
	status, page = ta.get("/curriculum/schedule-item?date=" + date)
	if status != 200 || !strings.Contains(page, "Recoverable Math") ||
		!strings.Contains(page, `id="curriculum-plan"`) ||
		!strings.Contains(page, `data-plan-id="`+itoa64(plan.ID)+`"`) ||
		!strings.Contains(page, "Lesson two") {
		t.Fatalf("schedule page status=%d body=%q", status, page)
	}
	ta.redirectAfterPost("/curriculum/schedule-item", mapValues(
		"kid_id", itoa64(kid),
		"plan_id", itoa64(plan.ID),
		"item_id", itoa64(items[1].ID),
		"scheduled_on", date,
	))
	lessons, err := ta.store.LessonsBetween(date, date, kid)
	if err != nil || len(lessons) != 1 || lessons[0].CurriculumItemID != items[1].ID {
		t.Fatalf("scheduled lessons=%+v err=%v", lessons, err)
	}
}

func TestStandaloneCurriculumLessonUsesPlanPDF(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	plan, items := recoveryCurriculum(t, ta)
	if _, err := ta.store.CreateAttachment(Attachment{
		OwnerType: OwnerCurriculum, CurriculumPlanID: plan.ID,
		OriginalName: "course.pdf", StoredPath: "course.pdf",
		SizeBytes: 100, ContentType: "application/pdf",
	}); err != nil {
		t.Fatal(err)
	}
	lessonID, err := ta.store.ScheduleCurriculumItem(kid, items[0].ID, today())
	if err != nil {
		t.Fatal(err)
	}
	status, page := ta.get("/lessons/" + itoa64(lessonID))
	if status != http.StatusOK || !strings.Contains(page, "data-pdf-viewer") {
		t.Fatalf("lesson page status=%d missing inherited PDF", status)
	}
}

func TestPlannerHTMXDeleteOffersUndoAndRestore(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	lessonID := ta.insertUnassignedLesson(kid, ta.mathSubjectID(), today(), "Undo me", "")
	back := plannerURL(weekStart(parseDate(today())).Format(dateLayout), kid)
	form := url.Values{
		"view":       {"planner"},
		"kid_filter": {itoa64(kid)},
		"back":       {back},
	}
	req, err := http.NewRequest(http.MethodPost,
		ta.server.URL+"/lessons/"+itoa64(lessonID)+"/delete",
		strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := ta.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Undo") ||
		!strings.Contains(string(body), `/lessons/restore`) {
		t.Fatalf("delete response status=%d body=%q", resp.StatusCode, body)
	}
	deleted, err := ta.store.DeletedLessons(time.Now())
	if err != nil || len(deleted) != 1 {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	status, planner := ta.get(back)
	if status != http.StatusOK || !strings.Contains(planner, `href="/trash"`) {
		t.Fatalf("planner missing trash link status=%d", status)
	}
	status, trash := ta.get("/trash")
	if status != http.StatusOK || !strings.Contains(trash, "Recently deleted") ||
		!strings.Contains(trash, "Undo me") ||
		!strings.Contains(trash, `name="back" value="/trash"`) {
		t.Fatalf("trash page status=%d body=%q", status, trash)
	}

	restoreForm := url.Values{
		"token":      {deleted[0].Token},
		"view":       {"planner"},
		"kid_filter": {itoa64(kid)},
	}
	req, err = http.NewRequest(http.MethodPost, ta.server.URL+"/lessons/restore",
		strings.NewReader(restoreForm.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err = ta.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Undo me") {
		t.Fatalf("restore response status=%d body=%q", resp.StatusCode, body)
	}
	if _, err := ta.store.Lesson(lessonID); err != nil {
		t.Fatalf("restored lesson: %v", err)
	}
}

func TestTrashAndRestoreLessonPreservesFilesAndAssessmentLinks(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	lessonID, err := ta.store.CreateLesson(Lesson{
		KidID: kid, SubjectID: subject, ScheduledOn: today(), Status: StatusPlanned,
		Title: "Fractions", Notes: "Keep me", PageStart: 4, PageEnd: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	assessmentID, err := ta.store.CreateAssessment(Assessment{
		KidID: kid, SubjectID: subject, LessonID: lessonID,
		GivenOn: today(), Name: "Fractions check",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join("2026", "09", "lesson-work.pdf")
	fullPath := filepath.Join(ta.uploadDir, stored)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("%PDF lesson work"), 0o644); err != nil {
		t.Fatal(err)
	}
	attachmentID, err := ta.store.CreateAttachment(Attachment{
		OwnerType: OwnerLesson, LessonID: lessonID, OriginalName: "lesson-work.pdf",
		StoredPath: filepath.ToSlash(stored), SizeBytes: 16, ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := ta.store.TrashLesson(lessonID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.Lesson(lessonID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted lesson err=%v", err)
	}
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("attachment file removed before expiry: %v", err)
	}
	var linkedLesson int64
	if err := ta.store.db().QueryRow(`SELECT COALESCE(lesson_id, 0) FROM assessments WHERE id = ?`,
		assessmentID).Scan(&linkedLesson); err != nil || linkedLesson != 0 {
		t.Fatalf("assessment link=%d err=%v", linkedLesson, err)
	}

	restored, err := ta.store.RestoreLesson(deleted.Token, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != lessonID || restored.PageStart != 4 || restored.PageEnd != 6 {
		t.Fatalf("restored lesson: %+v", restored)
	}
	files, err := ta.store.AttachmentsForLesson(lessonID)
	if err != nil || len(files) != 1 || files[0].ID != attachmentID {
		t.Fatalf("restored files=%+v err=%v", files, err)
	}
	if err := ta.store.db().QueryRow(`SELECT COALESCE(lesson_id, 0) FROM assessments WHERE id = ?`,
		assessmentID).Scan(&linkedLesson); err != nil || linkedLesson != lessonID {
		t.Fatalf("restored assessment link=%d err=%v", linkedLesson, err)
	}
}

func TestExpiredLessonTrashPurgesRetainedFiles(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	lessonID, err := ta.store.CreateLesson(Lesson{
		KidID: kid, SubjectID: ta.mathSubjectID(), ScheduledOn: today(),
		Status: StatusPlanned, Title: "Old lesson",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join("old", "work.txt")
	fullPath := filepath.Join(ta.uploadDir, stored)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.CreateAttachment(Attachment{
		OwnerType: OwnerLesson, LessonID: lessonID, OriginalName: "work.txt",
		StoredPath: filepath.ToSlash(stored), SizeBytes: 3, ContentType: "text/plain",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.TrashLesson(lessonID, time.Now().Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := ta.purgeExpiredLessonTrash(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fullPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired file still exists: %v", err)
	}
	deleted, err := ta.store.DeletedLessons(time.Now())
	if err != nil || len(deleted) != 0 {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
}

func mapValues(values ...string) url.Values {
	out := url.Values{}
	for i := 0; i+1 < len(values); i += 2 {
		out[values[i]] = []string{values[i+1]}
	}
	return out
}
