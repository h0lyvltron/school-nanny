package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxImportBytes = 1 << 20

type importDoc struct {
	Plans []importPlan `yaml:"plans"`
}

type importPlan struct {
	Name    string       `yaml:"name"`
	Subject string       `yaml:"subject"`
	Notes   string       `yaml:"notes"`
	Items   []importItem `yaml:"items"`
}

type importItem struct {
	Week    int    `yaml:"week"`
	Title   string `yaml:"title"`
	Minutes int    `yaml:"minutes"`
	Notes   string `yaml:"notes"`
}

func parseCurriculumImport(filename string, body []byte, subjects []Subject) ([]CurriculumPlan, error) {
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("that file is empty")
	}

	var drafts []importPlan
	var err error
	switch importKind(filename, body) {
	case "csv":
		drafts, err = parseCurriculumCSV(body)
	default:
		drafts, err = parseCurriculumYAML(body)
	}
	if err != nil {
		return nil, err
	}
	if len(drafts) == 0 {
		return nil, fmt.Errorf("that file has no plans in it")
	}
	return resolveImportPlans(drafts, subjects)
}

func importKind(filename string, body []byte) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		return "csv"
	case ".yaml", ".yml":
		return "yaml"
	}
	trim := bytes.TrimSpace(body)
	if bytes.HasPrefix(trim, []byte("plans:")) || bytes.Contains(trim[:min(len(trim), 400)], []byte("plans:")) {
		return "yaml"
	}
	first, _, _ := strings.Cut(string(trim), "\n")
	lower := strings.ToLower(first)
	if strings.Contains(lower, "plan") && strings.Contains(lower, "title") && strings.Contains(first, ",") {
		return "csv"
	}
	return "yaml"
}

func parseCurriculumYAML(body []byte) ([]importPlan, error) {
	var doc importDoc
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("could not read that YAML: %v", err)
	}
	return doc.Plans, nil
}

func parseCurriculumCSV(body []byte) ([]importPlan, error) {
	r := csv.NewReader(bytes.NewReader(body))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("could not read that CSV: %v", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("that CSV needs a header row and at least one lesson")
	}

	idx := csvHeaderIndex(rows[0])
	if _, ok := idx["plan"]; !ok {
		return nil, fmt.Errorf("that CSV needs a plan column")
	}
	if _, ok := idx["subject"]; !ok {
		return nil, fmt.Errorf("that CSV needs a subject column")
	}
	if _, ok := idx["title"]; !ok {
		return nil, fmt.Errorf("that CSV needs a title column")
	}

	var plans []importPlan
	type key struct{ plan, subject string }
	seen := map[key]int{}

	for i, row := range rows[1:] {
		line := i + 2
		planName := csvField(row, idx, "plan")
		subject := csvField(row, idx, "subject")
		title := csvField(row, idx, "title")
		if planName == "" && subject == "" && title == "" {
			continue
		}
		if planName == "" {
			return nil, fmt.Errorf("line %d: a lesson needs a plan name", line)
		}
		if subject == "" {
			return nil, fmt.Errorf("line %d: %s needs a subject", line, planName)
		}
		if title == "" {
			return nil, fmt.Errorf("line %d: a lesson in %s needs a title", line, planName)
		}
		k := key{planName, subject}
		pos, ok := seen[k]
		if !ok {
			plans = append(plans, importPlan{Name: planName, Subject: subject})
			pos = len(plans) - 1
			seen[k] = pos
		}
		item := importItem{
			Title:   title,
			Notes:   csvField(row, idx, "notes"),
			Week:    csvInt(row, idx, "week"),
			Minutes: csvInt(row, idx, "minutes"),
		}
		plans[pos].Items = append(plans[pos].Items, item)
	}
	return plans, nil
}

func csvHeaderIndex(header []string) map[string]int {
	out := map[string]int{}
	for i, col := range header {
		name := strings.ToLower(strings.TrimSpace(col))
		if name == "" {
			continue
		}
		out[name] = i
	}
	return out
}

func csvField(row []string, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func csvInt(row []string, idx map[string]int, name string) int {
	raw := csvField(row, idx, name)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func resolveImportPlans(drafts []importPlan, subjects []Subject) ([]CurriculumPlan, error) {
	byKey := map[string]Subject{}
	for _, s := range subjects {
		byKey[strings.ToLower(s.Name)] = s
		byKey[strings.ToLower(s.Slug)] = s
	}

	out := make([]CurriculumPlan, 0, len(drafts))
	for _, d := range drafts {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			return nil, fmt.Errorf("a plan needs a name")
		}
		sub, ok := byKey[strings.ToLower(strings.TrimSpace(d.Subject))]
		if !ok {
			return nil, fmt.Errorf("%s: unknown subject %q", name, strings.TrimSpace(d.Subject))
		}
		if len(d.Items) == 0 {
			return nil, fmt.Errorf("%s has no lessons", name)
		}
		plan := CurriculumPlan{
			Name:      name,
			SubjectID: sub.ID,
			Kind:      PlanAuthored,
			Notes:     strings.TrimSpace(d.Notes),
		}
		for i, it := range d.Items {
			title := strings.TrimSpace(it.Title)
			if title == "" {
				return nil, fmt.Errorf("%s: lesson %d needs a title", name, i+1)
			}
			plan.Items = append(plan.Items, CurriculumItem{
				Title:      title,
				Notes:      strings.TrimSpace(it.Notes),
				Minutes:    it.Minutes,
				WeekNumber: it.Week,
				SortOrder:  i + 1,
			})
		}
		out = append(out, plan)
	}
	return out, nil
}
