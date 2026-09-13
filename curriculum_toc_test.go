package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tocTestdata(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("tools", "toc2yaml", "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestSkipsFrontMatterAndParsesUnits(t *testing.T) {
	src := tocTestdata(t, "math1-toc.txt")
	items, err := ParseTOC(src, "Grade 1 Math")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("len(items)=%d want 6", len(items))
	}

	if got := TOCItemTitle(items[0]); got != "Lesson 1: Identifying Right and Left (Page: 2)" {
		t.Fatalf("items[0] title=%q", got)
	}
	if got := TOCItemNotes(items[0], "Grade 1 Math"); got != "Grade 1 Math: Unit 1" {
		t.Fatalf("items[0] notes=%q", got)
	}
	if items[0].Page != 2 {
		t.Fatalf("items[0].Page=%d want 2", items[0].Page)
	}

	if got := TOCItemTitle(items[1]); got != "Lesson 2: Writing Numbers 1–5 (Page: 4)" {
		t.Fatalf("items[1] title=%q", got)
	}
	if items[1].Unit != 1 {
		t.Fatalf("items[1].Unit=%d want 1", items[1].Unit)
	}
}

func TestWrappedLessons(t *testing.T) {
	src := tocTestdata(t, "math1-toc.txt")
	items, err := ParseTOC(src, "Grade 1 Math")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}

	if got := TOCItemTitle(items[2]); got != "Lesson 11: Writing One, Two, Three/ Order of Events (Page: 33)" {
		t.Fatalf("items[2] title=%q", got)
	}
	if items[2].Page != 33 {
		t.Fatalf("items[2].Page=%d want 33", items[2].Page)
	}

	if got := TOCItemTitle(items[3]); got != "Lesson 34: Using a Number Line for Numbers Up to 40 (Page: 92)" {
		t.Fatalf("items[3] title=%q", got)
	}
	if items[3].Page != 92 {
		t.Fatalf("items[3].Page=%d want 92", items[3].Page)
	}
}

func TestAssessmentRangeAndNextUnit(t *testing.T) {
	src := tocTestdata(t, "math1-toc.txt")
	items, err := ParseTOC(src, "Grade 1 Math")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}

	if got := TOCItemTitle(items[4]); got != "Lessons 39–40: Unit Assessment (Page: 105)" {
		t.Fatalf("items[4] title=%q", got)
	}
	if got := TOCItemNotes(items[4], "Grade 1 Math"); got != "Grade 1 Math: Unit 1" {
		t.Fatalf("items[4] notes=%q", got)
	}
	if !items[4].IsRange {
		t.Fatal("items[4].IsRange=false want true")
	}
	if items[4].Number != 39 || items[4].End != 40 {
		t.Fatalf("items[4] range=%d–%d want 39–40", items[4].Number, items[4].End)
	}
	if items[4].Page != 105 {
		t.Fatalf("items[4].Page=%d want 105", items[4].Page)
	}

	if got := TOCItemTitle(items[5]); got != "Lesson 41: Writing Four, Five, Six (Page: 112)" {
		t.Fatalf("items[5] title=%q", got)
	}
	if got := TOCItemNotes(items[5], "Grade 1 Math"); got != "Grade 1 Math: Unit 2" {
		t.Fatalf("items[5] notes=%q", got)
	}
}

func TestHyphenRange(t *testing.T) {
	src := "Unit 1\nLessons 39-40: Unit Assessment . . . 105\n"
	items, err := ParseTOC(src, "Grade 1 Math")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items)=%d want 1", len(items))
	}
	if got := TOCItemTitle(items[0]); got != "Lessons 39–40: Unit Assessment (Page: 105)" {
		t.Fatalf("title=%q", got)
	}
}

func TestEmptyIsAnError(t *testing.T) {
	_, err := ParseTOC("Table of Contents\n\n", "Grade 1 Math")
	if err == nil || err.Error() != "that file has no lessons in it" {
		t.Fatalf("err=%v", err)
	}
}

func TestYAMLQuotesTitles(t *testing.T) {
	src := tocTestdata(t, "math1-toc.txt")
	items, err := ParseTOC(src, "Grade 1 Math")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}
	yaml := EmitCurriculumYAML("Grade 1 Math", "math", items)
	if !strings.Contains(yaml, "subject: math") {
		t.Fatalf("missing subject: math in %q", yaml)
	}
	if !strings.Contains(yaml, `title: "Lesson 1: Identifying Right and Left (Page: 2)"`) {
		t.Fatalf("missing quoted title in %q", yaml)
	}
	if !strings.Contains(yaml, `notes: "Grade 1 Math: Unit 1"`) {
		t.Fatalf("missing quoted notes in %q", yaml)
	}
	if strings.Contains(yaml, "minutes:") {
		t.Fatal("yaml unexpectedly contains minutes:")
	}
}

func TestLanguageArtsEmDashAndOverview(t *testing.T) {
	src := tocTestdata(t, "la-k.txt")
	items, err := ParseTOC(src, "Level K Language Arts")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}
	if len(items) != 120 {
		t.Fatalf("len(items)=%d want 120", len(items))
	}

	if got := TOCItemTitle(items[0]); got != "Lesson 1: Vowels: Part 1 (Page: 2)" {
		t.Fatalf("items[0] title=%q", got)
	}
	if got := TOCItemNotes(items[0], "Level K Language Arts"); got != "Level K Language Arts: Unit 1" {
		t.Fatalf("items[0] notes=%q", got)
	}
	if items[0].Page != 2 {
		t.Fatalf("items[0].Page=%d want 2", items[0].Page)
	}

	if got := TOCItemTitle(items[38]); got != "Lesson 39: Unit 1 Review (Page: 103)" {
		t.Fatalf("items[38] title=%q", got)
	}
	if items[38].Unit != 1 {
		t.Fatalf("items[38].Unit=%d want 1", items[38].Unit)
	}

	if got := TOCItemTitle(items[39]); got != "Lesson 40: Unit 2 Spelling Words (Page: 108)" {
		t.Fatalf("items[39] title=%q", got)
	}
	if got := TOCItemNotes(items[39], "Level K Language Arts"); got != "Level K Language Arts: Unit 2" {
		t.Fatalf("items[39] notes=%q", got)
	}

	if got := TOCItemTitle(items[119]); got != "Lesson 120: Unit 3 Review (Page: 341)" {
		t.Fatalf("items[119] title=%q", got)
	}
	if items[119].Unit != 3 {
		t.Fatalf("items[119].Unit=%d want 3", items[119].Unit)
	}
}

func TestMath1FullPaste(t *testing.T) {
	src := tocTestdata(t, "math1-full.txt")
	items, err := ParseTOC(src, "Grade 1 Math")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}
	if len(items) < 115 {
		t.Fatalf("len(items)=%d want >= 115", len(items))
	}
	if got := TOCItemTitle(items[0]); got != "Lesson 1: Identifying Right and Left (Page: 2)" {
		t.Fatalf("items[0] title=%q", got)
	}
	last := items[len(items)-1]
	if !last.IsRange {
		t.Fatal("last.IsRange=false want true")
	}
	if last.Page != 318 {
		t.Fatalf("last.Page=%d want 318", last.Page)
	}
}

func TestPasteArtifactsAfterPageAreIgnored(t *testing.T) {
	src := tocTestdata(t, "la-1-artifacts.txt")
	items, err := ParseTOC(src, "Level 1 Language Arts")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("len(items)=%d want 6", len(items))
	}

	if got := TOCItemTitle(items[1]); got != "Lesson 37: Poetry (Page: 102)" {
		t.Fatalf("items[1] title=%q", got)
	}
	if got := TOCItemTitle(items[2]); got != "Lesson 38: Two-Syllable Words: Part 3 (Page: 105)" {
		t.Fatalf("items[2] title=%q", got)
	}
	if got := TOCItemTitle(items[4]); got != "Lesson 40: Unit 1 Review (Page: 109)" {
		t.Fatalf("items[4] title=%q", got)
	}
	if items[4].Unit != 1 {
		t.Fatalf("items[4].Unit=%d want 1", items[4].Unit)
	}
	if got := TOCItemTitle(items[5]); got != "Lesson 41: Sight Words: Group 2 (Page: 113)" {
		t.Fatalf("items[5] title=%q", got)
	}
	if items[5].Unit != 2 {
		t.Fatalf("items[5].Unit=%d want 2", items[5].Unit)
	}
}

func TestParseTOCHandlesBOMAndCRLF(t *testing.T) {
	src := "\uFEFFUnit 1\r\nLesson 1: Hello . . . 3\r\n"
	items, err := ParseTOC(src, "Demo")
	if err != nil {
		t.Fatalf("ParseTOC: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items)=%d want 1", len(items))
	}
	if got := TOCItemTitle(items[0]); got != "Lesson 1: Hello (Page: 3)" {
		t.Fatalf("title=%q", got)
	}
}
