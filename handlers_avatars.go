package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// maxAvatarBytes keeps a profile photo to something a phone camera produces
// after the usual downscaling. Anything larger is a scan, not a portrait.
const maxAvatarBytes = 2 << 20

// avatarTypes are the formats every browser this app runs in can display,
// mapped to the extension the file is stored under.
var avatarTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

var (
	errNoPhotoChosen = errors.New("choose a photo first")
	errNotAnImage    = errors.New("that file is not a JPEG, PNG, GIF, or WebP image")
)

func (a *App) handleKidAvatarUpload(w http.ResponseWriter, r *http.Request) {
	kid, err := a.store.Kid(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}

	stored, err := a.saveAvatarUpload(w, r)
	switch {
	case errors.Is(err, errNoPhotoChosen), errors.Is(err, errNotAnImage):
		http.Error(w, err.Error()+".", http.StatusBadRequest)
		return
	case err != nil:
		a.serverError(w, err)
		return
	}

	previous, err := a.store.SetKidAvatar(kid.ID, stored)
	if err != nil {
		a.removeUpload(stored)
		a.serverError(w, err)
		return
	}
	a.removeUpload(previous)
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/settings?saved=photo"))
}

func (a *App) handleKidAvatarDelete(w http.ResponseWriter, r *http.Request) {
	previous, err := a.store.SetKidAvatar(pathID(r, "id"), "")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	a.removeUpload(previous)
	a.redirect(w, r, safeRedirect(r.FormValue("back"), "/settings?saved=photo-removed"))
}

// handleKidAvatarImage serves the photo itself. A child with no photo is a 404
// rather than an error, because the page falls back to their colour dot.
func (a *App) handleKidAvatarImage(w http.ResponseWriter, r *http.Request) {
	kid, err := a.store.Kid(pathID(r, "id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.notFound(w)
			return
		}
		a.serverError(w, err)
		return
	}
	a.serveAvatar(w, r, kid.AvatarPath)
}

func (a *App) serveAvatar(w http.ResponseWriter, r *http.Request, stored string) {
	if stored == "" {
		a.notFound(w)
		return
	}
	path, ok := a.resolveUpload(stored)
	if !ok {
		a.notFound(w)
		return
	}
	if _, err := os.Stat(path); err != nil {
		a.notFound(w)
		return
	}
	// The URL carries a token derived from the stored path, so a cached copy
	// is only ever the photo that URL names.
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, path)
}

// saveAvatarUpload writes the posted photo under uploads/avatars and returns
// its path relative to the upload folder. The format is decided by sniffing
// the file, not by trusting the name or the browser's Content-Type.
func (a *App) saveAvatarUpload(w http.ResponseWriter, r *http.Request) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes)
	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
		return "", errNotAnImage
	}
	defer r.MultipartForm.RemoveAll()

	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 || headers[0].Size == 0 {
		return "", errNoPhotoChosen
	}
	src, err := headers[0].Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	sniff := make([]byte, 512)
	n, err := io.ReadFull(src, sniff)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	ext, ok := avatarTypes[http.DetectContentType(sniff[:n])]
	if !ok {
		return "", errNotAnImage
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	dir := filepath.Join("avatars", time.Now().Format("2006"))
	if err := os.MkdirAll(filepath.Join(a.uploadDir, dir), 0o755); err != nil {
		return "", err
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	stored := filepath.Join(dir, hex.EncodeToString(buf)+ext)

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

// removeUpload deletes a stored file, ignoring one that is already gone.
func (a *App) removeUpload(stored string) {
	if stored == "" {
		return
	}
	if path, ok := a.resolveUpload(stored); ok {
		os.Remove(path)
	}
}
