package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

var (
	pdfLessonMarkerRE   = regexp.MustCompile(`(?i)Lessons?\s+\d+(?:\s*(?:[–—-]|&)\s*\d+)?\s*[:—-]?\s*`)
	pdfUnitOverviewRE   = regexp.MustCompile(`(?i)Unit\s+\d+\s+Overview(?:\s+Page)?`)
	pdfTOCNumberRE      = regexp.MustCompile(`\d+`)
	pdfTOCPageRE        = regexp.MustCompile(`(?s)^(.*?)(?:\.+|\s)(\d{1,4})\s*$`)
	pdfTOCAnyPageRE     = regexp.MustCompile(`(?s)^(.*?)\.{2,}\s*(\d{1,4}).*$`)
	pdfEmbeddedNumberRE = regexp.MustCompile(`[[:alpha:]]\d{2,}[[:alpha:]]|\d+[–-]\d+\s*Number`)
)

// extractColumnTOC handles publisher TOCs laid out in multiple columns. PDF
// text is painted by visual row, so flattening a row joins unrelated columns.
// This rebuilds one stream per column, then reads the columns left-to-right.
func extractColumnTOC(path string) ([]TocItem, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tocPages, err := findTOCPages(r, min(r.NumPage(), 20))
	if err != nil {
		return nil, err
	}

	var items []TocItem
	for columnCount := 1; columnCount <= 3; columnCount++ {
		var stream strings.Builder
		for _, tocPage := range tocPages {
			text, textErr := columnOrderedText(tocPage.Rows, columnCount)
			if textErr != nil {
				stream.Reset()
				break
			}
			stream.WriteByte(' ')
			stream.WriteString(text)
		}
		if stream.Len() == 0 {
			continue
		}
		candidate, parseErr := parseColumnTOCStream(stream.String())
		if parseErr == nil && validateLessonCoverage(candidate) == nil &&
			validatePrintedPageOrder(candidate, r.NumPage()) == nil {
			items = candidate
			break
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("could not reconstruct the table of contents in reading order")
	}

	tocEndPage := tocPages[len(tocPages)-1].Number
	offset := inferPDFPageOffset(r, tocEndPage, items)
	repairTitle := make([]bool, len(items))
	for i := range items {
		printed := items[i].Page
		repairTitle[i] = suspiciousTOCTitle(items[i].Name) ||
			!items[i].HasPage || printed <= 0 || printed > r.NumPage()
		if items[i].HasPage && printed > 0 && printed <= r.NumPage() {
			items[i].Page = printed + offset
			continue
		}
		items[i].Page = 0
		items[i].HasPage = false
	}
	for i := range items {
		if items[i].HasPage {
			continue
		}
		from := tocEndPage + 1
		if i > 0 && items[i-1].HasPage {
			from = items[i-1].Page + 1
		}
		to := min(r.NumPage(), from+20)
		for j := i + 1; j < len(items); j++ {
			if items[j].HasPage {
				to = items[j].Page - 1
				break
			}
		}
		if physical := findLessonHeaderPage(r, from, to, items[i].Number); physical > 0 {
			items[i].Page = physical
			items[i].HasPage = true
		}
	}

	// Prefer the actual lesson-page heading only when the TOC row was visibly
	// damaged. The TOC wording is otherwise cleaner than activity-page chrome.
	for i := range items {
		if !repairTitle[i] || !items[i].HasPage ||
			items[i].Page <= 0 || items[i].Page > r.NumPage() {
			continue
		}
		if title := lessonHeading(r.Page(items[i].Page), items[i].Number); title != "" {
			items[i].Name = title
		}
	}
	if err := validatePDFTOC(items, r.NumPage()); err != nil {
		return nil, err
	}
	return items, nil
}

func suspiciousTOCTitle(title string) bool {
	if strings.Contains(title, "...") || strings.ContainsRune(title, unicode.ReplacementChar) {
		return true
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return true
		}
	}
	return pdfEmbeddedNumberRE.MatchString(title)
}

type positionedTOCPage struct {
	Number      int
	Rows        pdf.Rows
	MarkerCount int
}

func findTOCPages(r *pdf.Reader, maxPages int) ([]positionedTOCPage, error) {
	var candidates []positionedTOCPage
	for pageNumber := 1; pageNumber <= maxPages; pageNumber++ {
		rows, err := r.Page(pageNumber).GetTextByRow()
		if err != nil {
			continue
		}
		var text strings.Builder
		for _, row := range rows {
			for _, piece := range row.Content {
				text.WriteString(piece.S)
			}
			text.WriteByte('\n')
		}
		count := len(pdfLessonMarkerRE.FindAllString(text.String(), -1))
		if count >= 8 {
			candidates = append(candidates, positionedTOCPage{
				Number: pageNumber, Rows: rows, MarkerCount: count,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("could not find lesson entries in that PDF")
	}

	// Painting-credit and index pages may mention many lessons. The actual TOC
	// is the consecutive run with the most lesson markers.
	bestStart, bestEnd, bestScore := 0, 1, candidates[0].MarkerCount
	for start := 0; start < len(candidates); {
		end := start + 1
		score := candidates[start].MarkerCount
		for end < len(candidates) && candidates[end].Number == candidates[end-1].Number+1 {
			score += candidates[end].MarkerCount
			end++
		}
		if score > bestScore {
			bestStart, bestEnd, bestScore = start, end, score
		}
		start = end
	}
	return candidates[bestStart:bestEnd], nil
}

func columnOrderedText(rows pdf.Rows, columnCount int) (string, error) {
	minX := 0.0
	maxX := 0.0
	found := false
	for _, row := range rows {
		for _, piece := range row.Content {
			if strings.TrimSpace(piece.S) == "" {
				continue
			}
			if !found || piece.X < minX {
				minX = piece.X
			}
			if right := piece.X + piece.W; !found || right > maxX {
				maxX = right
			}
			found = true
		}
	}
	if !found || maxX <= minX {
		return "", fmt.Errorf("that TOC page has no positioned text")
	}

	// Small outer padding keeps edge glyphs in their column.
	minX -= 2
	maxX += 2
	width := (maxX - minX) / float64(columnCount)
	var stream strings.Builder
	for column := 0; column < columnCount; column++ {
		left := minX + float64(column)*width
		right := left + width
		for _, row := range rows {
			var line strings.Builder
			for _, piece := range row.Content {
				if piece.X >= left && (piece.X < right || column == columnCount-1) {
					line.WriteString(piece.S)
				}
			}
			cleaned := cleanPDFText(line.String())
			if cleaned == "" {
				continue
			}
			stream.WriteByte(' ')
			stream.WriteString(cleaned)
		}
	}
	return stream.String(), nil
}

func cleanPDFText(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func parseColumnTOCStream(stream string) ([]TocItem, error) {
	unitPattern := `Unit\s+\d+`
	if pdfUnitOverviewRE.MatchString(stream) {
		unitPattern = `Unit\s+\d+\s+Overview(?:\s+Page)?`
	}
	markerRE := regexp.MustCompile(`(?i)(` + unitPattern +
		`|Lessons?\s+\d+(?:\s*(?:[–—-]|&)\s*\d+)?)\s*[:—-]?\s*`)
	matches := markerRE.FindAllStringIndex(stream, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("that TOC has no lesson markers")
	}

	unit := 0
	var items []TocItem
	for i, match := range matches {
		label := strings.TrimSpace(stream[match[0]:match[1]])
		end := len(stream)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		segment := strings.TrimSpace(stream[match[1]:end])
		numbers := pdfTOCNumberRE.FindAllString(label, -1)
		if len(numbers) == 0 {
			continue
		}
		if strings.HasPrefix(strings.ToLower(label), "unit") {
			unit, _ = strconv.Atoi(numbers[0])
			continue
		}

		number, _ := strconv.Atoi(numbers[0])
		rangeEnd := number
		isRange := len(numbers) > 1
		if isRange {
			rangeEnd, _ = strconv.Atoi(numbers[1])
		}
		title, page := splitTOCTitleAndPage(segment)
		if title == "" {
			title = fmt.Sprintf("Lesson %d", number)
		}
		item := TocItem{
			IsRange: isRange,
			Number:  number,
			End:     rangeEnd,
			Name:    title,
			Unit:    unit,
		}
		if page > 0 {
			item.Page = page
			item.HasPage = true
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Number < items[j].Number
	})
	return items, nil
}

func splitTOCTitleAndPage(segment string) (string, int) {
	match := pdfTOCAnyPageRE.FindStringSubmatch(segment)
	if match == nil {
		match = pdfTOCPageRE.FindStringSubmatch(segment)
	}
	if match == nil {
		return strings.Trim(segment, " .:-"), 0
	}
	page, _ := strconv.Atoi(match[2])
	return strings.Trim(strings.TrimSpace(match[1]), " .:-"), page
}

func validateLessonCoverage(items []TocItem) error {
	if len(items) == 0 {
		return fmt.Errorf("the TOC produced no lessons")
	}
	expected := 1
	for _, item := range items {
		end := item.End
		if !item.IsRange {
			end = item.Number
		}
		if item.Number != expected || end < item.Number {
			return fmt.Errorf("the TOC lesson sequence is incomplete near lesson %d", expected)
		}
		expected = end + 1
	}
	return nil
}

func validatePrintedPageOrder(items []TocItem, pageCount int) error {
	lastPage := 0
	validPages := 0
	for _, item := range items {
		if !item.HasPage || item.Page <= 0 || item.Page > pageCount {
			continue
		}
		validPages++
		if item.Page <= lastPage {
			return fmt.Errorf("printed pages are out of order near lesson %d", item.Number)
		}
		lastPage = item.Page
	}
	if validPages*4 < len(items)*3 {
		return fmt.Errorf("too many lessons have no usable printed page")
	}
	return nil
}

func validatePDFTOC(items []TocItem, pageCount int) error {
	lastPage := 0
	for _, item := range items {
		if item.Unit <= 0 {
			return fmt.Errorf("lesson %d is not associated with a unit", item.Number)
		}
		if !item.HasPage || item.Page <= 0 || item.Page > pageCount {
			return fmt.Errorf("lesson %d has no valid PDF page", item.Number)
		}
		if item.Page <= lastPage {
			return fmt.Errorf("PDF pages are out of order near lesson %d", item.Number)
		}
		lastPage = item.Page
	}
	return nil
}

func inferPDFPageOffset(r *pdf.Reader, tocEndPage int, items []TocItem) int {
	const maxOffset = 60
	counts := make([]int, maxOffset+1)
	pageText := make(map[int]string)
	for _, item := range items {
		if !item.HasPage || item.Page <= 0 || item.Name == "" {
			continue
		}
		needle := normalizedPDFWords(item.Name)
		if len(needle) < 8 {
			continue
		}
		for offset := 0; offset <= maxOffset; offset++ {
			pageNumber := item.Page + offset
			if pageNumber <= tocEndPage || pageNumber > r.NumPage() {
				continue
			}
			text, ok := pageText[pageNumber]
			if !ok {
				text = normalizedPageText(r.Page(pageNumber))
				pageText[pageNumber] = text
			}
			if strings.Contains(text, needle) {
				counts[offset]++
			}
		}
	}
	bestOffset, bestCount := 0, 0
	for offset, count := range counts {
		if count > bestCount {
			bestOffset, bestCount = offset, count
		}
	}
	return bestOffset
}

func normalizedPageText(page pdf.Page) string {
	rows, err := page.GetTextByRow()
	if err != nil {
		return ""
	}
	var text strings.Builder
	for _, row := range rows {
		for _, piece := range row.Content {
			text.WriteString(piece.S)
		}
		text.WriteByte(' ')
	}
	return normalizedPDFWords(text.String())
}

func normalizedPDFWords(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			return unicode.ToLower(r)
		default:
			return -1
		}
	}, s)
}

func findLessonHeaderPage(r *pdf.Reader, from, to, lessonNumber int) int {
	numberOnly := regexp.MustCompile(`^\s*(\d{1,3})\s*$`)
	for pageNumber := from; pageNumber <= to; pageNumber++ {
		rows, err := r.Page(pageNumber).GetTextByRow()
		if err != nil {
			continue
		}
		lines := firstPDFLines(rows, 14)
		for i, line := range lines {
			if !strings.EqualFold(strings.TrimSpace(line), "lesson") {
				continue
			}
			for j := i + 1; j < len(lines) && j <= i+4; j++ {
				match := numberOnly.FindStringSubmatch(lines[j])
				if match == nil {
					continue
				}
				number, _ := strconv.Atoi(match[1])
				if number == lessonNumber {
					return pageNumber
				}
				break
			}
		}
	}
	return 0
}

func lessonHeading(page pdf.Page, lessonNumber int) string {
	rows, err := page.GetTextByRow()
	if err != nil {
		return ""
	}
	lines := firstPDFLines(rows, 14)
	number := strconv.Itoa(lessonNumber)
	var candidates []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == number || strings.EqualFold(line, "lesson") ||
			strings.EqualFold(line, "lessons") || strings.EqualFold(line, "review") {
			continue
		}
		if headingLike(line) {
			candidates = append(candidates, strings.Join(strings.Fields(line), " "))
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	// Lesson headings occasionally wrap onto two uppercase lines.
	if len(candidates) > 1 && len(candidates[0])+len(candidates[1]) < 90 {
		return titleCasePDFHeading(candidates[0] + " " + candidates[1])
	}
	return titleCasePDFHeading(candidates[0])
}

func firstPDFLines(rows pdf.Rows, limit int) []string {
	lines := make([]string, 0, limit)
	for _, row := range rows {
		var line strings.Builder
		for _, piece := range row.Content {
			line.WriteString(piece.S)
		}
		cleaned := cleanPDFText(line.String())
		if cleaned == "" {
			continue
		}
		lines = append(lines, cleaned)
		if len(lines) == limit {
			break
		}
	}
	return lines
}

func headingLike(s string) bool {
	letters := 0
	upper := 0
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.IsUpper(r) {
			upper++
		}
	}
	return letters >= 4 && float64(upper)/float64(letters) >= 0.65
}

var (
	pdfLetterDigitRE = regexp.MustCompile(`([[:alpha:]])(\d)`)
	pdfDigitLetterRE = regexp.MustCompile(`(\d)([[:alpha:]])`)
)

func titleCasePDFHeading(s string) string {
	s = pdfLetterDigitRE.ReplaceAllString(s, `$1 $2`)
	// Keep common forms such as "2s", but separate accidental "70Number".
	s = pdfDigitLetterRE.ReplaceAllStringFunc(s, func(part string) string {
		if strings.HasSuffix(strings.ToLower(part), "s") {
			return part
		}
		return part[:1] + " " + part[1:]
	})
	words := strings.Fields(strings.ToLower(s))
	small := map[string]bool{"a": true, "an": true, "and": true, "by": true, "for": true, "from": true, "in": true, "of": true, "on": true, "or": true, "the": true, "to": true}
	for i, word := range words {
		if i > 0 && small[word] {
			continue
		}
		runes := []rune(word)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}
