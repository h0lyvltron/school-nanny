package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const historyLimit = 128

var (
	errNoUndo          = errors.New("nothing to undo")
	errNoRedo          = errors.New("nothing to redo")
	errHistoryConflict = errors.New("history changed in another tab")
)

type HistoryNode struct {
	ID               int64
	ParentID         int64
	PreferredChildID int64
	Label            string
	Route            string
	CreatedAt        string
	VisitedAt        string
	Status           string
	Children         []*HistoryNode
	Current          bool
	Depth            int
	BranchPrefix     string
}

func (n HistoryNode) CreatedLabel() string {
	when, err := time.Parse(time.RFC3339Nano, n.CreatedAt)
	if err != nil {
		return n.CreatedAt
	}
	return when.Local().Format("Jan 2, 3:04 PM")
}

type HistoryPosition struct {
	CurrentID int64
	Revision  int64
	CanUndo   bool
	CanRedo   bool
	UndoLabel string
	RedoLabel string
}

type historyResponseBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newHistoryResponseBuffer() *historyResponseBuffer {
	return &historyResponseBuffer{header: make(http.Header), status: http.StatusOK}
}

func (w *historyResponseBuffer) Header() http.Header {
	return w.header
}

func (w *historyResponseBuffer) WriteHeader(status int) {
	if w.status == http.StatusOK {
		w.status = status
	}
}

func (w *historyResponseBuffer) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func (w *historyResponseBuffer) flushTo(dst http.ResponseWriter) {
	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	dst.WriteHeader(w.status)
	_, _ = dst.Write(w.body.Bytes())
}

type historyTable struct {
	columns []string
}

var historyTables = map[string]historyTable{
	"lessons":             {[]string{"id", "kid_id", "adult_id", "subject_id", "school_year_id", "series_id", "assignment_id", "sequence", "scheduled_on", "status", "title", "minutes", "notes", "completed_at", "created_at", "page_start", "page_end", "curriculum_item_id"}},
	"lesson_series":       {[]string{"id", "kid_id", "subject_id", "school_year_id", "title", "minutes", "notes", "weekdays", "starts_on", "ends_on", "occurrence_count", "created_at"}},
	"plan_assignments":    {[]string{"id", "kid_id", "subject_id", "school_year_id", "plan_id", "name", "weekdays", "starts_on", "created_at"}},
	"curriculum_plans":    {[]string{"id", "name", "subject_id", "kind", "source_kid_id", "source_year_id", "notes", "created_at"}},
	"curriculum_items":    {[]string{"id", "plan_id", "sort_order", "title", "notes", "minutes", "week_number", "created_at", "page_start", "page_end"}},
	"attendance":          {[]string{"id", "kid_id", "attended_on", "status", "notes", "created_at"}},
	"assessments":         {[]string{"id", "kid_id", "subject_id", "lesson_id", "school_year_id", "given_on", "name", "score", "max_score", "letter", "notes", "created_at"}},
	"notes":               {[]string{"id", "kid_id", "adult_id", "subject_id", "noted_on", "body", "created_at"}},
	"attachments":         {[]string{"id", "owner_type", "lesson_id", "assessment_id", "kid_id", "subject_id", "original_name", "stored_path", "size_bytes", "content_type", "created_at", "curriculum_plan_id"}},
	"adult_events":        {[]string{"id", "adult_id", "starts_on", "ends_on", "title", "body", "created_at", "label_id"}},
	"adult_event_labels":  {[]string{"id", "adult_id", "name", "color", "sort_order", "created_at", "emoji"}},
	"adult_holiday_notes": {[]string{"id", "adult_id", "observed_on", "holiday_name", "emoji", "notes", "label_id"}},
}

var historyRouteLabels = map[string]string{
	"POST /attendance":                            "Update attendance",
	"POST /curriculum":                            "Create curriculum",
	"POST /curriculum/import":                     "Import curriculum",
	"POST /curriculum/from-toc":                   "Import curriculum outline",
	"POST /curriculum/from-pdf":                   "Import curriculum PDF",
	"POST /curriculum/schedule-item":              "Schedule curriculum lesson",
	"POST /curriculum/{id}":                       "Update curriculum",
	"POST /curriculum/{id}/delete":                "Delete curriculum",
	"POST /curriculum/{id}/items":                 "Add curriculum item",
	"POST /curriculum/{id}/items/{itemID}":        "Update curriculum item",
	"POST /curriculum/{id}/items/{itemID}/delete": "Delete curriculum item",
	"POST /curriculum/{id}/items/{itemID}/move":   "Reorder curriculum",
	"POST /curriculum/{id}/apply":                 "Schedule curriculum",
	"POST /archive/export":                        "Create curriculum from archive",
	"POST /lessons":                               "Add lesson",
	"POST /lessons/restore":                       "Restore deleted lesson",
	"POST /lessons/{id}":                          "Update lesson",
	"POST /lessons/{id}/status":                   "Change lesson status",
	"POST /lessons/{id}/reschedule":               "Reschedule lesson",
	"POST /lessons/{id}/clone":                    "Clone lesson",
	"POST /lessons/{id}/delete":                   "Delete lesson",
	"POST /lessons/{id}/delete-future":            "Delete future lessons",
	"POST /series/{id}":                           "Update lesson series",
	"POST /series/{id}/stop":                      "Stop lesson series",
	"POST /assignments/{id}":                      "Update curriculum schedule",
	"POST /assignments/{id}/stop":                 "Stop curriculum schedule",
	"POST /assignments/{id}/pause":                "Pause curriculum schedule",
	"POST /assignments/{id}/vacation":             "Shift around vacation",
	"POST /lessons/{id}/push":                     "Push remaining lessons",
	"POST /lessons/{id}/pull":                     "Pull remaining lessons",
	"POST /lessons/{id}/double-up":                "Double up lessons",
	"POST /lessons/{id}/shift":                    "Shift lessons",
	"POST /adults/{id}/schedule":                  "Add adult lesson",
	"POST /adults/{id}/calendar/import":           "Import calendar events",
	"POST /adults/{id}/events":                    "Add calendar event",
	"POST /adults/{id}/events/{eventID}":          "Update calendar event",
	"POST /adults/{id}/events/{eventID}/label":    "Label calendar event",
	"POST /adults/{id}/events/{eventID}/delete":   "Delete calendar event",
	"POST /adults/{id}/labels":                    "Add calendar label",
	"POST /adults/{id}/labels/{labelID}":          "Update calendar label",
	"POST /adults/{id}/labels/{labelID}/delete":   "Delete calendar label",
	"POST /adults/{id}/holidays":                  "Update holiday",
	"POST /assessments":                           "Save assessment",
	"POST /assessments/{id}/delete":               "Delete assessment",
	"POST /notes":                                 "Add note",
	"POST /notes/{id}/delete":                     "Delete note",
	"POST /files":                                 "Upload attachment",
	"POST /files/{id}/delete":                     "Delete attachment",
}

func (s *Store) RecoverHistory() error {
	var exists int
	if err := s.db().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='history_nodes'`).Scan(&exists); err != nil || exists == 0 {
		return err
	}
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE history_context SET node_id = NULL WHERE id = 1`); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT n.id, COUNT(c.id)
		FROM history_nodes n LEFT JOIN history_changes c ON c.node_id = n.id
		WHERE n.status = 'pending' GROUP BY n.id`)
	if err != nil {
		return err
	}
	var complete, empty []int64
	for rows.Next() {
		var id, count int64
		if err := rows.Scan(&id, &count); err != nil {
			rows.Close()
			return err
		}
		if count == 0 {
			empty = append(empty, id)
		} else {
			complete = append(complete, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range empty {
		if _, err := tx.Exec(`DELETE FROM history_nodes WHERE id = ?`, id); err != nil {
			return err
		}
	}
	for _, id := range complete {
		if _, err := tx.Exec(`UPDATE history_nodes SET status='complete' WHERE id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE history_state SET current_node_id=?, revision=revision+1 WHERE id=1`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) beginHistory(label, route string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var parent sql.NullInt64
	if err := tx.QueryRow(`SELECT current_node_id FROM history_state WHERE id=1`).Scan(&parent); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`INSERT INTO history_nodes(parent_id,label,route,created_at,visited_at,status)
		VALUES(?,?,?,?,?,'pending')`, nullableNullInt(parent), label, route, now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE history_context SET node_id=? WHERE id=1`, id); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func nullableNullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func (s *Store) finishHistory(id int64) (changed, pruned bool, err error) {
	tx, err := s.db().Begin()
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE history_context SET node_id=NULL WHERE id=1 AND node_id=?`, id); err != nil {
		return false, false, err
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM history_changes WHERE node_id=?`, id).Scan(&count); err != nil {
		return false, false, err
	}
	if count == 0 {
		if _, err := tx.Exec(`DELETE FROM history_nodes WHERE id=?`, id); err != nil {
			return false, false, err
		}
		return false, false, tx.Commit()
	}
	var parent sql.NullInt64
	if err := tx.QueryRow(`SELECT parent_id FROM history_nodes WHERE id=?`, id).Scan(&parent); err != nil {
		return false, false, err
	}
	if _, err := tx.Exec(`UPDATE history_nodes SET status='complete', visited_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return false, false, err
	}
	if parent.Valid {
		if _, err := tx.Exec(`UPDATE history_nodes SET preferred_child_id=? WHERE id=?`, id, parent.Int64); err != nil {
			return false, false, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE history_state SET preferred_root_id=? WHERE id=1`, id); err != nil {
			return false, false, err
		}
	}
	if _, err := tx.Exec(`UPDATE history_state SET current_node_id=?, revision=revision+1 WHERE id=1`, id); err != nil {
		return false, false, err
	}
	if err := tx.Commit(); err != nil {
		return false, false, err
	}
	pruned, err = s.pruneHistory()
	return true, pruned, err
}

func (s *Store) discardHistory(id int64) {
	_, _ = s.db().Exec(`UPDATE history_context SET node_id=NULL WHERE id=1 AND node_id=?`, id)
	_, _ = s.db().Exec(`DELETE FROM history_nodes WHERE id=? AND status='pending'`, id)
}

func (s *Store) HistoryPosition() (HistoryPosition, error) {
	var p HistoryPosition
	var current, root sql.NullInt64
	if err := s.db().QueryRow(`SELECT current_node_id, preferred_root_id, revision FROM history_state WHERE id=1`).
		Scan(&current, &root, &p.Revision); err != nil {
		return p, err
	}
	if current.Valid {
		p.CurrentID = current.Int64
		p.CanUndo = true
		_ = s.db().QueryRow(`SELECT label FROM history_nodes WHERE id=?`, current.Int64).Scan(&p.UndoLabel)
		var child sql.NullInt64
		if err := s.db().QueryRow(`SELECT preferred_child_id FROM history_nodes WHERE id=?`, current.Int64).Scan(&child); err == nil && child.Valid {
			p.CanRedo = true
			_ = s.db().QueryRow(`SELECT label FROM history_nodes WHERE id=?`, child.Int64).Scan(&p.RedoLabel)
		}
	} else if root.Valid {
		p.CanRedo = true
		_ = s.db().QueryRow(`SELECT label FROM history_nodes WHERE id=?`, root.Int64).Scan(&p.RedoLabel)
	}
	return p, nil
}

func (s *Store) checkExpected(tx *sql.Tx, expected, expectedRevision int64) (sql.NullInt64, error) {
	var current sql.NullInt64
	var revision int64
	if err := tx.QueryRow(`SELECT current_node_id,revision FROM history_state WHERE id=1`).
		Scan(&current, &revision); err != nil {
		return current, err
	}
	got := int64(0)
	if current.Valid {
		got = current.Int64
	}
	if got != expected || (expectedRevision >= 0 && revision != expectedRevision) {
		return current, errHistoryConflict
	}
	return current, nil
}

func (s *Store) Undo(expected int64) error {
	return s.UndoAt(expected, -1)
}

func (s *Store) UndoAt(expected, expectedRevision int64) error {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := s.checkExpected(tx, expected, expectedRevision)
	if err != nil {
		return err
	}
	if !current.Valid {
		return errNoUndo
	}
	var parent sql.NullInt64
	if err := tx.QueryRow(`SELECT parent_id FROM history_nodes WHERE id=?`, current.Int64).Scan(&parent); err != nil {
		return err
	}
	if err := applyHistoryNode(tx, current.Int64, false); err != nil {
		return err
	}
	if parent.Valid {
		if _, err := tx.Exec(`UPDATE history_nodes SET preferred_child_id=?, visited_at=? WHERE id=?`,
			current.Int64, time.Now().UTC().Format(time.RFC3339Nano), parent.Int64); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE history_state SET preferred_root_id=? WHERE id=1`, current.Int64); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE history_state SET current_node_id=?, revision=revision+1 WHERE id=1`, nullableNullInt(parent))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Redo(expected int64) error {
	return s.RedoAt(expected, -1)
}

func (s *Store) RedoAt(expected, expectedRevision int64) error {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := s.checkExpected(tx, expected, expectedRevision)
	if err != nil {
		return err
	}
	var child sql.NullInt64
	if current.Valid {
		err = tx.QueryRow(`SELECT preferred_child_id FROM history_nodes WHERE id=?`, current.Int64).Scan(&child)
	} else {
		err = tx.QueryRow(`SELECT preferred_root_id FROM history_state WHERE id=1`).Scan(&child)
	}
	if err != nil {
		return err
	}
	if !child.Valid {
		return errNoRedo
	}
	if err := applyHistoryNode(tx, child.Int64, true); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE history_nodes SET visited_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339Nano), child.Int64)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE history_state SET current_node_id=?, revision=revision+1 WHERE id=1`, child.Int64)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Checkout(expected, target int64) error {
	return s.CheckoutAt(expected, -1, target)
}

func (s *Store) CheckoutAt(expected, expectedRevision, target int64) error {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := s.checkExpected(tx, expected, expectedRevision)
	if err != nil {
		return err
	}
	if target == 0 {
		return fmt.Errorf("invalid history target")
	}
	if current.Valid && current.Int64 == target {
		return nil
	}
	currentPath, err := historyAncestors(tx, nullableNullInt(current))
	if err != nil {
		return err
	}
	targetPath, err := historyAncestors(tx, target)
	if err != nil {
		return err
	}
	targetSet := map[int64]int{}
	for i, id := range targetPath {
		targetSet[id] = i
	}
	lca := int64(0)
	for _, id := range currentPath {
		if _, ok := targetSet[id]; ok {
			lca = id
			break
		}
	}
	for _, id := range currentPath {
		if id == lca {
			break
		}
		if err := applyHistoryNode(tx, id, false); err != nil {
			return err
		}
	}
	var forward []int64
	for _, id := range targetPath {
		if id == lca {
			break
		}
		forward = append(forward, id)
	}
	for i := len(forward) - 1; i >= 0; i-- {
		id := forward[i]
		if err := applyHistoryNode(tx, id, true); err != nil {
			return err
		}
		var parent sql.NullInt64
		if err := tx.QueryRow(`SELECT parent_id FROM history_nodes WHERE id=?`, id).Scan(&parent); err != nil {
			return err
		}
		if parent.Valid {
			_, _ = tx.Exec(`UPDATE history_nodes SET preferred_child_id=? WHERE id=?`, id, parent.Int64)
		} else {
			_, _ = tx.Exec(`UPDATE history_state SET preferred_root_id=? WHERE id=1`, id)
		}
	}
	_, err = tx.Exec(`UPDATE history_nodes SET visited_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339Nano), target)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE history_state SET current_node_id=?, revision=revision+1 WHERE id=1`, target)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func historyAncestors(tx *sql.Tx, start any) ([]int64, error) {
	if start == nil {
		return nil, nil
	}
	rows, err := tx.Query(`WITH RECURSIVE chain(id,parent_id) AS (
		SELECT id,parent_id FROM history_nodes WHERE id=?
		UNION ALL SELECT n.id,n.parent_id FROM history_nodes n JOIN chain c ON n.id=c.parent_id
	) SELECT id FROM chain`, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type historyChange struct {
	ID        int64
	Table     string
	RowID     int64
	Operation string
	Before    sql.NullString
	After     sql.NullString
}

func applyHistoryNode(tx *sql.Tx, nodeID int64, forward bool) error {
	order := "DESC"
	if forward {
		order = "ASC"
	}
	rows, err := tx.Query(`SELECT id,table_name,row_id,operation,before_json,after_json
		FROM history_changes WHERE node_id=? ORDER BY id `+order, nodeID)
	if err != nil {
		return err
	}
	var changes []historyChange
	for rows.Next() {
		var c historyChange
		if err := rows.Scan(&c.ID, &c.Table, &c.RowID, &c.Operation, &c.Before, &c.After); err != nil {
			rows.Close()
			return err
		}
		changes = append(changes, c)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, c := range changes {
		if err := applyHistoryChange(tx, c, forward); err != nil {
			return fmt.Errorf("%s history %s row %d: %w", c.Table, c.Operation, c.RowID, err)
		}
	}
	return nil
}

func applyHistoryChange(tx DBTX, c historyChange, forward bool) error {
	table, ok := historyTables[c.Table]
	if !ok {
		return fmt.Errorf("unknown history table")
	}
	op := c.Operation
	raw := c.After
	if !forward {
		raw = c.Before
		switch op {
		case "insert":
			op = "delete"
		case "delete":
			op = "insert"
		}
	}
	if op == "delete" {
		_, err := tx.Exec(`DELETE FROM `+c.Table+` WHERE id=?`, c.RowID)
		return err
	}
	if !raw.Valid {
		return fmt.Errorf("missing row snapshot")
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(raw.String), &row); err != nil {
		return err
	}
	if op == "insert" {
		marks := make([]string, len(table.columns))
		args := make([]any, len(table.columns))
		for i, col := range table.columns {
			marks[i] = "?"
			args[i] = row[col]
		}
		_, err := tx.Exec(`INSERT INTO `+c.Table+` (`+strings.Join(table.columns, ",")+`)
			VALUES (`+strings.Join(marks, ",")+`)`, args...)
		return err
	}
	sets := make([]string, 0, len(table.columns)-1)
	args := make([]any, 0, len(table.columns))
	for _, col := range table.columns {
		if col == "id" {
			continue
		}
		sets = append(sets, col+"=?")
		args = append(args, row[col])
	}
	args = append(args, c.RowID)
	_, err := tx.Exec(`UPDATE `+c.Table+` SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	return err
}

func (s *Store) HistoryTree() ([]*HistoryNode, HistoryPosition, error) {
	pos, err := s.HistoryPosition()
	if err != nil {
		return nil, pos, err
	}
	rows, err := s.db().Query(`SELECT id,COALESCE(parent_id,0),COALESCE(preferred_child_id,0),
		label,route,created_at,visited_at,status FROM history_nodes
		WHERE status='complete' ORDER BY id`)
	if err != nil {
		return nil, pos, err
	}
	defer rows.Close()
	byID := map[int64]*HistoryNode{}
	var ordered []*HistoryNode
	for rows.Next() {
		n := &HistoryNode{}
		if err := rows.Scan(&n.ID, &n.ParentID, &n.PreferredChildID, &n.Label,
			&n.Route, &n.CreatedAt, &n.VisitedAt, &n.Status); err != nil {
			return nil, pos, err
		}
		n.Current = n.ID == pos.CurrentID
		byID[n.ID] = n
		ordered = append(ordered, n)
	}
	var roots []*HistoryNode
	for _, n := range ordered {
		if parent := byID[n.ParentID]; parent != nil {
			parent.Children = append(parent.Children, n)
		} else {
			roots = append(roots, n)
		}
	}
	return roots, pos, rows.Err()
}

func (s *Store) pruneHistory() (bool, error) {
	var current sql.NullInt64
	if err := s.db().QueryRow(`SELECT current_node_id FROM history_state WHERE id=1`).Scan(&current); err != nil || !current.Valid {
		return false, err
	}
	path, err := historyAncestorsDB(s.db(), current.Int64)
	if err != nil || len(path) <= historyLimit {
		return false, err
	}
	cutoff := path[historyLimit-1]
	tx, err := s.db().Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE history_nodes SET parent_id=NULL WHERE id=?`, cutoff); err != nil {
		return false, err
	}
	// Once cutoff becomes the sole retained root, every other root is either
	// old ancestry or a branch from the virtual root beyond the undo window.
	if _, err := tx.Exec(`WITH RECURSIVE doomed(id) AS (
		SELECT id FROM history_nodes WHERE parent_id IS NULL AND id<>?
		UNION ALL
		SELECT n.id FROM history_nodes n JOIN doomed d ON n.parent_id=d.id
	) DELETE FROM history_nodes WHERE id IN (SELECT id FROM doomed)`, cutoff); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ClearHistory is used after an out-of-scope structural delete. Such a delete
// can remove parent rows required by retained planner snapshots, so keeping
// those branches would promise a restore that foreign keys cannot perform.
func (s *Store) ClearHistory() error {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	tx, err := s.db().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE history_context SET node_id=NULL WHERE id=1`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM history_nodes`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE history_state
		SET current_node_id=NULL, preferred_root_id=NULL, revision=revision+1 WHERE id=1`); err != nil {
		return err
	}
	return tx.Commit()
}

type queryRower interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func historyAncestorsDB(q queryRower, start int64) ([]int64, error) {
	rows, err := q.Query(`WITH RECURSIVE chain(id,parent_id) AS (
		SELECT id,parent_id FROM history_nodes WHERE id=?
		UNION ALL SELECT n.id,n.parent_id FROM history_nodes n JOIN chain c ON n.id=c.parent_id
	) SELECT id FROM chain`, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func parseExpected(r *http.Request) int64 {
	_ = r.ParseForm()
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("expected")), 10, 64)
	return id
}

func parseExpectedRevision(r *http.Request) int64 {
	_ = r.ParseForm()
	revision, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("expected_revision")), 10, 64)
	if err != nil {
		return -1
	}
	return revision
}

// historyFlat returns newest-first nodes for compact template rendering.
func historyFlat(roots []*HistoryNode) []*HistoryNode {
	var out []*HistoryNode
	var walk func(*HistoryNode, int)
	walk = func(n *HistoryNode, depth int) {
		n.Depth = depth
		n.BranchPrefix = strings.Repeat("↳ ", depth)
		out = append(out, n)
		for _, child := range n.Children {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	return out
}
