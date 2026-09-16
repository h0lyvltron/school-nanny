package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestExtractCurriculumFromPDFText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "toc.pdf")
	if err := writeMinimalTOCPDF(path); err != nil {
		t.Fatal(err)
	}
	items, method, err := ExtractCurriculumFromPDF(path)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if method != "text" && method != "bookmarks" {
		t.Fatalf("method=%q", method)
	}
	if len(items) < 2 {
		t.Fatalf("items=%d method=%s", len(items), method)
	}
	if !items[0].HasPage || items[0].Page != 2 {
		t.Fatalf("first item pages: %+v", items[0])
	}
}

func TestPreferTOCRegion(t *testing.T) {
	in := "Copyright\nPreface\nTable of Contents\nUnit 1\nLesson 1: A . . . 2\n"
	out := preferTOCRegion(in)
	if !strings.Contains(out, "Unit 1") || !strings.Contains(out, "Lesson 1") {
		t.Fatalf("out=%q", out)
	}
	if strings.Contains(out, "Copyright") {
		t.Fatalf("should drop front matter: %q", out)
	}
}

func writeMinimalTOCPDF(path string) error {
	pages := [][]string{
		{
			"Table of Contents",
			"Unit 1",
			"Lesson 1: Identifying Right and Left . . . 2",
			"Lesson 2: Writing Numbers 1-5 . . . 4",
		},
		{"Lesson body page 2"},
	}
	var out []byte
	offsets := map[int]int{}
	write := func(s string) { out = append(out, s...) }
	write("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	obj := func(n int, body string) {
		offsets[n] = len(out)
		write(strconv.Itoa(n) + " 0 obj\n" + body + "\nendobj\n")
	}
	obj(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	var pageIDs []int
	nid := 4
	for _, lines := range pages {
		cid := nid
		nid++
		pid := nid
		nid++
		parts := []string{"BT /F1 12 Tf 72 750 Td"}
		for i, line := range lines {
			safe := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(line)
			if i > 0 {
				parts = append(parts, "0 -18 Td")
			}
			parts = append(parts, "("+safe+") Tj")
		}
		parts = append(parts, "ET")
		stream := strings.Join(parts, "\n")
		obj(cid, "<< /Length "+strconv.Itoa(len(stream))+" >>\nstream\n"+stream+"\nendstream")
		obj(pid, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents "+strconv.Itoa(cid)+
			" 0 R /Resources << /Font << /F1 3 0 R >> >> >>")
		pageIDs = append(pageIDs, pid)
	}
	kids := make([]string, len(pageIDs))
	for i, p := range pageIDs {
		kids[i] = strconv.Itoa(p) + " 0 R"
	}
	obj(2, "<< /Type /Pages /Kids ["+strings.Join(kids, " ")+"] /Count "+strconv.Itoa(len(pageIDs))+" >>")
	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	xrefPos := len(out)
	write("xref\n")
	write("0 " + strconv.Itoa(nid) + "\n")
	write("0000000000 65535 f \n")
	for i := 1; i < nid; i++ {
		write(pad10(offsets[i]) + " 00000 n \n")
	}
	write("trailer\n")
	write("<< /Size " + strconv.Itoa(nid) + " /Root 1 0 R >>\n")
	write("startxref\n")
	write(strconv.Itoa(xrefPos) + "\n")
	write("%%EOF\n")
	return os.WriteFile(path, out, 0o644)
}

func pad10(n int) string {
	s := strconv.Itoa(n)
	return strings.Repeat("0", 10-len(s)) + s
}

func TestNormalizePDFExtractedText(t *testing.T) {
	in := "Table of ContentsUnit 1Lesson 1: Alpha . . . 2Lesson 2: Beta . . . 4"
	out := normalizePDFExtractedText(in)
	if !strings.Contains(out, "\nUnit 1\n") {
		t.Fatalf("unit break missing: %q", out)
	}
	if !strings.Contains(out, "\nLesson 1:") || !strings.Contains(out, "\nLesson 2:") {
		t.Fatalf("lesson breaks missing: %q", out)
	}
}

func TestParseColumnTOCStreamOrdersLessonsAndUnits(t *testing.T) {
	stream := `Unit 1 ........ 1
		Lesson 1: Alpha ........ 2
		Lesson 2: Beta ........ 4
		Unit 2 ........ 10
		Lesson 3: Gamma ........ 11
		Lessons 4–5: Assessment ........ 14`
	items, err := parseColumnTOCStream(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("items=%d: %+v", len(items), items)
	}
	if items[0].Number != 1 || items[0].Unit != 1 || items[0].Page != 2 || items[0].Name != "Alpha" {
		t.Fatalf("first item: %+v", items[0])
	}
	if items[2].Number != 3 || items[2].Unit != 2 || items[2].Page != 11 {
		t.Fatalf("third item: %+v", items[2])
	}
	if !items[3].IsRange || items[3].Number != 4 || items[3].End != 5 || items[3].Page != 14 {
		t.Fatalf("range item: %+v", items[3])
	}
	if err := validateLessonCoverage(items); err != nil {
		t.Fatalf("coverage: %v", err)
	}
}

func TestParseColumnTOCStreamHandlesOverviewUnitsAndAmpersandRanges(t *testing.T) {
	stream := `Unit 1 Overview Page ................................ 1
		Lesson 1—Unit 1 Review ................................ 2
		Unit 2 Overview ................................ 4
		Lessons 2 & 3—Writer’s Workshop ................................ 5
		Lesson 4—Course Review ................................ 9`
	items, err := parseColumnTOCStream(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items=%d: %+v", len(items), items)
	}
	if items[0].Name != "Unit 1 Review" || items[0].Unit != 1 || items[0].Page != 2 {
		t.Fatalf("first item: %+v", items[0])
	}
	if !items[1].IsRange || items[1].Number != 2 || items[1].End != 3 ||
		items[1].Unit != 2 || items[1].Page != 5 {
		t.Fatalf("range item: %+v", items[1])
	}
	if err := validateLessonCoverage(items); err != nil {
		t.Fatalf("coverage: %v", err)
	}
}

func TestSplitTOCTitleUsesFirstLeaderPage(t *testing.T) {
	title, page := splitTOCTitleAndPage(
		"Spelling Rule: Double S, F, L, or Z ........ 34 Important Reminder ........ 38",
	)
	if title != "Spelling Rule: Double S, F, L, or Z" || page != 34 {
		t.Fatalf("title=%q page=%d", title, page)
	}
}

func TestBookmarkTOCRejectsSourceFileArtifacts(t *testing.T) {
	items := []TocItem{
		{Number: 1, Name: "Pages from Course Book.pdf", Page: 1, HasPage: true},
		{Number: 2, Name: "Unit 1 working file.pdf", Page: 2, HasPage: true},
	}
	if err := validateBookmarkTOC(items); err == nil {
		t.Fatal("expected source-file bookmark titles to be rejected")
	}
}

func TestValidatePDFTOCRejectsBadOrderAndPages(t *testing.T) {
	items := []TocItem{
		{Number: 1, Unit: 1, Page: 9, HasPage: true},
		{Number: 2, Unit: 1, Page: 500, HasPage: true},
	}
	if err := validatePDFTOC(items, 331); err == nil {
		t.Fatal("expected out-of-document page to fail")
	}
	items[1].Page = 8
	if err := validatePDFTOC(items, 331); err == nil {
		t.Fatal("expected decreasing page to fail")
	}
}

func TestValidatePrintedPageOrderRejectsInterleavedColumns(t *testing.T) {
	items := []TocItem{
		{Number: 1, Page: 2, HasPage: true},
		{Number: 2, Page: 112, HasPage: true},
		{Number: 3, Page: 4, HasPage: true},
	}
	if err := validatePrintedPageOrder(items, 331); err == nil {
		t.Fatal("expected interleaved column pages to fail")
	}
	items[1].Page = 3
	if err := validatePrintedPageOrder(items, 331); err != nil {
		t.Fatalf("ordered pages: %v", err)
	}
}

func TestSuspiciousTOCTitleAndHeadingCleanup(t *testing.T) {
	for _, title := range []string{
		"Fact F120art 1",
		"Coun173y 2s to 70",
		"Using a Number Line for s 40–50Number",
		"Title ........",
	} {
		if !suspiciousTOCTitle(title) {
			t.Errorf("should be suspicious: %q", title)
		}
	}
	if got := titleCasePDFHeading("COUNTING BY 2s to70"); got != "Counting by 2s to 70" {
		t.Fatalf("heading cleanup=%q", got)
	}
}
