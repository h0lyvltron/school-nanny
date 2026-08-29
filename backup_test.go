package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (ta *testApp) kidNames() []string {
	ta.t.Helper()
	kids, err := ta.store.Kids(true)
	if err != nil {
		ta.t.Fatalf("listing kids: %v", err)
	}
	names := make([]string, 0, len(kids))
	for _, kid := range kids {
		names = append(names, kid.Name)
	}
	return names
}

func (ta *testApp) onlyBackup() Backup {
	ta.t.Helper()
	backups, err := ta.Backups()
	if err != nil {
		ta.t.Fatalf("listing backups: %v", err)
	}
	if len(backups) != 1 {
		ta.t.Fatalf("expected exactly one backup, got %d", len(backups))
	}
	return backups[0]
}

func TestRestoreBringsBackWhatWasThereBefore(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")

	backup, err := ta.MakeBackup()
	if err != nil {
		t.Fatalf("making a backup: %v", err)
	}

	ta.addKid("Noah")
	if got := ta.kidNames(); len(got) != 2 {
		t.Fatalf("expected two kids before restoring, got %v", got)
	}

	if err := ta.RestoreBackup(backup.Name); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	got := ta.kidNames()
	if len(got) != 1 || got[0] != "Mia" {
		t.Fatalf("expected only Mia after restoring, got %v", got)
	}
}

// Restoring the wrong backup should not be the end of the story, so the records
// it replaced are kept and can be put straight back.
func TestRestoreItselfCanBeUndone(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")
	early, err := ta.MakeBackup()
	if err != nil {
		t.Fatalf("making a backup: %v", err)
	}

	ta.addKid("Noah")
	if err := ta.RestoreBackup(early.Name); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	backups, err := ta.Backups()
	if err != nil {
		t.Fatalf("listing backups: %v", err)
	}
	var safety Backup
	for _, backup := range backups {
		if backup.BeforeRestore() {
			safety = backup
		}
	}
	if safety.Name == "" {
		t.Fatalf("expected a copy kept from before the restore, got %v", backups)
	}

	if err := ta.RestoreBackup(safety.Name); err != nil {
		t.Fatalf("restoring the safety copy: %v", err)
	}
	if got := ta.kidNames(); len(got) != 2 {
		t.Fatalf("expected both kids back, got %v", got)
	}
}

// The database file is swapped underneath a running app, so the pages have to
// keep working afterwards without a restart.
func TestAppKeepsServingAfterARestore(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")
	backup, err := ta.MakeBackup()
	if err != nil {
		t.Fatalf("making a backup: %v", err)
	}
	ta.addKid("Noah")

	status, _ := ta.post("/settings/backups/"+backup.Name+"/restore", nil)
	if status != http.StatusOK {
		t.Fatalf("restore returned %d", status)
	}

	status, body := ta.get("/")
	if status != http.StatusOK {
		t.Fatalf("home returned %d after restoring", status)
	}
	mustContain(t, body, "Mia", "home after restoring")
	mustNotContain(t, body, "Noah", "home after restoring")

	// Writing has to work too: a read-only pool would still pass the check above.
	ta.addKid("Ivy")
	if got := ta.kidNames(); len(got) != 2 {
		t.Fatalf("expected Mia and Ivy after restoring, got %v", got)
	}
}

// A snapshot has to include writes still sitting in the write-ahead log, which
// is what copying the .db file on its own would miss.
func TestBackupIncludesTheMostRecentWrites(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")

	backup, err := ta.MakeBackup()
	if err != nil {
		t.Fatalf("making a backup: %v", err)
	}

	path := filepath.Join(ta.backupsDir(), backup.Name)

	// VACUUM INTO produces one self-contained file, with nothing alongside it
	// that could be left behind when the backup is copied elsewhere. Checked
	// before opening it, since opening is itself what creates those sidecars.
	for _, sidecar := range []string{"-wal", "-shm"} {
		if fileExists(path + sidecar) {
			t.Errorf("backup left a %s file beside it", sidecar)
		}
	}

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("opening the backup: %v", err)
	}
	defer store.Close()

	kids, err := store.Kids(true)
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	if len(kids) != 1 || kids[0].Name != "Mia" {
		t.Fatalf("expected Mia inside the backup, got %v", kids)
	}
}

func TestRestoreRefusesAFileThatIsNotABackup(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")

	// Correctly named, so only reading it can tell it apart.
	name := backupName(time.Now(), 0, false)
	if err := os.MkdirAll(ta.backupsDir(), 0o755); err != nil {
		t.Fatalf("making the backups folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ta.backupsDir(), name), []byte("not a database"), 0o644); err != nil {
		t.Fatalf("writing the decoy: %v", err)
	}

	if err := ta.RestoreBackup(name); err == nil {
		t.Fatal("expected restoring a file that is not a backup to fail")
	}
	if got := ta.kidNames(); len(got) != 1 || got[0] != "Mia" {
		t.Fatalf("records should be untouched after a refused restore, got %v", got)
	}
}

func TestBackupNamesCannotReachOtherFiles(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")

	for _, name := range []string{
		"../school.db",
		"..%2Fschool.db",
		"school.db",
		"school-2026-01-02-030405.db.txt",
	} {
		if _, err := ta.backupPath(name); err == nil {
			t.Errorf("expected %q to be refused as a backup name", name)
		}
	}

	status, _ := ta.get("/settings/backups/..%2F..%2Fschool.db")
	if status == http.StatusOK {
		t.Error("downloading a path outside the backups folder should not succeed")
	}
}

func TestOldBackupsArePrunedButSafetyCopiesAreKept(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")

	when := time.Now().Add(-time.Duration(keepBackups+5) * time.Hour)
	if _, err := ta.makeBackup(when, true); err != nil {
		t.Fatalf("making the safety copy: %v", err)
	}
	for i := 0; i < keepBackups+3; i++ {
		if _, err := ta.makeBackup(when.Add(time.Duration(i+1)*time.Hour), false); err != nil {
			t.Fatalf("making backup %d: %v", i, err)
		}
	}

	backups, err := ta.Backups()
	if err != nil {
		t.Fatalf("listing backups: %v", err)
	}
	plain, safety := 0, 0
	for _, backup := range backups {
		if backup.BeforeRestore() {
			safety++
		} else {
			plain++
		}
	}
	if plain != keepBackups {
		t.Errorf("expected %d ordinary backups to be kept, got %d", keepBackups, plain)
	}
	if safety != 1 {
		t.Errorf("expected the copy kept before a restore to survive pruning, got %d", safety)
	}
}

func TestStartupBackupRunsOnceADay(t *testing.T) {
	ta := newTestApp(t)

	// Nothing recorded yet, so there is nothing worth keeping.
	if err := ta.backupOnStartup(); err != nil {
		t.Fatalf("first startup: %v", err)
	}
	if backups, _ := ta.Backups(); len(backups) != 0 {
		t.Fatalf("expected no backup before any records exist, got %v", backups)
	}

	ta.addKid("Mia")
	if err := ta.backupOnStartup(); err != nil {
		t.Fatalf("startup with records: %v", err)
	}
	first := ta.onlyBackup()

	if err := ta.backupOnStartup(); err != nil {
		t.Fatalf("second startup the same day: %v", err)
	}
	again := ta.onlyBackup()
	if again.Name != first.Name {
		t.Errorf("expected one backup per day, got %q then %q", first.Name, again.Name)
	}
}

func TestSettingsPageShowsWhereTheRecordsLive(t *testing.T) {
	ta := newTestApp(t)
	ta.addKid("Mia")
	if _, err := ta.MakeBackup(); err != nil {
		t.Fatalf("making a backup: %v", err)
	}

	status, body := ta.get("/settings")
	if status != http.StatusOK {
		t.Fatalf("settings returned %d", status)
	}
	mustContain(t, body, ta.dataDir, "settings page")
	mustContain(t, body, "Back up now", "settings page")
	mustContain(t, body, "/restore", "settings page")
}

// Moving to the per-user folder must not look like losing everything, so an
// older folder is copied in the first time and left alone afterwards.
func TestExistingRecordsAreAdoptedOnce(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	fresh := filepath.Join(root, "fresh")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatalf("making the older folder: %v", err)
	}

	store, err := OpenStore(filepath.Join(old, dbFileName))
	if err != nil {
		t.Fatalf("creating the older database: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	if _, err := store.CreateKid("Mia", "3rd", "#5b8def"); err != nil {
		t.Fatalf("adding a kid: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(old, uploadsFolderName), 0o755); err != nil {
		t.Fatalf("making the uploads folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(old, uploadsFolderName, "worksheet.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("writing an attachment: %v", err)
	}
	store.Close()

	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatalf("making the new folder: %v", err)
	}
	from, err := adoptFrom(fresh, []string{old})
	if err != nil {
		t.Fatalf("adopting: %v", err)
	}
	if from != old {
		t.Fatalf("expected records to come from %s, got %q", old, from)
	}

	moved, err := OpenStore(filepath.Join(fresh, dbFileName))
	if err != nil {
		t.Fatalf("opening the copied database: %v", err)
	}
	defer moved.Close()
	kids, err := moved.Kids(true)
	if err != nil {
		t.Fatalf("reading the copied database: %v", err)
	}
	if len(kids) != 1 || kids[0].Name != "Mia" {
		t.Fatalf("expected Mia to come across, got %v", kids)
	}
	if !fileExists(filepath.Join(fresh, uploadsFolderName, "worksheet.txt")) {
		t.Error("attachments should be copied across too")
	}

	// The originals stay put, so a wrong guess here costs nothing.
	if !fileExists(filepath.Join(old, dbFileName)) {
		t.Error("the older folder should be left alone")
	}

	// Second run: records are already here, so nothing is copied over them.
	if _, err := moved.CreateKid("Noah", "1st", "#e0709a"); err != nil {
		t.Fatalf("adding a kid: %v", err)
	}
	from, err = adoptFrom(fresh, []string{old})
	if err != nil {
		t.Fatalf("second adoption attempt: %v", err)
	}
	if from != "" {
		t.Errorf("expected no second adoption, got %q", from)
	}
	kids, err = moved.Kids(true)
	if err != nil {
		t.Fatalf("reading after the second attempt: %v", err)
	}
	if len(kids) != 2 {
		t.Errorf("expected the newer records to survive, got %v", kids)
	}
}

func TestAdoptionIgnoresAFolderWithNoDatabase(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	fresh := filepath.Join(root, "fresh")
	for _, dir := range []string{empty, fresh} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("making %s: %v", dir, err)
		}
	}

	from, err := adoptFrom(fresh, []string{empty, filepath.Join(root, "missing")})
	if err != nil {
		t.Fatalf("adopting: %v", err)
	}
	if from != "" {
		t.Errorf("expected nothing to be adopted, got %q", from)
	}
}

func TestDefaultDataDirIsOutsideTheProgramFolder(t *testing.T) {
	dir, err := defaultDataDir()
	if err != nil {
		t.Fatalf("finding the default folder: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected an absolute path, got %q", dir)
	}
	if !strings.HasSuffix(dir, dataFolderName) {
		t.Errorf("expected the folder to be named %q, got %q", dataFolderName, dir)
	}

	// The whole point of the move: a build that clears its output folder can no
	// longer take the records with it.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting the working directory: %v", err)
	}
	if strings.HasPrefix(dir, filepath.Join(cwd, "dist")) {
		t.Errorf("records must not default to inside the build output: %q", dir)
	}
}
