package handlers

import (
	"fmt"

	"errors"
	"github.com/gofiber/fiber/v2"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// errBadUploadPath is returned by resolveUploadPath for values that are empty, absolute,
// or try to leave the upload root.
var errBadUploadPath = errors.New("invalid upload path")

// uploadRoot is the single source of truth for where uploaded files live on disk. Upload
// endpoints write under it, the static /uploads route serves it, and attachment existence
// checks resolve against it, so they can never disagree.
//
// UPLOAD_DIR overrides it. The default "uploads" is relative to the working directory, which
// is /app/uploads in the Docker image (WORKDIR /app) and ./uploads on a dev machine — the
// same location every writer used before this helper existed.
func uploadRoot() string {
	if v := strings.TrimSpace(os.Getenv("UPLOAD_DIR")); v != "" {
		return v
	}
	return "uploads"
}

// uploadDiskDir returns uploadRoot()/sub... for writers.
func uploadDiskDir(sub ...string) string {
	return filepath.Join(append([]string{uploadRoot()}, sub...)...)
}

// uploadURLPath turns a disk path under uploadRoot() into the stable "uploads/<rel>" form that
// is stored in the DB and absolutized for clients (independent of where the root is mounted).
func uploadURLPath(diskPath string) string {
	if rel, err := filepath.Rel(uploadRoot(), diskPath); err == nil && !strings.HasPrefix(rel, "..") {
		return "uploads/" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(diskPath)
}

// uploadRelFromValue extracts the cleaned path after the first "/uploads/" segment of a full
// URL ("https://host/any/prefix/uploads/memo/x.jpg"), "/uploads/..." or "uploads/..." value.
// It does not depend on scheme, host or any base-URL prefix.
func uploadRelFromValue(value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", errBadUploadPath
	}
	// Drop query/fragment, and the scheme://host part of a full URL.
	if i := strings.IndexAny(v, "?#"); i >= 0 {
		v = v[:i]
	}
	if _, rest, ok := strings.Cut(v, "://"); ok {
		if _, p, ok := strings.Cut(rest, "/"); ok {
			v = "/" + p
		} else {
			return "", errBadUploadPath
		}
	}
	v = strings.ReplaceAll(v, "\\", "/")
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	_, after, ok := strings.Cut(v, "/uploads/")
	if !ok {
		return "", errBadUploadPath
	}
	decoded, err := url.PathUnescape(after)
	if err != nil {
		decoded = after
	}
	// Reject traversal before cleaning, so "a/../b" can't be silently normalized into range.
	for _, seg := range strings.Split(decoded, "/") {
		if seg == ".." {
			return "", errBadUploadPath
		}
	}
	rel := path.Clean(decoded)
	if rel == "." || rel == "" || path.IsAbs(rel) || strings.ContainsRune(rel, 0) {
		return "", errBadUploadPath
	}
	return rel, nil
}

// resolveUploadPath maps a stored/sent attachment file_path to an absolute-or-root-relative
// disk path inside uploadRoot(), or errBadUploadPath if it is empty, absolute, contains ".."
// or would escape the root.
func resolveUploadPath(value string) (string, error) {
	rel, err := uploadRelFromValue(value)
	if err != nil {
		return "", err
	}
	root := filepath.Clean(uploadRoot())
	full := filepath.Join(root, filepath.FromSlash(rel))
	back, err := filepath.Rel(root, full)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
		return "", errBadUploadPath
	}
	return full, nil
}

// checkAttachmentOnDisk validates a client-supplied attachment file_path and verifies the file
// exists under uploadRoot(). Bad/unsafe values get a plain Thai 400; genuinely missing files keep
// the original "was not found on disk" message.
func checkAttachmentOnDisk(filePath, fileName string) error {
	full, err := resolveUploadPath(filePath)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("พาธไฟล์แนบ %q ไม่ถูกต้อง กรุณาอัปโหลดไฟล์ใหม่อีกครั้ง", fileName))
	}
	if _, err := os.Stat(full); err != nil {
		return fiber.NewError(fiber.StatusBadRequest,
			fmt.Sprintf("attachment %q was not found on disk — please re-upload", fileName))
	}
	return nil
}

// UploadRoot exposes uploadRoot for the static file route in internal/routes.
func UploadRoot() string { return uploadRoot() }

// readUploadFile reads a stored/sent file_path (URL or relative) from under uploadRoot().
func readUploadFile(value string) ([]byte, error) {
	full, err := resolveUploadPath(value)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

// removeUploadFile best-effort deletes a stored/sent file_path under uploadRoot(). Values that
// don't resolve inside the root are ignored, never deleted.
func removeUploadFile(value string) {
	if full, err := resolveUploadPath(value); err == nil {
		os.Remove(full)
	}
}
