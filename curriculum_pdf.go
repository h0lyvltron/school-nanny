package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// maxPDFTOCPages caps how many leading pages we OCR-via-text for a pasted-style TOC.
const maxPDFTOCPages = 40

// ExtractCurriculumFromPDF reads a curriculum PDF and returns TOC-shaped items.
// Prefer the document outline/bookmarks when present; otherwise scrape text from
// the early pages and run it through ParseTOC.
func ExtractCurriculumFromPDF(path string) ([]TocItem, string, error) {
	if items, err := extractTOCFromBookmarks(path); err == nil && len(items) > 0 {
		return items, "bookmarks", nil
	}
	text, err := extractPDFFrontMatterText(path, maxPDFTOCPages)
	if err != nil {
		return nil, "", err
	}
	text = preferTOCRegion(text)
	items, err := ParseTOC(text, "")
	if err != nil {
		return nil, "", fmt.Errorf("could not find lessons in that PDF: %w", err)
	}
	return items, "text", nil
}

// ExtractCurriculumFromPDFReader writes r to a temp file (pdfcpu wants a path)
// then extracts. Caller owns the bytes.
func ExtractCurriculumFromPDFReader(r io.Reader) ([]TocItem, string, error) {
	tmp, err := os.CreateTemp("", "school-nanny-curriculum-*.pdf")
	if err != nil {
		return nil, "", err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return nil, "", err
	}
	if err := tmp.Close(); err != nil {
		return nil, "", err
	}
	return ExtractCurriculumFromPDF(path)
}

func extractTOCFromBookmarks(path string) ([]TocItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	bms, err := api.Bookmarks(f, conf)
	if err != nil {
		return nil, err
	}
	var flat []bookmarkRow
	flattenBookmarks(bms, 0, &flat)
	if len(flat) == 0 {
		return nil, fmt.Errorf("no bookmarks")
	}

	// Rebuild a TOC-like text so ParseTOC can share unit/lesson heuristics,
	// falling back to a direct conversion when titles are already lesson-like.
	var b strings.Builder
	unit := 0
	for _, row := range flat {
		title := strings.TrimSpace(row.Title)
		if title == "" {
			continue
		}
		lower := strings.ToLower(title)
		if strings.HasPrefix(lower, "unit ") || strings.HasPrefix(lower, "unit:") {
			unit++
			fmt.Fprintf(&b, "Unit %d\n", unit)
			continue
		}
		if row.Page > 0 {
			fmt.Fprintf(&b, "%s . . . %d\n", title, row.Page)
		} else {
			fmt.Fprintf(&b, "%s\n", title)
		}
	}
	items, err := ParseTOC(b.String(), "")
	if err == nil && len(items) > 0 {
		return items, nil
	}

	// Direct conversion when bookmarks are plain lesson titles without "Lesson N:".
	out := make([]TocItem, 0, len(flat))
	for i, row := range flat {
		title := strings.TrimSpace(row.Title)
		if title == "" {
			continue
		}
		lower := strings.ToLower(title)
		if strings.HasPrefix(lower, "unit ") {
			continue
		}
		it := TocItem{Number: i + 1, Name: title}
		if row.Page > 0 {
			it.Page = row.Page
			it.HasPage = true
		}
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no lesson bookmarks")
	}
	return out, nil
}

type bookmarkRow struct {
	Title string
	Page  int
	Level int
}

func flattenBookmarks(bms []pdfcpu.Bookmark, level int, out *[]bookmarkRow) {
	for _, bm := range bms {
		page := bm.PageFrom
		if page <= 0 {
			page = bm.PageThru
		}
		*out = append(*out, bookmarkRow{Title: bm.Title, Page: page, Level: level})
		if len(bm.Kids) > 0 {
			flattenBookmarks(bm.Kids, level+1, out)
		}
	}
}

func extractPDFFrontMatterText(path string, maxPages int) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	n := r.NumPage()
	if n <= 0 {
		return "", fmt.Errorf("that PDF has no pages")
	}
	if maxPages > 0 && n > maxPages {
		n = maxPages
	}

	var b strings.Builder
	for i := 1; i <= n; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		rows, err := page.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range rows {
			var line strings.Builder
			for _, word := range row.Content {
				line.WriteString(word.S)
			}
			text := strings.TrimSpace(line.String())
			if text == "" {
				continue
			}
			b.WriteString(text)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("could not read text from that PDF")
	}
	return normalizePDFExtractedText(out), nil
}

// normalizePDFExtractedText inserts line breaks before Unit/Lesson markers when
// a PDF text extractor glued TOC rows into one long string.
var tocBreakRE = regexp.MustCompile(`(?i)(Unit\s+\d+|Lessons?\s+\d+)`)

func normalizePDFExtractedText(text string) string {
	return tocBreakRE.ReplaceAllStringFunc(text, func(m string) string {
		return "\n" + m
	})
}

// preferTOCRegion drops front-matter before a TOC heading when present.
func preferTOCRegion(text string) string {
	lines := strings.Split(text, "\n")
	start := 0
	for i, line := range lines {
		trim := strings.TrimSpace(strings.ToLower(line))
		if trim == "table of contents" || trim == "contents" ||
			strings.HasPrefix(trim, "table of contents") {
			start = i + 1
			break
		}
	}
	if start == 0 {
		return text
	}
	return strings.Join(lines[start:], "\n")
}
