package main

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// dateLayout is how every date is stored, so string comparison sorts correctly.
const dateLayout = "2006-01-02"

// dbFileName is the database inside the data folder.
const dbFileName = "school.db"

// Store owns the SQLite connection and all queries against it.
//
// The pool is guarded rather than final because restoring a backup swaps the
// database file underneath a running app: the only safe way to do that is to
// close every connection, replace the file, and open it again.
type Store struct {
	mu   sync.RWMutex
	pool *sql.DB
	path string
}

func OpenStore(dbPath string) (*Store, error) {
	pool, err := openPool(dbPath)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, path: dbPath}, nil
}

func openPool(dbPath string) (*sql.DB, error) {
	dsn := "file:" + url.PathEscape(dbPath) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"

	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", dbPath, err)
	}
	// SQLite handles one writer at a time; a small pool avoids lock churn.
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	if err := pool.Ping(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("opening %s: %w", dbPath, err)
	}
	return pool, nil
}

// db hands out the current pool. Every query goes through it so that a restore
// can put a different one in place.
func (s *Store) db() *sql.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pool
}

// Path is the database file, which the backup code needs to name its copies.
func (s *Store) Path() string {
	return s.path
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pool.Close()
}

// SnapshotTo writes a consistent copy of the database to dest, safe to run
// while the app is in use. VACUUM INTO folds the write-ahead log into the copy,
// which plain file copying would miss, and produces a single self-contained
// file with no -wal or -shm sidecar to keep track of.
func (s *Store) SnapshotTo(dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	// VACUUM INTO refuses to overwrite, so a half-written file from a failed
	// run would block every later one.
	if err := os.Remove(dest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := s.db().Exec(`VACUUM INTO ?`, dest); err != nil {
		return fmt.Errorf("copying the database to %s: %w", dest, err)
	}
	return nil
}

// RestoreFrom replaces the live database with a backup.
//
// The current database is snapshotted to safetyCopy first, so a restore of the
// wrong file is itself undoable. Nothing is touched until the backup has been
// opened and checked, and the app keeps running throughout: connections are
// closed, the file is swapped, and a fresh pool takes its place.
func (s *Store) RestoreFrom(src, safetyCopy string) error {
	if err := checkRestorable(src); err != nil {
		return err
	}
	if err := s.swapIn(src, safetyCopy); err != nil {
		return err
	}
	// A backup taken by an older version may predate the newest migrations.
	return s.Migrate()
}

func (s *Store) swapIn(src, safetyCopy string) error {
	// Writing the safety copy over the file being restored would destroy both
	// the backup and the records it was there to protect. Callers pick a name
	// that does not exist, so this only guards against a future caller.
	if safetyCopy != "" && samePath(src, safetyCopy) {
		return fmt.Errorf("refusing to save the current records over the backup being restored")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if safetyCopy != "" {
		if err := os.MkdirAll(filepath.Dir(safetyCopy), 0o755); err != nil {
			return err
		}
		if err := os.Remove(safetyCopy); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := s.pool.Exec(`VACUUM INTO ?`, safetyCopy); err != nil {
			return fmt.Errorf("saving the current records before restoring: %w", err)
		}
	}

	if err := s.pool.Close(); err != nil {
		return fmt.Errorf("closing the database: %w", err)
	}

	// The sidecars belong to the database being replaced. Leaving them behind
	// would let a stale write-ahead log graft itself onto the restored file.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(s.path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.reopenAfterFailure()
			return err
		}
	}
	if err := copyFile(src, s.path); err != nil {
		s.reopenAfterFailure()
		return fmt.Errorf("putting the backup in place: %w", err)
	}

	pool, err := openPool(s.path)
	if err != nil {
		return err
	}
	s.pool = pool
	return nil
}

// reopenAfterFailure gets the app working again when a restore falls over
// partway. Callers already have an error to report, so a failure here only
// means the app cannot recover on its own and has to be restarted.
func (s *Store) reopenAfterFailure() {
	pool, err := openPool(s.path)
	if err != nil {
		log.Printf("could not reopen the database after a failed restore: %v", err)
		return
	}
	s.pool = pool
}

// checkRestorable makes sure a file really is one of this app's backups before
// it is allowed to replace the live records.
func checkRestorable(path string) error {
	pool, err := openPool(path)
	if err != nil {
		return fmt.Errorf("that backup could not be opened: %w", err)
	}
	defer pool.Close()

	var tables int
	err = pool.QueryRow(`SELECT count(*) FROM sqlite_master
		WHERE type = 'table' AND name IN ('kids', 'subjects', 'lessons', 'schema_migrations')`).Scan(&tables)
	if err != nil {
		return fmt.Errorf("that backup could not be read: %w", err)
	}
	if tables < 4 {
		return errNotABackup
	}
	return nil
}

var errNotABackup = errors.New("that file is not a School Nanny backup")

// Migrate applies every embedded migration that has not run yet, each in its
// own transaction, in filename order.
func (s *Store) Migrate() error {
	if _, err := s.db().Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}

	applied := map[int]bool{}
	rows, err := s.db().Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if applied[version] {
			continue
		}
		body, err := migrationFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return err
		}
		tx, err := s.db().Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q must be named like 0001_description.sql", name)
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration %q must be named like 0001_description.sql", name)
	}
	return v, nil
}

// Setting reads a single settings row, returning "" when it is absent.
func (s *Store) Setting(key string) (string, error) {
	var value string
	err := s.db().QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db().Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) DeleteSetting(key string) error {
	_, err := s.db().Exec(`DELETE FROM settings WHERE key = ?`, key)
	return err
}
