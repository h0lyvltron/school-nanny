package main

// Backups of the family's records.
//
// A backup is a single self-contained SQLite file in the "backups" folder next
// to the live database. One is taken automatically on the first start of each
// day, and restoring one keeps a copy of what it replaced, so no single action
// here can be the last word on anything.
//
// Attached files are not in these snapshots: they are ordinary files in
// "uploads", and copying the whole data folder is what captures those. The
// Settings page says so next to the folder path.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	backupsFolderName   = "backups"
	backupStamp         = "2006-01-02-150405"
	beforeRestoreSuffix = "-before-restore.db"
	// keepBackups is generous: the files are small next to the attachments
	// they sit beside, and the whole point is to still have yesterday's.
	keepBackups = 20
)

// backupPattern is also the guard against a crafted name reaching the
// filesystem, so it allows nothing but the shapes the app writes. The middle
// group is the counter that keeps two backups in the same second apart.
var backupPattern = regexp.MustCompile(`^school-\d{4}-\d{2}-\d{2}-\d{6}(-\d+)?(-before-restore)?\.db$`)

// Backup is one snapshot on disk.
type Backup struct {
	Name  string
	Taken time.Time
	Size  int64
}

// BeforeRestore marks the automatic copy taken of the records a restore was
// about to replace, which is the one to reach for after restoring the wrong file.
func (b Backup) BeforeRestore() bool {
	return strings.HasSuffix(b.Name, beforeRestoreSuffix)
}

func (b Backup) SizeLabel() string { return sizeLabel(b.Size) }

// TakenLabel reads as a moment rather than a filename. It goes down to seconds
// because several copies can land in the same minute while someone is trying
// restores, which is exactly when telling them apart matters.
func (b Backup) TakenLabel() string {
	return b.Taken.Format("Mon, Jan 2") + " at " + b.Taken.Format("3:04:05 pm")
}

func (a *App) backupsDir() string {
	return filepath.Join(a.dataDir, backupsFolderName)
}

// MakeBackup writes a new snapshot and prunes the oldest.
func (a *App) MakeBackup() (Backup, error) {
	return a.makeBackup(time.Now(), false)
}

func (a *App) makeBackup(when time.Time, beforeRestore bool) (Backup, error) {
	path := a.freeBackupPath(when, beforeRestore)
	name := filepath.Base(path)

	if err := a.store.SnapshotTo(path); err != nil {
		return Backup{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Backup{}, err
	}
	if err := a.pruneBackups(); err != nil {
		return Backup{}, err
	}
	return Backup{Name: name, Taken: when, Size: info.Size()}, nil
}

// Backups lists the snapshots newest first.
func (a *App) Backups() ([]Backup, error) {
	entries, err := os.ReadDir(a.backupsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var backups []Backup
	for _, entry := range entries {
		if entry.IsDir() || !backupPattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		taken, err := backupTime(entry.Name())
		if err != nil {
			continue
		}
		backups = append(backups, Backup{Name: entry.Name(), Taken: taken, Size: info.Size()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Taken.After(backups[j].Taken) })
	return backups, nil
}

// backupTime reads the moment out of the filename rather than the file's
// timestamp, which copying between folders would rewrite.
func backupTime(name string) (time.Time, error) {
	stamp := name[len("school-"):]
	stamp = stamp[:len(backupStamp)]
	return time.ParseInLocation(backupStamp, stamp, time.Local)
}

// RestoreBackup replaces the live records with the named snapshot.
func (a *App) RestoreBackup(name string) error {
	path, err := a.backupPath(name)
	if err != nil {
		return err
	}
	return a.store.RestoreFrom(path, a.freeBackupPath(time.Now(), true))
}

// freeBackupPath names a backup that does not exist yet. Two backups within the
// same second would otherwise share a name, and the second one would quietly
// overwrite the first -- including, when restoring twice in quick succession,
// the very file being restored from.
func (a *App) freeBackupPath(when time.Time, beforeRestore bool) string {
	for counter := 0; ; counter++ {
		path := filepath.Join(a.backupsDir(), backupName(when, counter, beforeRestore))
		if !fileExists(path) {
			return path
		}
	}
}

func backupName(when time.Time, counter int, beforeRestore bool) string {
	name := "school-" + when.Format(backupStamp)
	if counter > 0 {
		name += "-" + strconv.Itoa(counter)
	}
	if beforeRestore {
		return name + beforeRestoreSuffix
	}
	return name + ".db"
}

func (a *App) DeleteBackup(name string) error {
	path, err := a.backupPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// backupPath turns a name from a form into a path, refusing anything that is
// not one of this app's own backup files.
func (a *App) backupPath(name string) (string, error) {
	if !backupPattern.MatchString(name) {
		return "", errNoSuchBackup
	}
	path := filepath.Join(a.backupsDir(), name)
	if !fileExists(path) {
		return "", errNoSuchBackup
	}
	return path, nil
}

var errNoSuchBackup = errors.New("no such backup")

// backupOnStartup takes one snapshot per day, before anything has had a chance
// to write. It is best-effort: a family that cannot write backups should still
// be able to use the app, so the caller only logs what comes back.
func (a *App) backupOnStartup() error {
	backups, err := a.Backups()
	if err != nil {
		return err
	}
	if len(backups) > 0 && sameDay(backups[0].Taken, time.Now()) {
		return nil
	}
	// Nothing to protect yet on a first run.
	if empty, err := a.storeIsEmpty(); err != nil || empty {
		return err
	}
	_, err = a.MakeBackup()
	return err
}

func (a *App) storeIsEmpty() (bool, error) {
	kids, err := a.store.Kids(true)
	if err != nil {
		return false, err
	}
	return len(kids) == 0, nil
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// pruneBackups keeps the newest few, never discarding a copy taken just before
// a restore: those exist precisely because something went wrong.
func (a *App) pruneBackups() error {
	backups, err := a.Backups()
	if err != nil {
		return err
	}
	kept := 0
	for _, backup := range backups {
		if backup.BeforeRestore() {
			continue
		}
		kept++
		if kept <= keepBackups {
			continue
		}
		if err := os.Remove(filepath.Join(a.backupsDir(), backup.Name)); err != nil {
			return err
		}
	}
	return nil
}

func sizeLabel(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
