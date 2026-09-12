package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const familyArchiveName = "family-export.zip"

// handleFamilyExport downloads school.db + uploads/ for the signed-in family only.
func (a *App) handleFamilyExport(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "No family data.", http.StatusBadRequest)
		return
	}

	tmp, err := os.CreateTemp("", "school-nanny-export-*.zip")
	if err != nil {
		a.serverError(w, err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	zw := zip.NewWriter(tmp)

	snapPath := filepath.Join(a.dataDir, ".export-snapshot.db")
	defer os.Remove(snapPath)
	if err := a.store.SnapshotTo(snapPath); err != nil {
		tmp.Close()
		a.serverError(w, err)
		return
	}
	if err := zipAddFile(zw, snapPath, dbFileName); err != nil {
		tmp.Close()
		a.serverError(w, err)
		return
	}
	uploads := filepath.Join(a.dataDir, uploadsFolderName)
	if err := zipAddDir(zw, uploads, uploadsFolderName); err != nil {
		tmp.Close()
		a.serverError(w, err)
		return
	}
	if err := zw.Close(); err != nil {
		tmp.Close()
		a.serverError(w, err)
		return
	}
	if err := tmp.Close(); err != nil {
		a.serverError(w, err)
		return
	}

	name := fmt.Sprintf("school-nanny-family-%s.zip", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, tmpName)
}

// handleFamilyImport restores an export archive into the signed-in family only.
func (a *App) handleFamilyImport(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "No family data.", http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		http.Error(w, "Could not read that upload.", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		http.Error(w, "Choose a family export zip.", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "school-nanny-import-*.zip")
	if err != nil {
		a.serverError(w, err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		a.serverError(w, err)
		return
	}
	if err := tmp.Close(); err != nil {
		a.serverError(w, err)
		return
	}
	_ = header

	if err := a.importFamilyArchive(tmpName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.redirect(w, r, "/settings?saved=imported")
}

func (a *App) importFamilyArchive(zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("That file is not a readable zip.")
	}
	defer zr.Close()

	var dbEntry *zip.File
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if name == dbFileName || strings.HasSuffix(name, "/"+dbFileName) {
			dbEntry = f
			break
		}
	}
	if dbEntry == nil {
		return fmt.Errorf("That archive has no school.db.")
	}

	extractDir, err := os.MkdirTemp(a.dataDir, "import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	dbDest := filepath.Join(extractDir, dbFileName)
	if err := unzipFile(dbEntry, dbDest); err != nil {
		return err
	}
	uploadsDest := filepath.Join(extractDir, uploadsFolderName)
	if err := os.MkdirAll(uploadsDest, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if f.FileInfo().IsDir() {
			continue
		}
		rel := name
		if i := strings.Index(name, uploadsFolderName+"/"); i >= 0 {
			rel = name[i+len(uploadsFolderName)+1:]
		} else if strings.HasPrefix(name, uploadsFolderName+"/") {
			rel = strings.TrimPrefix(name, uploadsFolderName+"/")
		} else {
			continue
		}
		if rel == "" || strings.Contains(rel, "..") {
			continue
		}
		out := filepath.Join(uploadsDest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := unzipFile(f, out); err != nil {
			return err
		}
	}

	safety, err := a.MakeBackup()
	if err != nil {
		return err
	}
	_ = safety

	if err := a.store.RestoreFrom(dbDest, ""); err != nil {
		return err
	}

	liveUploads := filepath.Join(a.dataDir, uploadsFolderName)
	backupUploads := liveUploads + ".before-import"
	_ = os.RemoveAll(backupUploads)
	if _, err := os.Stat(liveUploads); err == nil {
		if err := os.Rename(liveUploads, backupUploads); err != nil {
			return err
		}
	}
	if err := os.Rename(uploadsDest, liveUploads); err != nil {
		_ = os.Rename(backupUploads, liveUploads)
		return err
	}
	_ = os.RemoveAll(backupUploads)
	return nil
}

func zipAddFile(zw *zip.Writer, src, name string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = name
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func zipAddDir(zw *zip.Writer, dir, prefix string) error {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(prefix, rel))
		return zipAddFile(zw, path, name)
	})
}

func unzipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// Silence unused in case of build tags; keep name for docs.
var _ = familyArchiveName
