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



