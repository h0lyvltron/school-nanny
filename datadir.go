package main

// Where the family's records live.
//
// The folder used to be "data" resolved against whatever directory the app was
// started from, which meant the launcher in dist\windows and a "go run ." in
// the source tree quietly used two different databases, and a rebuild that
// cleared dist\windows took the records with it. The default is now a fixed
// per-user folder that no build ever touches, so every way of starting the app
// finds the same records.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	dataFolderName    = "school-nanny"
	uploadsFolderName = "uploads"
)

// defaultDataDir is where records go when -data was not given.
func defaultDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		// %LOCALAPPDATA% rather than the roaming %APPDATA%: a SQLite database
		// should not be copied around by a roaming profile.
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, dataFolderName), nil
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", dataFolderName), nil
		}
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, dataFolderName), nil
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share", dataFolderName), nil
		}
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding a folder for the records: %w", err)
	}
	return filepath.Join(dir, dataFolderName), nil
}

// adoptExistingData seeds a new data folder from records left in one of the old
// locations, so moving to the per-user folder does not look like losing
// everything. It copies rather than moves: if anything about this is wrong, the
// originals are still sitting where they were.
//
// It only ever runs when the new folder has no database, so it cannot overwrite
// records that are already in use.
func adoptExistingData(dataDir string) (string, error) {
	return adoptFrom(dataDir, legacyDataDirs())
}

func adoptFrom(dataDir string, candidates []string) (string, error) {
	if fileExists(filepath.Join(dataDir, dbFileName)) {
		return "", nil
	}

	for _, candidate := range candidates {
		if candidate == "" || samePath(candidate, dataDir) {
			continue
		}
		if !fileExists(filepath.Join(candidate, dbFileName)) {
			continue
		}
		if err := copyTree(candidate, dataDir); err != nil {
			return "", fmt.Errorf("copying records from %s: %w", candidate, err)
		}
		return candidate, nil
	}
	return "", nil
}

// legacyDataDirs lists the places the old CWD-relative default could have put
// records: beside the executable, which is where the Windows launcher ran it,
// and under the working directory, which is where "go run ." ran it.
func legacyDataDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "data"))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, "data"))
	}
	return dirs
}

// samePath reports whether two paths name the same thing, taking Windows'
// case-insensitive filesystem into account.
func samePath(a, b string) bool {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(absA, absB)
	}
	return absA == absB
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// copyTree copies a folder recursively. The database is copied along with its
// -wal and -shm sidecars, which is only safe because this runs before anything
// has opened it.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
