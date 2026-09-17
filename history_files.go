package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// gcHistoryFiles removes immutable upload bytes only when neither live data,
// retained history, nor legacy lesson trash can restore a reference to them.
func (a *App) gcHistoryFiles() error {
	a.filesMu.Lock()
	defer a.filesMu.Unlock()
	keep := map[string]bool{}
	add := func(path string) {
		path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
		if path != "." && path != "" && !strings.HasPrefix(path, "../") {
			keep[path] = true
		}
	}

	rows, err := a.store.db().Query(`SELECT stored_path FROM attachments
		UNION SELECT avatar_path FROM kids WHERE avatar_path <> ''
		UNION SELECT avatar_path FROM adults WHERE avatar_path <> ''`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		add(path)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	rows, err = a.store.db().Query(`SELECT before_json,after_json FROM history_changes
		UNION ALL SELECT attachments_json,NULL FROM deleted_lessons`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var before, after sql.NullString
		if err := rows.Scan(&before, &after); err != nil {
			rows.Close()
			return err
		}
		collectStoredPaths(before.String, add)
		collectStoredPaths(after.String, add)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	return filepath.Walk(a.uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(a.uploadDir, path)
		if err != nil {
			return err
		}
		if !keep[filepath.ToSlash(rel)] {
			return os.Remove(path)
		}
		return nil
	})
}

func collectStoredPaths(raw string, add func(string)) {
	if raw == "" {
		return
	}
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, item := range x {
				if key == "stored_path" || key == "StoredPath" {
					if path, ok := item.(string); ok {
						add(path)
					}
				}
				walk(item)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(value)
}
