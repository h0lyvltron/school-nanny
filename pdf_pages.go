package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const pdfPageContentType = "application/pdf"

// PDFPage is one extracted page of a curriculum PDF.
type PDFPage struct {
	AttachmentID int64
	PageNumber   int
	ContentHash  string
	StoredPath   string
	ContentType  string
}

func (a *App) startPDFPageSplit() {
	if a == nil || a.store == nil || a.uploadDir == "" {
		return
	}
	go func() {
		if err := a.splitPendingCurriculumPDFs(); err != nil {
			log.Printf("pdf pages: %v", err)
		}
	}()
}

func (a *App) splitPendingCurriculumPDFs() error {
	atts, err := a.store.CurriculumPDFAttachments()
	if err != nil {
		return err
	}
	for _, att := range atts {
		if err := a.splitCurriculumPDF(att); err != nil {
			log.Printf("pdf pages: attachment %d: %v", att.ID, err)
		}
	}
	return nil
}

// splitCurriculumPDF writes any missing pages of att. Pages already recorded
// are left in place. A page that cannot be extracted is skipped so the rest
// of the book, and the lessons that point at it, stay usable.
func (a *App) splitCurriculumPDF(att Attachment) error {
	if !isPDFAttachment(att) {
		return nil
	}
	a.pdfSplitMu.Lock()
	defer a.pdfSplitMu.Unlock()

	src, ok := a.resolveUpload(att.StoredPath)
	if !ok {
		return nil
	}
	count, err := api.PageCountFile(src)
	if err != nil {
		return err
	}
	if count < 1 {
		return nil
	}
	have, err := a.store.PDFPageNumbers(att.ID)
	if err != nil {
		return err
	}
	var missing []string
	for n := 1; n <= count; n++ {
		if !have[n] {
			missing = append(missing, strconv.Itoa(n))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	log.Printf("pdf pages: splitting %s (%d pages)", att.OriginalName, len(missing))

	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	err = api.ExtractPages(f, missing, func(r io.Reader, page int) error {
		body, readErr := io.ReadAll(r)
		if readErr != nil {
			log.Printf("pdf pages: attachment %d page %d: %v", att.ID, page, readErr)
			return nil
		}
		if err := a.storePDFPage(att.ID, page, body); err != nil {
			log.Printf("pdf pages: attachment %d page %d: %v", att.ID, page, err)
		}
		return nil
	}, conf)
	return err
}

func (a *App) storePDFPage(attachmentID int64, page int, body []byte) error {
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	rel := "pdf-pages/" + hash + ".pdf"
	dest, ok := containedPath(a.uploadDir, rel)
	if !ok {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(dest), dirPerm); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(dest, body, filePerm); err != nil {
			return err
		}
	}
	return a.store.UpsertPDFPage(PDFPage{
		AttachmentID: attachmentID,
		PageNumber:   page,
		ContentHash:  hash,
		StoredPath:   rel,
		ContentType:  pdfPageContentType,
	})
}

func isPDFAttachment(att Attachment) bool {
	if strings.EqualFold(att.ContentType, pdfPageContentType) {
		return true
	}
	return strings.HasSuffix(strings.ToLower(att.OriginalName), ".pdf")
}

func (s *Store) CurriculumPDFAttachments() ([]Attachment, error) {
	rows, err := s.db().Query(attachmentSelect+`
		WHERE owner_type = ? AND (
			LOWER(content_type) = ? OR LOWER(original_name) LIKE '%.pdf'
		)
		ORDER BY id`, OwnerCurriculum, pdfPageContentType)
	if err != nil {
		return nil, err
	}
	return scanAttachments(rows)
}

func (s *Store) CurriculumPDFForPlan(planID int64) (Attachment, error) {
	rows, err := s.db().Query(attachmentSelect+`
		WHERE owner_type = ? AND curriculum_plan_id = ? AND (
			LOWER(content_type) = ? OR LOWER(original_name) LIKE '%.pdf'
		)
		ORDER BY id DESC LIMIT 1`, OwnerCurriculum, planID, pdfPageContentType)
	if err != nil {
		return Attachment{}, err
	}
	list, err := scanAttachments(rows)
	if err != nil {
		return Attachment{}, err
	}
	if len(list) == 0 {
		return Attachment{}, sql.ErrNoRows
	}
	return list[0], nil
}

func (s *Store) PDFPageNumbers(attachmentID int64) (map[int]bool, error) {
	rows, err := s.db().Query(
		`SELECT page_number FROM pdf_pages WHERE attachment_id = ?`, attachmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

func (s *Store) UpsertPDFPage(p PDFPage) error {
	_, err := s.db().Exec(`
INSERT INTO pdf_pages (attachment_id, page_number, content_hash, stored_path, content_type)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(attachment_id, page_number) DO UPDATE SET
  content_hash = excluded.content_hash,
  stored_path = excluded.stored_path,
  content_type = excluded.content_type`,
		p.AttachmentID, p.PageNumber, p.ContentHash, p.StoredPath, p.ContentType)
	return err
}

// PDFPageHashes returns one hash per page from start through end.
// The slice is nil when any page in that range has not been stored yet.
func (s *Store) PDFPageHashes(attachmentID int64, start, end int) ([]string, error) {
	if start < 1 || end < start {
		return nil, nil
	}
	rows, err := s.db().Query(`
SELECT page_number, content_hash FROM pdf_pages
WHERE attachment_id = ? AND page_number >= ? AND page_number <= ?
ORDER BY page_number`, attachmentID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	got := map[int]string{}
	for rows.Next() {
		var n int
		var hash string
		if err := rows.Scan(&n, &hash); err != nil {
			return nil, err
		}
		got[n] = hash
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]string, 0, end-start+1)
	for n := start; n <= end; n++ {
		hash := got[n]
		if hash == "" {
			return nil, nil
		}
		out = append(out, hash)
	}
	return out, nil
}

func (s *Store) PDFPageByHash(hash string) (PDFPage, error) {
	var p PDFPage
	err := s.db().QueryRow(`
SELECT attachment_id, page_number, content_hash, stored_path, content_type
FROM pdf_pages WHERE content_hash = ? LIMIT 1`, hash).Scan(
		&p.AttachmentID, &p.PageNumber, &p.ContentHash, &p.StoredPath, &p.ContentType)
	return p, err
}
