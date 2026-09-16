package main

import (
	"fmt"
	"strings"
	"unicode"
)

// EmitPlansYAML marshals curriculum plans into the same schema import accepts.
func EmitPlansYAML(plans []CurriculumPlan) (string, error) {
	if len(plans) == 0 {
		return "", fmt.Errorf("there are no plans to export")
	}
	var b strings.Builder
	b.WriteString("plans:\n")
	for _, p := range plans {
		if err := writePlanYAML(&b, p); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func writePlanYAML(b *strings.Builder, p CurriculumPlan) error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return fmt.Errorf("a plan is missing a name")
	}
	subject := strings.TrimSpace(p.SubjectName)
	if subject == "" {
		return fmt.Errorf("plan %q is missing a subject", name)
	}

	b.WriteString("  - name: ")
	writeYAMLScalar(b, name)
	b.WriteString("\n    subject: ")
	writeYAMLScalar(b, subject)
	if notes := strings.TrimSpace(p.Notes); notes != "" {
		b.WriteString("\n    notes: ")
		writeYAMLScalar(b, notes)
	}
	b.WriteString("\n    items:\n")
	if len(p.Items) == 0 {
		return fmt.Errorf("plan %q has no lessons", name)
	}
	for _, it := range p.Items {
		title := strings.TrimSpace(it.Title)
		if title == "" {
			return fmt.Errorf("plan %q has a lesson with no title", name)
		}
		b.WriteString("      - title: ")
		writeYAMLScalar(b, title)
		if it.WeekNumber > 0 {
			b.WriteString("\n        week: ")
			fmt.Fprintf(b, "%d", it.WeekNumber)
		}
		if it.Minutes > 0 {
			b.WriteString("\n        minutes: ")
			fmt.Fprintf(b, "%d", it.Minutes)
		}
		if it.PageStart > 0 {
			b.WriteString("\n        page_start: ")
			fmt.Fprintf(b, "%d", it.PageStart)
		}
		if it.PageEnd > 0 {
			b.WriteString("\n        page_end: ")
			fmt.Fprintf(b, "%d", it.PageEnd)
		}
		if notes := strings.TrimSpace(it.Notes); notes != "" {
			b.WriteString("\n        notes: ")
			writeYAMLScalar(b, notes)
		}
		b.WriteByte('\n')
	}
	return nil
}

func planYAMLFilename(p CurriculumPlan) string {
	base := slugifyFilename(p.Name)
	if base == "" {
		base = fmt.Sprintf("plan-%d", p.ID)
	}
	return base + ".yaml"
}

func slugifyFilename(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '_' || r == '-' || r == '.':
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = out[:60]
		out = strings.TrimRight(out, "-")
	}
	return out
}
