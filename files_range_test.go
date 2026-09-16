package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestDetectUploadContentTypePDFMagic(t *testing.T) {
	body := []byte("%PDF-1.4\n% rest of a pretend file")
	ct, reader, err := detectUploadContentType("x.bin", "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if ct != "application/pdf" {
		t.Fatalf("ct=%q", ct)
	}
	got, _ := io.ReadAll(reader)
	if !bytes.Equal(got, body) {
		t.Fatalf("round-trip bytes mismatch")
	}
}

func TestDetectUploadContentTypePDFExtension(t *testing.T) {
	body := []byte("not magic but named pdf")
	ct, _, err := detectUploadContentType("lesson.pdf", "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if ct != "application/pdf" {
		t.Fatalf("ct=%q want application/pdf from extension", ct)
	}
}

func TestFileDownloadSupportsRange(t *testing.T) {
	ta := newTestApp(t)
	kid := ta.addKid("Mia")
	subject := ta.mathSubjectID()

	payload := []byte("%PDF-1.4\n" + strings.Repeat("0123456789", 20))
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("owner_type", OwnerResource)
	writer.WriteField("kid_id", itoa64(kid))
	writer.WriteField("subject_id", itoa64(subject))
	writer.WriteField("back", "/")
	part, err := writer.CreateFormFile("file", "book.pdf")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(payload)
	writer.Close()

	resp, err := ta.client.Post(ta.server.URL+"/files", writer.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	files, err := ta.store.ResourceAttachments(kid, subject)
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%d err=%v", len(files), err)
	}
	if files[0].ContentType != "application/pdf" {
		t.Fatalf("content type=%q", files[0].ContentType)
	}

	req, err := http.NewRequest(http.MethodGet, ta.server.URL+"/files/"+itoa64(files[0].ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-11")
	rangeResp, err := ta.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rangeResp.Body.Close()
	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status=%d want 206", rangeResp.StatusCode)
	}
	if got := rangeResp.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges=%q", got)
	}
	if got := rangeResp.Header.Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control=%q", got)
	}
	etag := rangeResp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	body, _ := io.ReadAll(rangeResp.Body)
	if string(body) != string(payload[:12]) {
		t.Fatalf("range body=%q", body)
	}
	if ct := rangeResp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type=%q", ct)
	}

	cachedReq, err := http.NewRequest(http.MethodGet, ta.server.URL+"/files/"+itoa64(files[0].ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	cachedReq.Header.Set("If-None-Match", etag)
	cachedResp, err := ta.client.Do(cachedReq)
	if err != nil {
		t.Fatal(err)
	}
	defer cachedResp.Body.Close()
	if cachedResp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional status=%d want 304", cachedResp.StatusCode)
	}
}
