package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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
	if !a.requirePlanningAccess(w, r) {
		return
	}
	a.filesMu.Lock()
	defer a.filesMu.Unlock()
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

	var entries []Attachment
	var staged []string
	cleanup := func() {
		for _, stored := range staged {
			_ = os.Remove(filepath.Join(a.uploadDir, filepath.FromSlash(stored)))
		}
	}
	for _, header := range files {
		if header.Size == 0 {
			continue
		}
		stored, contentType, err := a.saveUpload(header.Filename, header)
		if err != nil {
			cleanup()
			a.serverError(w, err)
			return
		}
		staged = append(staged, stored)
		entry := record
		entry.OriginalName = filepath.Base(header.Filename)
		entry.StoredPath = stored
		entry.SizeBytes = header.Size
		entry.ContentType = contentType
		entries = append(entries, entry)
	}
	if err := a.store.CreateAttachments(entries); err != nil {
		cleanup()
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, back)
}

// saveUpload writes the file under uploads/YYYY/MM and returns the path
// relative to the upload folder, so the data folder stays movable, plus a
// sniffed Content-Type (PDF magic wins over a vague browser multipart type).
func (a *App) saveUpload(name string, header *multipart.FileHeader) (string, string, error) {
	src, err := header.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	now := time.Now()
	dir := filepath.Join(now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(filepath.Join(a.uploadDir, dir), 0o755); err != nil {
		return "", "", err
	}

	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	stored := filepath.Join(dir, hex.EncodeToString(buf)+"-"+safeFilename(name))

	dst, err := os.OpenFile(filepath.Join(a.uploadDir, stored), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()

	contentType, reader, err := detectUploadContentType(name, header.Header.Get("Content-Type"), src)
	if err != nil {
		os.Remove(dst.Name())
		return "", "", err
	}
	if _, err := io.Copy(dst, reader); err != nil {
		os.Remove(dst.Name())
		return "", "", err
	}
	return filepath.ToSlash(stored), contentType, nil
}

// detectUploadContentType prefers PDF magic bytes, then http.DetectContentType,
// then the multipart Content-Type, then the filename extension.
func detectUploadContentType(filename, headerType string, r io.Reader) (string, io.Reader, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", nil, err
	}
	head = head[:n]
	rest := io.MultiReader(bytes.NewReader(head), r)

	if bytes.HasPrefix(head, []byte("%PDF")) {
		return "application/pdf", rest, nil
	}
	sniffed := http.DetectContentType(head)
	if sniffed != "" && sniffed != "application/octet-stream" && !strings.HasPrefix(sniffed, "text/plain") {
		return sniffed, rest, nil
	}
	if ct := strings.TrimSpace(headerType); ct != "" && ct != "application/octet-stream" {
		return ct, rest, nil
	}
	if ext := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); ext != "" {
		return ext, rest, nil
	}
	if sniffed != "" {
		return sniffed, rest, nil
	}
	return "application/octet-stream", rest, nil
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
	if !a.enforceAttachmentAccess(w, r, record) {
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
	// Attachment IDs are immutable: replacing a file creates a new record and
	// URL. Let the user's browser retain fetched PDF ranges without allowing a
	// shared proxy to cache private family files.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Vary", "Cookie")
	w.Header().Set("ETag", fmt.Sprintf(`"attachment-%d-%d-%d"`,
		record.ID, info.Size(), info.ModTime().Unix()))
	// Images and PDFs preview in the browser; SVG is never inline (stored XSS).
	// Anything else downloads.
	disposition := "attachment"
	isSVG := strings.EqualFold(record.ContentType, "image/svg+xml") ||
		strings.EqualFold(filepath.Ext(record.OriginalName), ".svg")
	if !isSVG && (strings.HasPrefix(record.ContentType, "image/") || record.ContentType == "application/pdf") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType(disposition, map[string]string{"filename": record.OriginalName}))
	// ServeContent honors Range requests (Accept-Ranges / 206), which PDF.js
	// uses to stream large curriculum books without downloading every byte.
	http.ServeContent(w, r, record.OriginalName, info.ModTime(), file)
}

func (a *App) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if !a.requirePlanningAccess(w, r) {
		return
	}
	record, err := a.store.Attachment(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
			return
		}
		a.serverError(w, err)
		return
	}
	if err := a.store.DeleteAttachment(record.ID); err != nil {
		a.serverError(w, err)
		return
	}
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/"))
}

func (a *App) deleteLessonFiles(lessonID int64) error {
	// Attachment bytes are immutable history objects. The attachment rows are
	// removed by the lesson cascade and history GC reclaims bytes only after no
	// live row or retained branch references them.
	return nil
}

func (a *App) purgeExpiredLessonTrash(now time.Time) error {
	_, err := a.store.PurgeExpiredDeletedLessons(now)
	if err != nil {
		return err
	}
	return a.gcHistoryFiles()
}

func (a *App) deleteAssessmentFiles(assessmentID int64) error {
	// See deleteLessonFiles: retain bytes until history GC proves them unused.
	return nil
}

func (a *App) removeFiles(records []Attachment) {
	// SQL cascades remove the metadata. Immutable bytes remain available to
	// undo branches and are reclaimed by history garbage collection.
}

// resolveUpload turns a stored relative path into an absolute one, refusing
// anything that would escape the upload folder.
func (a *App) resolveUpload(stored string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(stored))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || strings.HasPrefix(clean, string(os.PathSeparator)) {
		return "", false
	}
	full := filepath.Join(a.uploadDir, clean)
	// Rel is the right check on Windows: drive-letter case can differ between
	// the folder we opened and the path Join produces, and HasPrefix would
	// refuse a photo that is sitting right where we put it.
	rel, err := filepath.Rel(a.uploadDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
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
