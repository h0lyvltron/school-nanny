package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const historyLabelMax = 100

var historyBareLabels = func() map[string]bool {
	out := make(map[string]bool, len(historyRouteLabels))
	for _, label := range historyRouteLabels {
		out[label] = true
	}
	return out
}()

func (s *Store) enrichHistoryLabel(tx *sql.Tx, nodeID int64) error {
	var label string
	if err := tx.QueryRow(`SELECT label FROM history_nodes WHERE id=?`, nodeID).Scan(&label); err != nil {
		return err
	}
	changes, err := loadHistoryChanges(tx, nodeID)
	if err != nil {
		return err
	}
	enriched := summarizeHistoryLabel(tx, label, changes)
	if enriched == label || enriched == "" {
		return nil
	}
	_, err = tx.Exec(`UPDATE history_nodes SET label=? WHERE id=?`, enriched, nodeID)
	return err
}

func loadHistoryChanges(tx DBTX, nodeID int64) ([]historyChange, error) {
	rows, err := tx.Query(`SELECT id,table_name,row_id,operation,before_json,after_json
		FROM history_changes WHERE node_id=? ORDER BY id`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var changes []historyChange
	for rows.Next() {
		var c historyChange
		if err := rows.Scan(&c.ID, &c.Table, &c.RowID, &c.Operation, &c.Before, &c.After); err != nil {
			return nil, err
		}
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

func summarizeHistoryLabel(tx DBTX, verb string, changes []historyChange) string {
	verb = strings.TrimSpace(verb)
	if verb == "" || len(changes) == 0 {
		return verb
	}
	names := map[string]string{}
	subject := historySubject(tx, changes, names)
	if subject == "" {
		return verb
	}
	return truncateHistoryLabel(verb + " · " + subject)
}

func historySubject(tx DBTX, changes []historyChange, names map[string]string) string {
	byTable := map[string][]historyChange{}
	for _, c := range changes {
		byTable[c.Table] = append(byTable[c.Table], c)
	}
	if lessons := byTable["lessons"]; len(lessons) > 0 {
		return lessonHistorySubject(tx, lessons, names)
	}
	order := []string{
		"plan_assignments", "lesson_series", "curriculum_plans", "curriculum_items",
		"attendance", "assessments", "notes", "attachments",
		"adult_events", "adult_event_labels", "adult_holiday_notes",
	}
	for _, table := range order {
		rows := byTable[table]
		if len(rows) == 0 {
			continue
		}
		if subject := tableHistorySubject(tx, table, rows, names); subject != "" {
			return subject
		}
	}
	if len(changes) > 1 {
		return fmt.Sprintf("%d changes", len(changes))
	}
	return ""
}

func lessonHistorySubject(tx DBTX, lessons []historyChange, names map[string]string) string {
	if len(lessons) > 1 {
		parts := []string{fmt.Sprintf("%d lessons", len(lessons))}
		if row := historyRow(lessons[0]); row != nil {
			if person := historyPersonName(tx, row, names); person != "" {
				parts = append(parts, person)
			}
			if title := snapshotString(row, "title"); title != "" {
				parts = append(parts, title)
			}
		}
		return strings.Join(parts, " · ")
	}
	row := historyRow(lessons[0])
	if row == nil {
		return ""
	}
	return joinHistoryParts(
		historyPersonName(tx, row, names),
		snapshotString(row, "title"),
		historyShortDate(snapshotString(row, "scheduled_on")),
	)
}

func tableHistorySubject(tx DBTX, table string, rows []historyChange, names map[string]string) string {
	if len(rows) > 1 && (table == "curriculum_items" || table == "adult_events" || table == "attendance") {
		return fmt.Sprintf("%d %s", len(rows), historyTableNoun(table, len(rows)))
	}
	row := historyRow(rows[0])
	if row == nil {
		return ""
	}
	switch table {
	case "plan_assignments":
		return joinHistoryParts(
			historyPersonName(tx, row, names),
			snapshotString(row, "name"),
			historyShortDate(snapshotString(row, "starts_on")),
		)
	case "lesson_series":
		return joinHistoryParts(
			historyPersonName(tx, row, names),
			snapshotString(row, "title"),
			historyShortDate(snapshotString(row, "starts_on")),
		)
	case "curriculum_plans":
		return snapshotString(row, "name")
	case "curriculum_items":
		return snapshotString(row, "title")
	case "attendance":
		return joinHistoryParts(
			historyPersonName(tx, row, names),
			historyShortDate(snapshotString(row, "attended_on")),
			snapshotString(row, "status"),
		)
	case "assessments":
		score := historyScoreLabel(row)
		return joinHistoryParts(
			historyPersonName(tx, row, names),
			snapshotString(row, "name"),
			score,
		)
	case "notes":
		return joinHistoryParts(
			historyPersonName(tx, row, names),
			historySnippet(snapshotString(row, "body"), 40),
		)
	case "attachments":
		return snapshotString(row, "original_name")
	case "adult_events":
		return joinHistoryParts(
			historyAdultName(tx, row, names),
			snapshotString(row, "title"),
			historyShortDate(snapshotString(row, "starts_on")),
		)
	case "adult_event_labels":
		return snapshotString(row, "name")
	case "adult_holiday_notes":
		return joinHistoryParts(
			historyAdultName(tx, row, names),
			snapshotString(row, "holiday_name"),
			historyShortDate(snapshotString(row, "observed_on")),
		)
	}
	return ""
}

func historyTableNoun(table string, n int) string {
	switch table {
	case "curriculum_items":
		if n == 1 {
			return "curriculum item"
		}
		return "curriculum items"
	case "adult_events":
		if n == 1 {
			return "calendar event"
		}
		return "calendar events"
	case "attendance":
		return "attendance rows"
	default:
		return "changes"
	}
}

func historyRow(c historyChange) map[string]any {
	raw := c.After
	if c.Operation == "delete" || !raw.Valid {
		raw = c.Before
	}
	if !raw.Valid {
		return nil
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(raw.String), &row); err != nil {
		return nil
	}
	return row
}

func historyPersonName(tx DBTX, row map[string]any, names map[string]string) string {
	if kidID := snapshotInt(row, "kid_id"); kidID > 0 {
		return lookupHistoryName(tx, names, "kid", kidID, `SELECT name FROM kids WHERE id=?`)
	}
	if adultID := snapshotInt(row, "adult_id"); adultID > 0 {
		return lookupHistoryName(tx, names, "adult", adultID, `SELECT name FROM adults WHERE id=?`)
	}
	return ""
}

func historyAdultName(tx DBTX, row map[string]any, names map[string]string) string {
	if adultID := snapshotInt(row, "adult_id"); adultID > 0 {
		return lookupHistoryName(tx, names, "adult", adultID, `SELECT name FROM adults WHERE id=?`)
	}
	return ""
}

func lookupHistoryName(tx DBTX, names map[string]string, kind string, id int64, query string) string {
	key := kind + ":" + strconv.FormatInt(id, 10)
	if name, ok := names[key]; ok {
		return name
	}
	var name string
	if err := tx.QueryRow(query, id).Scan(&name); err != nil {
		names[key] = ""
		return ""
	}
	names[key] = name
	return name
}

func historyScoreLabel(row map[string]any) string {
	score := snapshotString(row, "score")
	max := snapshotString(row, "max_score")
	letter := snapshotString(row, "letter")
	switch {
	case score != "" && max != "":
		return score + "/" + max
	case score != "":
		return score
	case letter != "":
		return letter
	}
	return ""
}

func historyShortDate(value string) string {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return ""
	}
	return t.Format("Jan 2")
}

func historySnippet(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

func joinHistoryParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " · ")
}

func truncateHistoryLabel(s string) string {
	if utf8.RuneCountInString(s) <= historyLabelMax {
		return s
	}
	runes := []rune(s)
	return string(runes[:historyLabelMax-1]) + "…"
}

func snapshotString(row map[string]any, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func snapshotInt(row map[string]any, key string) int64 {
	v, ok := row[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}

func (s *Store) backfillHistoryLabels() error {
	rows, err := s.db().Query(`SELECT id, label FROM history_nodes WHERE status='complete'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type bare struct {
		id    int64
		label string
	}
	var todo []bare
	for rows.Next() {
		var id int64
		var label string
		if err := rows.Scan(&id, &label); err != nil {
			return err
		}
		if historyBareLabels[label] {
			todo = append(todo, bare{id, label})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range todo {
		tx, err := s.db().Begin()
		if err != nil {
			return err
		}
		if err := s.enrichHistoryLabel(tx, item.id); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func markPreferredHistoryPathFrom(byID map[int64]*HistoryNode, pos HistoryPosition, preferredRoot int64) {
	start := pos.CurrentID
	if start == 0 {
		start = preferredRoot
	}
	seen := map[int64]bool{}
	for id := start; id != 0 && !seen[id]; {
		seen[id] = true
		n := byID[id]
		if n == nil {
			break
		}
		n.OnPreferredPath = true
		id = n.PreferredChildID
	}
	for id := start; id != 0 && byID[id] != nil; {
		n := byID[id]
		n.OnPreferredPath = true
		if n.ParentID == 0 || seen[n.ParentID] {
			break
		}
		seen[n.ParentID] = true
		id = n.ParentID
	}
}

func loadHistoryChangeCounts(db DBTX) (map[int64]int, error) {
	rows, err := db.Query(`SELECT node_id, COUNT(*) FROM history_changes GROUP BY node_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		out[id] = count
	}
	return out, rows.Err()
}
