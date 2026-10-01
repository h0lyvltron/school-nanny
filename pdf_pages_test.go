package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestSplitCurriculumPDFKeepsLessons(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Ada")
	subject, err := ta.store.CreateSubject("Math", "#336699")
	if err != nil {
		t.Fatal(err)
	}
	lessonID, err := ta.store.CreateLesson(Lesson{
		KidID:       kid,
		SubjectID:   subject,
		Title:       "Fractions",
		ScheduledOn: today(),
		PageStart:   1,
		PageEnd:     2,
		Status:      StatusPlanned,
	})
	if err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(t.TempDir(), "book.pdf")
	if err := writeTwoPagePDF(src); err != nil {
		t.Fatal(err)
	}
	rel := "book.pdf"
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(ta.uploadDir, rel)
	if err := os.MkdirAll(ta.uploadDir, dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, body, filePerm); err != nil {
		t.Fatal(err)
	}
	id, err := ta.store.CreateAttachment(Attachment{
		OwnerType:    OwnerCurriculum,
		OriginalName: "book.pdf",
		StoredPath:   rel,
		SizeBytes:    int64(len(body)),
		ContentType:  "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	att, err := ta.store.Attachment(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := ta.splitCurriculumPDF(att); err != nil {
		t.Fatal(err)
	}

	hashes, err := ta.store.PDFPageHashes(id, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 || hashes[0] == "" || hashes[1] == "" {
		t.Fatalf("hashes: %#v", hashes)
	}
	if hashes[0] == hashes[1] {
		t.Fatal("distinct pages hashed the same")
	}
	for _, hash := range hashes {
		if _, err := os.Stat(filepath.Join(ta.uploadDir, "pdf-pages", hash+".pdf")); err != nil {
			t.Fatal(err)
		}
		code, pageBody := ta.get("/files/pages/" + hash)
		if code != 200 || !strings.HasPrefix(pageBody, "%PDF") {
			t.Fatalf("page %s: %d %q", hash, code, pageBody[:min(20, len(pageBody))])
		}
	}

	got, err := ta.store.Lesson(lessonID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Fractions" || got.PageStart != 1 || got.PageEnd != 2 || got.Status != StatusPlanned {
		t.Fatalf("lesson changed: %+v", got)
	}
	again, err := ta.store.Attachment(id)
	if err != nil {
		t.Fatal(err)
	}
	if again.StoredPath != rel {
		t.Fatalf("original path changed: %s", again.StoredPath)
	}

	// A second pass finds nothing left to extract.
	if err := ta.splitCurriculumPDF(att); err != nil {
		t.Fatal(err)
	}
	numbers, err := ta.store.PDFPageNumbers(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(numbers) != 2 {
		t.Fatalf("pages: %v", numbers)
	}
}

func writeTwoPagePDF(path string) error {
	conf := model.NewDefaultConfiguration()
	xRef, err := pdfcpu.CreateResourceDictInheritanceDemoXRef()
	if err != nil {
		return err
	}
	ctx := pdfcpu.CreateContext(xRef, conf)
	one := path + ".one"
	if err := api.WriteContextFile(ctx, one); err != nil {
		return err
	}
	defer os.Remove(one)
	pageConf, err := pdfcpu.ParsePageConfiguration("form:A4", conf.Unit)
	if err != nil {
		return err
	}
	return api.InsertPagesFile(one, path, []string{"1"}, false, pageConf, conf)
}
