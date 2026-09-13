package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const tocENDash = "–"

// TocItem is one parsed lesson (or lesson range) from a pasted table of contents.
type TocItem struct {
	IsRange bool
	Number  int
	End     int
	Name    string
	Page    int
	HasPage bool
	Unit    int
}

// ParseTOC parses a pasted curriculum table of contents into lesson items.
// curriculumName is accepted for API parity with the Odin tool; it is unused while parsing.
func ParseTOC(text, curriculumName string) ([]TocItem, error) {
	_ = curriculumName
	src := strings.TrimPrefix(text, "\uFEFF")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")

	var out []TocItem
	var cur TocItem
	open := false
	unit := 0

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if n, ok := parseUnitLine(line); ok {
			flushTOCItem(&out, &cur, &open)
			unit = n
			continue
		}

		if start, rest, ok := parseLessonStart(line); ok {
			flushTOCItem(&out, &cur, &open)
			cur = start
			cur.Unit = unit
			open = true
			applyTOCFragment(&cur, rest)
			continue
		}

		// Wrap lines only come before the page number. After that, paste junk
		// from running headers ("vii", "© Good and Beautiful", "Continued")
		// must not glue onto the title.
		if open && !cur.HasPage {
			applyTOCFragment(&cur, line)
		}
	}
	flushTOCItem(&out, &cur, &open)

	if len(out) == 0 {
		return nil, fmt.Errorf("that file has no lessons in it")
	}
	return out, nil
}

// TOCItemTitle formats a lesson the same way as the Odin toc2yaml tool.
func TOCItemTitle(it TocItem) string {
	if it.IsRange {
		if it.HasPage {
			return fmt.Sprintf("Lessons %d%s%d: %s (Page: %d)", it.Number, tocENDash, it.End, it.Name, it.Page)
		}
		return fmt.Sprintf("Lessons %d%s%d: %s", it.Number, tocENDash, it.End, it.Name)
	}
	if it.HasPage {
		return fmt.Sprintf("Lesson %d: %s (Page: %d)", it.Number, it.Name, it.Page)
	}
	return fmt.Sprintf("Lesson %d: %s", it.Number, it.Name)
}

// TOCItemNotes builds the notes field used on import (curriculum name + optional unit).
func TOCItemNotes(it TocItem, curriculumName string) string {
	if it.Unit > 0 {
		return fmt.Sprintf("%s: Unit %d", curriculumName, it.Unit)
	}
	return curriculumName
}

// EmitCurriculumYAML writes import-compatible YAML matching the Odin emit_yaml schema.
func EmitCurriculumYAML(name, subject string, items []TocItem) string {
	var b strings.Builder
	b.WriteString("plans:\n")
	b.WriteString("  - name: ")
	writeYAMLScalar(&b, name)
	b.WriteString("\n    subject: ")
	writeYAMLScalar(&b, subject)
	b.WriteString("\n    items:\n")
	for _, it := range items {
		b.WriteString("      - title: ")
		writeYAMLScalar(&b, TOCItemTitle(it))
		b.WriteString("\n        notes: ")
		writeYAMLScalar(&b, TOCItemNotes(it, name))
		b.WriteString("\n")
	}
	return b.String()
}

func writeYAMLScalar(b *strings.Builder, s string) {
	if !yamlNeedsQuote(s) {
		b.WriteString(s)
		return
	}
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
}

func yamlNeedsQuote(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		switch r {
		case ':', '#', '\'', '"', '{', '}', '[', ']', ',', '&', '*', '!', '|', '>', '%', '@', '`', '\\', '/', ' ', '\t':
			return true
		}
	}
	return false
}

func flushTOCItem(out *[]TocItem, cur *TocItem, open *bool) {
	if !*open {
		return
	}
	cur.Name = collapseSpace(cur.Name)
	*out = append(*out, *cur)
	*open = false
	*cur = TocItem{}
}

func applyTOCFragment(cur *TocItem, fragment string) {
	rest, page, ok := takePage(fragment)
	if ok {
		appendTOCName(cur, rest)
		cur.Page = page
		cur.HasPage = true
		return
	}
	appendTOCName(cur, fragment)
}

func appendTOCName(cur *TocItem, fragment string) {
	bit := collapseSpace(fragment)
	if bit == "" {
		return
	}
	if cur.Name == "" {
		cur.Name = bit
		return
	}
	cur.Name = cur.Name + " " + bit
}

func parseUnitLine(line string) (int, bool) {
	if !hasASCIIPrefixCI(line, "unit") {
		return 0, false
	}
	rest := line[4:]
	if rest == "" || !isTOCSpace(rest[0]) {
		return 0, false
	}
	rest = strings.TrimLeft(rest, " \t")
	n, rest, ok := parseIntPrefix(rest)
	if !ok || n <= 0 {
		return 0, false
	}
	_ = rest
	return n, true
}

func parseLessonStart(line string) (TocItem, string, bool) {
	if hasASCIIPrefixCI(line, "lessons") {
		after := line[7:]
		if after == "" || !isTOCSpace(after[0]) {
			return TocItem{}, "", false
		}
		after = strings.TrimLeft(after, " \t")
		n, after, ok := parseIntPrefix(after)
		if !ok {
			return TocItem{}, "", false
		}
		after, ok = skipDash(after)
		if !ok {
			return TocItem{}, "", false
		}
		after = strings.TrimLeft(after, " \t")
		m, after, ok := parseIntPrefix(after)
		if !ok {
			return TocItem{}, "", false
		}
		after = strings.TrimLeft(after, " \t")
		if after == "" {
			return TocItem{}, "", false
		}
		after, ok = skipTitleSep(after)
		if !ok {
			return TocItem{}, "", false
		}
		return TocItem{IsRange: true, Number: n, End: m}, after, true
	}

	if !hasASCIIPrefixCI(line, "lesson") {
		return TocItem{}, "", false
	}
	after := line[6:]
	if after == "" || !isTOCSpace(after[0]) {
		return TocItem{}, "", false
	}
	after = strings.TrimLeft(after, " \t")
	n, after, ok := parseIntPrefix(after)
	if !ok {
		return TocItem{}, "", false
	}
	after = strings.TrimLeft(after, " \t")

	// Singular "Lesson 32–33: Unit Assessment" still counts as a range.
	if dashRest, dashOK := skipDash(after); dashOK {
		rangeEnd, afterM, mok := parseIntPrefix(dashRest)
		if mok {
			afterM = strings.TrimLeft(afterM, " \t")
			if rest, sepOK := skipTitleSep(afterM); sepOK {
				return TocItem{IsRange: true, Number: n, End: rangeEnd}, rest, true
			}
		}
	}

	after, ok = skipTitleSep(after)
	if !ok {
		return TocItem{}, "", false
	}
	return TocItem{Number: n}, after, true
}

// Colons, dashes, and the mojibake that shows up when an em dash is pasted
// from a PDF (UTF-8 bytes read as Windows-1252).
func skipTitleSep(s string) (string, bool) {
	s = strings.TrimLeft(s, " \t")
	emMojibake := string([]byte{0xC3, 0xA2, 0xE2, 0x82, 0xAC, 0xE2, 0x80, 0x9D})
	enMojibake := string([]byte{0xC3, 0xA2, 0xE2, 0x82, 0xAC, 0xE2, 0x80, 0x9C})
	seps := []string{
		emMojibake,
		enMojibake,
		"—",
		"–",
		"−",
		"：",
		":",
		"-",
	}
	for _, sep := range seps {
		if strings.HasPrefix(s, sep) {
			return strings.TrimLeft(s[len(sep):], " \t"), true
		}
	}
	return s, false
}

func skipDash(s string) (string, bool) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return s, false
	}
	emMojibake := string([]byte{0xC3, 0xA2, 0xE2, 0x82, 0xAC, 0xE2, 0x80, 0x9D})
	enMojibake := string([]byte{0xC3, 0xA2, 0xE2, 0x82, 0xAC, 0xE2, 0x80, 0x9C})
	seps := []string{
		emMojibake,
		enMojibake,
		"—",
		"–",
		"−",
		"-",
	}
	for _, sep := range seps {
		if strings.HasPrefix(s, sep) {
			return strings.TrimLeft(s[len(sep):], " \t"), true
		}
	}
	return s, false
}

func takePage(src string) (rest string, page int, ok bool) {
	s := strings.TrimRight(src, " \t")
	if s == "" {
		return s, 0, false
	}
	i := len(s) - 1
	for i >= 0 && s[i] >= '0' && s[i] <= '9' {
		i--
	}
	digitStart := i + 1
	if digitStart >= len(s) {
		return s, 0, false
	}
	j := i
	for j >= 0 && isTOCSpace(s[j]) {
		j--
	}
	dots := 0
	k := j
	for k >= 0 {
		if s[k] == '.' {
			dots++
			k--
			for k >= 0 && isTOCSpace(s[k]) {
				k--
			}
			continue
		}
		break
	}
	if dots < 1 {
		return s, 0, false
	}
	page, _ = strconv.Atoi(s[digitStart:])
	rest = strings.TrimSpace(s[:k+1])
	return rest, page, true
}

func parseIntPrefix(s string) (n int, rest string, ok bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, s, false
	}
	n, _ = strconv.Atoi(s[:i])
	return n, s[i:], true
}

func hasASCIIPrefixCI(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		a := s[i]
		b := prefix[i]
		if a >= 'A' && a <= 'Z' {
			a += 32
		}
		if b >= 'A' && b <= 'Z' {
			b += 32
		}
		if a != b {
			return false
		}
	}
	return true
}

func isTOCSpace(c byte) bool {
	return c == ' ' || c == '\t'
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}
