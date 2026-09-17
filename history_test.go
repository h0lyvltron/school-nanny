package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func historyAction(t *testing.T, s *Store, label string, fn func()) int64 {
	t.Helper()
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	id, err := s.beginHistory(label, "test")
	if err != nil {
		t.Fatal(err)
	}
	fn()
	if _, _, err := s.finishHistory(id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestHistoryUndoRedoAndBranchCheckout(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	var firstLesson int64
	firstNode := historyAction(t, ta.store, "Add first lesson", func() {
		var err error
		firstLesson, err = ta.store.CreateLesson(Lesson{
			KidID: kid, SubjectID: subject, ScheduledOn: today(),
			Status: StatusPlanned, Title: "First",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	pos, err := ta.store.HistoryPosition()
	if err != nil || pos.CurrentID != firstNode || !pos.CanUndo {
		t.Fatalf("position after first: %+v err=%v", pos, err)
	}
	if err := ta.store.Undo(firstNode); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.Lesson(firstLesson); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("first lesson survived undo: %v", err)
	}
	if err := ta.store.Redo(0); err != nil {
		t.Fatal(err)
	}
	if got, err := ta.store.Lesson(firstLesson); err != nil || got.Title != "First" {
		t.Fatalf("redo lesson=%+v err=%v", got, err)
	}

	if err := ta.store.Undo(firstNode); err != nil {
		t.Fatal(err)
	}
	var branchLesson int64
	branchNode := historyAction(t, ta.store, "Add branch lesson", func() {
		var err error
		branchLesson, err = ta.store.CreateLesson(Lesson{
			KidID: kid, SubjectID: subject, ScheduledOn: today(),
			Status: StatusPlanned, Title: "Branch",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	if firstNode == branchNode {
		t.Fatal("branch reused node id")
	}
	if err := ta.store.Checkout(branchNode, firstNode); err != nil {
		t.Fatal(err)
	}
	if got, err := ta.store.Lesson(branchLesson); err == nil && got.Title == "Branch" {
		t.Fatalf("branch lesson survived checkout: %+v", got)
	}
	if got, err := ta.store.Lesson(firstLesson); err != nil || got.Title != "First" {
		t.Fatalf("first branch not restored: %+v err=%v", got, err)
	}
	if err := ta.store.Undo(firstNode); err != nil {
		t.Fatal(err)
	}
	if err := ta.store.Redo(0); err != nil {
		t.Fatal(err)
	}
	if got, err := ta.store.Lesson(firstLesson); err != nil || got.Title != "First" {
		t.Fatalf("redo did not follow last-visited branch: %+v err=%v", got, err)
	}
}

func TestHistoryBulkActionIsOneNode(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	historyAction(t, ta.store, "Schedule a week", func() {
		for i := 0; i < 5; i++ {
			if _, err := ta.store.CreateLesson(Lesson{
				KidID: kid, SubjectID: subject, ScheduledOn: addDays(today(), i),
				Status: StatusPlanned, Title: "Week",
			}); err != nil {
				t.Fatal(err)
			}
		}
	})
	var nodes, changes int
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM history_nodes`).Scan(&nodes); err != nil {
		t.Fatal(err)
	}
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM history_changes`).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if nodes != 1 || changes != 5 {
		t.Fatalf("nodes=%d changes=%d", nodes, changes)
	}
}

func TestHistoryRouteAndKeyboardUI(t *testing.T) {
	ta := newTestApp(t)
	status, body := ta.get("/history")
	if status != http.StatusOK || !strings.Contains(body, "History") {
		t.Fatalf("history page status=%d body=%q", status, body)
	}
	resp, err := ta.client.Get(ta.server.URL + "/trash")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Request.URL.Path != "/history" {
		t.Fatalf("trash ended at %s", resp.Request.URL.Path)
	}
	raw, err := os.ReadFile("static/history.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `key === "z"`) || !strings.Contains(string(raw), `key === "y"`) {
		t.Fatal("history keyboard shortcuts missing")
	}
}

func TestHistoryMiddlewareCapturesPlannerButNotSettings(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	ta.post("/lessons", url.Values{
		"kid_id": {itoa64(kid)}, "subject_id": {itoa64(ta.mathSubjectID())},
		"scheduled_on": {today()}, "title": {"Captured"}, "status": {"planned"},
	})
	var count int
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM history_nodes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("planner nodes=%d", count)
	}
	ta.post("/settings/timezone", url.Values{"timezone": {"UTC"}})
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM history_nodes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("settings entered history: nodes=%d", count)
	}
}

func TestHistoryRetainsAttachmentBytesAcrossUndo(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	path := filepath.Join("2026", "09", "kept.pdf")
	full := filepath.Join(ta.uploadDir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("%PDF kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	var attachmentID int64
	node := historyAction(t, ta.store, "Upload attachment", func() {
		var err error
		attachmentID, err = ta.store.CreateAttachment(Attachment{
			OwnerType: OwnerResource, KidID: kid, SubjectID: subject,
			OriginalName: "kept.pdf", StoredPath: filepath.ToSlash(path),
			SizeBytes: 9, ContentType: "application/pdf",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	if err := ta.store.Undo(node); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.Attachment(attachmentID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("attachment survived undo: %v", err)
	}
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("history file removed: %v", err)
	}
	if err := ta.store.Redo(0); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.Attachment(attachmentID); err != nil {
		t.Fatalf("attachment redo: %v", err)
	}
}

func TestHistoryLessonDeleteRestoresRelationshipsAndFile(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()
	lesson := ta.insertUnassignedLesson(kid, subject, today(), "Keep links", "")
	assessment, err := ta.store.CreateAssessment(Assessment{
		KidID: kid, SubjectID: subject, LessonID: lesson,
		GivenOn: today(), Name: "Check",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("2026", "09", "lesson.pdf")
	full := filepath.Join(ta.uploadDir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	attachment, err := ta.store.CreateAttachment(Attachment{
		OwnerType: OwnerLesson, LessonID: lesson, OriginalName: "lesson.pdf",
		StoredPath: filepath.ToSlash(path), SizeBytes: 4, ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	ta.redirectAfterPost("/lessons/"+itoa64(lesson)+"/delete", url.Values{"back": {"/planner"}})
	pos, err := ta.store.HistoryPosition()
	if err != nil || !pos.CanUndo {
		t.Fatalf("position=%+v err=%v", pos, err)
	}
	if err := ta.store.Undo(pos.CurrentID); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.Lesson(lesson); err != nil {
		t.Fatalf("lesson restore: %v", err)
	}
	if _, err := ta.store.Attachment(attachment); err != nil {
		t.Fatalf("attachment restore: %v", err)
	}
	got, err := ta.store.Assessment(assessment)
	if err != nil || got.LessonID != lesson {
		t.Fatalf("assessment=%+v err=%v", got, err)
	}
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("file restore: %v", err)
	}
}

func TestHistoryRejectsStaleCursor(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	node := historyAction(t, ta.store, "Add note", func() {
		if _, err := ta.store.CreateNote(Note{KidID: kid, NotedOn: today(), Body: "one"}); err != nil {
			t.Fatal(err)
		}
	})
	if err := ta.store.Undo(node + 99); !errors.Is(err, errHistoryConflict) {
		t.Fatalf("stale undo err=%v", err)
	}
}

func TestHistoryRecoversCommittedPendingAction(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	node, err := ta.store.beginHistory("Interrupted note", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ta.store.CreateNote(Note{KidID: kid, NotedOn: today(), Body: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := ta.store.RecoverHistory(); err != nil {
		t.Fatal(err)
	}
	pos, err := ta.store.HistoryPosition()
	if err != nil || pos.CurrentID != node {
		t.Fatalf("position=%+v err=%v", pos, err)
	}
}

func TestHistoryPrunesTo128StepHorizon(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	historyAction(t, ta.store, "Obsolete root", func() {
		if _, err := ta.store.CreateNote(Note{KidID: kid, NotedOn: today(), Body: "old branch"}); err != nil {
			t.Fatal(err)
		}
	})
	oldRoot, err := ta.store.HistoryPosition()
	if err != nil {
		t.Fatal(err)
	}
	if err := ta.store.Undo(oldRoot.CurrentID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < historyLimit+2; i++ {
		historyAction(t, ta.store, "Add note", func() {
			if _, err := ta.store.CreateNote(Note{
				KidID: kid, NotedOn: today(), Body: "step",
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
	var count int
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM history_nodes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != historyLimit {
		t.Fatalf("history nodes=%d want %d", count, historyLimit)
	}
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM history_nodes WHERE id=?`,
		oldRoot.CurrentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("obsolete sibling root survived pruning")
	}
}

func TestHistoryRegistryMatchesTrackedTables(t *testing.T) {
	ta := newTestApp(t)
	for table, spec := range historyTables {
		rows, err := ta.store.db().Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			got = append(got, name)
		}
		rows.Close()
		if strings.Join(got, ",") != strings.Join(spec.columns, ",") {
			t.Errorf("%s columns=%v registry=%v", table, got, spec.columns)
		}
	}
	var triggerCount int
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='trigger' AND name LIKE 'history_%'`).Scan(&triggerCount); err != nil {
		t.Fatal(err)
	}
	if triggerCount != len(historyTables)*3 {
		t.Fatalf("history triggers=%d want %d", triggerCount, len(historyTables)*3)
	}
}

func TestSeriesRematerializationKeepsCurrentSchoolYear(t *testing.T) {
	ta := newTestApp(t)
	if _, err := ta.store.CreateSchoolYear("2026-27", "2026-08-01", "2027-06-01", true); err != nil {
		t.Fatal(err)
	}
	year, err := ta.store.CurrentSchoolYear()
	if err != nil {
		t.Fatal(err)
	}
	sr := LessonSeries{
		KidID: ta.addKid("Mia"), SubjectID: ta.mathSubjectID(), Title: "Series",
		Weekdays: "1,3,5", StartsOn: today(), OccurrenceCount: 3,
	}
	dates, err := occurrenceDates(sr.StartsOn, "", sr.OccurrenceCount, parseWeekdays(sr.Weekdays))
	if err != nil {
		t.Fatal(err)
	}
	id, err := ta.store.CreateSeries(sr, dates)
	if err != nil {
		t.Fatal(err)
	}
	sr.Weekdays = "2,4"
	if err := ta.store.UpdateSeriesCommand(id, sr, today(), true); err != nil {
		t.Fatal(err)
	}
	var missing int
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM lessons
		WHERE series_id=? AND COALESCE(school_year_id,0)<>?`, id, year.ID).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != 0 {
		t.Fatalf("%d rematerialized lessons lost school year %d", missing, year.ID)
	}
}

func TestBulkImportsRollbackAsOneAction(t *testing.T) {
	ta := newTestApp(t)
	if _, err := ta.store.db().Exec(`CREATE TEMP TRIGGER reject_bad_event
		BEFORE INSERT ON adult_events WHEN NEW.title='reject'
		BEGIN SELECT RAISE(ABORT,'reject event'); END`); err != nil {
		t.Fatal(err)
	}
	adults, err := ta.store.Adults(false)
	if err != nil || len(adults) == 0 {
		t.Fatalf("default adult: %+v err=%v", adults, err)
	}
	adult := adults[0].ID
	err = ta.store.CreateAdultEvents([]AdultEvent{
		{AdultID: adult, StartsOn: today(), EndsOn: today(), Title: "keep"},
		{AdultID: adult, StartsOn: today(), EndsOn: today(), Title: "reject"},
	})
	if err == nil {
		t.Fatal("calendar batch unexpectedly succeeded")
	}
	var count int
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM adult_events WHERE adult_id=?`, adult).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("calendar import left %d partial events", count)
	}

	if _, err := ta.store.db().Exec(`CREATE TEMP TRIGGER reject_pdf_attachment
		BEFORE INSERT ON attachments WHEN NEW.original_name='reject.pdf'
		BEGIN SELECT RAISE(ABORT,'reject attachment'); END`); err != nil {
		t.Fatal(err)
	}
	plan := CurriculumPlan{
		Name: "Atomic PDF", SubjectID: ta.mathSubjectID(),
		Items: []CurriculumItem{{Title: "Lesson one"}},
	}
	_, err = ta.store.ImportCurriculumPDF(plan, Attachment{
		OriginalName: "reject.pdf", StoredPath: "test/reject.pdf", ContentType: "application/pdf",
	})
	if err == nil {
		t.Fatal("PDF import unexpectedly succeeded")
	}
	if err := ta.store.db().QueryRow(`SELECT COUNT(*) FROM curriculum_plans WHERE name=?`, plan.Name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("PDF import left a partial curriculum plan")
	}
}

func TestHistoryHTTPRejectsStaleTab(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	ta.post("/lessons", url.Values{
		"kid_id": {itoa64(kid)}, "subject_id": {itoa64(ta.mathSubjectID())},
		"scheduled_on": {today()}, "title": {"Captured"}, "status": {"planned"},
	})
	status, body := ta.post("/history/undo", url.Values{
		"expected": {"99999"}, "back": {"/planner"},
	})
	if status != http.StatusConflict || !strings.Contains(body, "another tab") {
		t.Fatalf("stale history status=%d body=%q", status, body)
	}

	pos, err := ta.store.HistoryPosition()
	if err != nil {
		t.Fatal(err)
	}
	oldRevision := pos.Revision
	if err := ta.store.Undo(pos.CurrentID); err != nil {
		t.Fatal(err)
	}
	if err := ta.store.Redo(0); err != nil {
		t.Fatal(err)
	}
	status, body = ta.post("/history/undo", url.Values{
		"expected":          {itoa64(pos.CurrentID)},
		"expected_revision": {itoa64(oldRevision)},
		"back":              {"/planner"},
	})
	if status != http.StatusConflict || !strings.Contains(body, "another tab") {
		t.Fatalf("same-node stale revision status=%d body=%q", status, body)
	}
}
