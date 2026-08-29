package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// maxUploadBytes caps a single upload. Worksheets and phone photos of graded
// pages sit well under this.
const maxUploadBytes = 64 << 20

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "That file was too large or the upload was incomplete.", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	back := safeRedirect(r.FormValue("back"), "/")
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		a.redirect(w, r, back)
		return
	}

	record := Attachment{
		OwnerType:        r.FormValue("owner_type"),
		LessonID:         formID(r, "lesson_id"),
		AssessmentID:     formID(r, "assessment_id"),
		KidID:            formID(r, "kid_id"),
		SubjectID:        formID(r, "subject_id"),
		CurriculumPlanID: formID(r, "curriculum_plan_id"),
	}
	switch record.OwnerType {
	case OwnerLesson:
		if record.LessonID == 0 {
			http.Error(w, "Missing lesson for this file.", http.StatusBadRequest)
			return
		}
	case OwnerAssessment:
		if record.AssessmentID == 0 {
			http.Error(w, "Missing test for this file.", http.StatusBadRequest)
			return
		}
	case OwnerResource:
		if record.KidID == 0 || record.SubjectID == 0 {
			http.Error(w, "Missing child or subject for this file.", http.StatusBadRequest)
			return
		}
	case OwnerCurriculum:
		if record.CurriculumPlanID == 0 {
			http.Error(w, "Missing curriculum plan for this file.", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "Unknown file owner.", http.StatusBadRequest)
		return
	}

	for _, header := range files {
		if header.Size == 0 {
			continue
		}
		stored, err := a.saveUpload(header.Filename, header)
		if err != nil {
			a.serverError(w, err)
			return
		}
		entry := record
		entry.OriginalName = filepath.Base(header.Filename)
		entry.StoredPath = stored
		entry.SizeBytes = header.Size
		entry.ContentType = header.Header.Get("Content-Type")
		if _, err := a.store.CreateAttachment(entry); err != nil {
			os.Remove(filepath.Join(a.uploadDir, stored))
			a.serverError(w, err)
			return
		}
	}
	a.redirect(w, r, back)
}

// saveUpload writes the file under uploads/YYYY/MM and returns the path
// relative to the upload folder, so the data folder stays movable.
func (a *App) saveUpload(name string, header *multipart.FileHeader) (string, error) {
	src, err := header.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	now := time.Now()
	dir := filepath.Join(now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(filepath.Join(a.uploadDir, dir), 0o755); err != nil {
		return "", err
	}

	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	stored := filepath.Join(dir, hex.EncodeToString(buf)+"-"+safeFilename(name))

	dst, err := os.OpenFile(filepath.Join(a.uploadDir, stored), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(dst.Name())
		return "", err
	}
	return filepath.ToSlash(stored), nil
}

func (a *App) handleDownload(w http.ResponseWriter, r *http.Request) {
	record, err := a.store.Attachment(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}

	path, ok := a.resolveUpload(record.StoredPath)
	if !ok {
		a.notFound(w)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		a.notFound(w)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		a.serverError(w, err)
		return
	}

	if record.ContentType != "" {
		w.Header().Set("Content-Type", record.ContentType)
	}
	// Images and PDFs preview in the browser; anything else downloads.
	disposition := "attachment"
	if strings.HasPrefix(record.ContentType, "image/") || record.ContentType == "application/pdf" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType(disposition, map[string]string{"filename": record.OriginalName}))
	http.ServeContent(w, r, record.OriginalName, info.ModTime(), file)
}

func (a *App) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	record, err := a.store.Attachment(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
			return
		}
		a.serverError(w, err)
		return
	}
	if path, ok := a.resolveUpload(record.StoredPath); ok {
		os.Remove(path)
	}
	if err := a.store.DeleteAttachment(record.ID); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
}

func (a *App) deleteLessonFiles(lessonID int64) error {
	records, err := a.store.AttachmentsForLesson(lessonID)
	if err != nil {
		return err
	}
	a.removeFiles(records)
	return nil
}

func (a *App) deleteAssessmentFiles(assessmentID int64) error {
	records, err := a.store.AttachmentsForAssessment(assessmentID)
	if err != nil {
		return err
	}
	a.removeFiles(records)
	return nil
}

func (a *App) removeFiles(records []Attachment) {
	for _, record := range records {
		if path, ok := a.resolveUpload(record.StoredPath); ok {
			os.Remove(path)
		}
	}
}

// resolveUpload turns a stored relative path into an absolute one, refusing
// anything that would escape the upload folder.
func (a *App) resolveUpload(stored string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(stored))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || strings.HasPrefix(clean, string(os.PathSeparator)) {
		return "", false
	}
	full := filepath.Join(a.uploadDir, clean)
	if !strings.HasPrefix(full, a.uploadDir+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeFilename(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	base = unsafeFilename.ReplaceAllString(base, "_")
	base = strings.Trim(base, "._-")
	if base == "" {
		base = "file"
	}
	if len(base) > 80 {
		ext := filepath.Ext(base)
		base = base[:80-len(ext)] + ext
	}
	return base
}
