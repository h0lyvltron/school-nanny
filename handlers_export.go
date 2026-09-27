package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const familyArchiveName = "family-export.zip"

const (
	// zipBombFloor is how much honest data we always allow before the
	// compression ratio is allowed to reject an archive. Photos and PDFs
	// barely compress; a bomb expands by hundreds of times.
	zipBombFloor   = 64 << 20
	zipBombRatio   = 100
	maxZipEntries  = 100_000
	diskFreeMargin = 64 << 20
)

var errArchiveTooLarge = errors.New("That archive is too large to restore here.")

// zipBudget counts bytes actually written while an archive is unpacked.
// Compressed size is the zip file on disk, not the size claimed in the headers.
type zipBudget struct {
	compressed int64
	written    int64
	entries    int
	volume     string
}

func newZipBudget(zipPath, volume string) (*zipBudget, error) {
	info, err := os.Stat(zipPath)
	if err != nil {
		return nil, err
	}
	return &zipBudget{compressed: info.Size(), volume: volume}, nil
}

func (b *zipBudget) addEntry() error {
	if b == nil {
		return nil
	}
	b.entries++
	if b.entries > maxZipEntries {
		return errArchiveTooLarge
	}
	return nil
}

func (b *zipBudget) addBytes(n int64) error {
	if b == nil || n <= 0 {
		return nil
	}
	b.written += n
	if b.written > zipBombFloor && b.compressed > 0 && b.written/zipBombRatio > b.compressed {
		return errArchiveTooLarge
	}
	if b.volume == "" {
		return nil
	}
	free, err := diskFree(b.volume)
	if err != nil {
		return nil
	}
	if free < diskFreeMargin {
		return errArchiveTooLarge
	}
	return nil
}

func zipMemberCount(files []*zip.File) int {
	n := 0
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		n++
	}
	return n
}

func uploadZipRel(name string) (string, bool) {
	name = filepath.ToSlash(name)
	marker := uploadsFolderName + "/"
	i := strings.Index(name, marker)
	if i < 0 {
		return "", false
	}
	rel := name[i+len(marker):]
	if rel == "" {
		return "", false
	}
	return rel, true
}

// handleFamilyExport downloads school.db + uploads/ for the signed-in family only.
func (a *App) handleFamilyExport(w http.ResponseWriter, r *http.Request) {
	if !a.requireOwner(w, r) {
		return
	}
	if a.store == nil {
		http.Error(w, "No family data.", http.StatusBadRequest)
		return
	}

	tmp, err := os.CreateTemp(a.dataDir, "school-nanny-export-*.zip")
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
	if err := a.zipCurriculumYAML(zw); err != nil {
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
	if !a.requireOwner(w, r) {
		return
	}
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

	tmp, err := os.CreateTemp(a.dataDir, "school-nanny-import-*.zip")
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

	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode == "" {
		mode = "replace"
	}
	switch mode {
	case "replace":
		if strings.TrimSpace(r.FormValue("confirm_replace")) != "REPLACE" {
			http.Error(w, `Type REPLACE to confirm replacing this family's records.`, http.StatusBadRequest)
			return
		}
		if err := a.importFamilyArchive(tmpName); err != nil {
			a.archiveImportError(w, err)
			return
		}
		a.redirect(w, r, "/settings/data?saved=imported")
	case "merge":
		n, err := a.mergeFamilyArchive(tmpName)
		if err != nil {
			a.archiveImportError(w, err)
			return
		}
		a.redirect(w, r, fmt.Sprintf("/settings/data?saved=merged&plans=%d", n))
	default:
		http.Error(w, "Choose Replace or Merge.", http.StatusBadRequest)
	}
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
	if zipMemberCount(zr.File) > maxZipEntries {
		return errArchiveTooLarge
	}
	budget, err := newZipBudget(zipPath, a.dataDir)
	if err != nil {
		return err
	}

	extractDir, err := os.MkdirTemp(a.dataDir, "import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	dbDest := filepath.Join(extractDir, dbFileName)
	if err := unzipFile(dbEntry, dbDest, budget); err != nil {
		return err
	}
	uploadsDest := filepath.Join(extractDir, uploadsFolderName)
	if err := os.MkdirAll(uploadsDest, dirPerm); err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel, ok := uploadZipRel(f.Name)
		if !ok {
			continue
		}
		out, ok := containedPath(uploadsDest, rel)
		if !ok {
			continue
		}
		if err := unzipFile(f, out, budget); err != nil {
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
	// Replace import is an operational boundary, not a branchable planner
	// action. Do not inherit a tree whose snapshots describe the source family.
	if err := a.store.ClearHistory(); err != nil {
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

// mergeFamilyArchive keeps the live school.db and adds curriculum YAML plus
// missing upload files from the archive. It never replaces family records.
func (a *App) mergeFamilyArchive(zipPath string) (imported int, err error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("That file is not a readable zip.")
	}
	defer zr.Close()
	if zipMemberCount(zr.File) > maxZipEntries {
		return 0, errArchiveTooLarge
	}
	budget, err := newZipBudget(zipPath, a.dataDir)
	if err != nil {
		return 0, err
	}

	if _, err = a.MakeBackup(); err != nil {
		return 0, err
	}

	subjects, err := a.store.Subjects(true)
	if err != nil {
		return 0, err
	}

	var yamlFiles []*zip.File
	var allYAML *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		if !strings.Contains(name, "curriculum/") {
			continue
		}
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			continue
		}
		base := filepath.Base(name)
		if strings.EqualFold(base, "all.yaml") || strings.EqualFold(base, "all.yml") {
			allYAML = f
			continue
		}
		yamlFiles = append(yamlFiles, f)
	}
	toImport := yamlFiles
	if allYAML != nil {
		toImport = []*zip.File{allYAML}
	}
	for _, f := range toImport {
		body, err := readZipFile(f)
		if err != nil {
			return imported, err
		}
		plans, err := parseCurriculumImport(filepath.Base(f.Name), body, subjects)
		if err != nil {
			return imported, fmt.Errorf("curriculum %s: %v", filepath.Base(f.Name), err)
		}
		n, err := a.store.ImportCurriculum(plans)
		if err != nil {
			return imported, err
		}
		imported += n
	}

	liveUploads := filepath.Join(a.dataDir, uploadsFolderName)
	if err = os.MkdirAll(liveUploads, dirPerm); err != nil {
		return imported, err
	}
	var created []string
	defer func() {
		if err == nil {
			return
		}
		for _, path := range created {
			_ = os.Remove(path)
		}
	}()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel, ok := uploadZipRel(f.Name)
		if !ok {
			continue
		}
		out, ok := containedPath(liveUploads, rel)
		if !ok {
			continue
		}
		if _, statErr := os.Stat(out); statErr == nil {
			continue
		}
		created = append(created, out)
		if err = unzipFile(f, out, budget); err != nil {
			return imported, err
		}
	}
	return imported, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxImportBytes+1))
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

func unzipFile(f *zip.File, dest string, budget *zipBudget) error {
	if err := budget.addEntry(); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), dirPerm); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerm)
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, 32<<10)
	for {
		n, rerr := rc.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			if err := budget.addBytes(int64(n)); err != nil {
				return err
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

func (a *App) archiveImportError(w http.ResponseWriter, err error) {
	if errors.Is(err, errArchiveTooLarge) || userArchiveError(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.serverError(w, err)
}

func userArchiveError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "That ") || strings.HasPrefix(msg, "curriculum ")
}

func (a *App) zipCurriculumYAML(zw *zip.Writer) error {
	if a.store == nil {
		return nil
	}
	summaries, err := a.store.CurriculumPlans()
	if err != nil {
		return err
	}
	plans := make([]CurriculumPlan, 0, len(summaries))
	for _, summary := range summaries {
		plan, err := a.store.CurriculumPlan(summary.ID)
		if err != nil {
			return err
		}
		if len(plan.Items) == 0 {
			continue
		}
		plans = append(plans, plan)
		body, err := EmitPlansYAML([]CurriculumPlan{plan})
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join("curriculum", planYAMLFilename(plan)))
		if err := zipAddBytes(zw, name, []byte(body)); err != nil {
			return err
		}
	}
	if len(plans) == 0 {
		return zipAddBytes(zw, "curriculum/all.yaml", []byte("# no curriculum plans\n"))
	}
	all, err := EmitPlansYAML(plans)
	if err != nil {
		return err
	}
	return zipAddBytes(zw, "curriculum/all.yaml", []byte(all))
}

func zipAddBytes(zw *zip.Writer, name string, body []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// Silence unused in case of build tags; keep name for docs.
var _ = familyArchiveName
