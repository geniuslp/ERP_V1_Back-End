package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveUploadPath(t *testing.T) {
	t.Setenv("UPLOAD_DIR", "/app/uploads")
	root := filepath.Clean("/app/uploads")

	ok := []struct{ name, in, wantRel string }{
		{"uat url", "https://portal.geniuslp.com/erp/uat/uploads/memo/2026/10/1791258969983_S__12566534.jpg", "memo/2026/10/1791258969983_S__12566534.jpg"},
		{"dev url", "http://localhost:8080/uploads/memo/2026/09/1_a.jpg", "memo/2026/09/1_a.jpg"},
		{"leading slash", "/uploads/memo/2026/10/a.jpg", "memo/2026/10/a.jpg"},
		{"no leading slash", "uploads/memo/2026/10/a.jpg", "memo/2026/10/a.jpg"},
		{"spaces encoded", "https://h/erp/uat/uploads/memo/2026/10/my%20file%20(1).jpg", "memo/2026/10/my file (1).jpg"},
		{"thai encoded", "https://h/uploads/memo/2026/10/%E0%B9%84%E0%B8%9F%E0%B8%A5%E0%B9%8C.pdf", "memo/2026/10/ไฟล์.pdf"},
		{"query ignored", "https://h/uploads/memo/a.jpg?x=1", "memo/a.jpg"},
	}
	for _, c := range ok {
		got, err := resolveUploadPath(c.in)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
			continue
		}
		if want := filepath.Join(root, filepath.FromSlash(c.wantRel)); got != want {
			t.Errorf("%s: got %q want %q", c.name, got, want)
		}
	}

	bad := []struct{ name, in string }{
		{"traversal", "https://x/uploads/../../etc/passwd"},
		{"traversal encoded", "https://x/uploads/%2e%2e/%2e%2e/etc/passwd"},
		{"traversal mid", "https://x/uploads/memo/../../../etc/passwd"},
		{"empty", ""},
		{"no uploads segment", "https://x/other/file.jpg"},
		{"empty rel", "https://x/uploads/"},
		{"host only", "https://x"},
	}
	for _, c := range bad {
		if got, err := resolveUploadPath(c.in); err == nil {
			t.Errorf("%s: expected error, got %q", c.name, got)
		}
	}
}

// Real value formats per module: memo/pr/po attachments, stock_item_image, user signature.
func TestResolveUploadPath_ModuleFormats(t *testing.T) {
	t.Setenv("UPLOAD_DIR", "/app/uploads")
	root := filepath.Clean("/app/uploads")
	cases := []struct{ module, in, rel string }{
		{"pr uat url", "https://portal.geniuslp.com/erp/uat/uploads/pr/2026/10/1_a.pdf", "pr/2026/10/1_a.pdf"},
		{"po dev url", "http://localhost:8080/uploads/po/2026/09/1_a.pdf", "po/2026/09/1_a.pdf"},
		{"po relative stored", "uploads/po/2026/09/1_my report.pdf", "po/2026/09/1_my report.pdf"},
		{"stock image relative", "uploads/stock/12/abc.jpg", "stock/12/abc.jpg"},
		{"signature relative", "uploads/signatures/5_x.png", "signatures/5_x.png"},
		{"wo uat url thai+space", "https://portal.geniuslp.com/erp/uat/uploads/wo/2026/10/1_%E0%B9%84%E0%B8%9F%E0%B8%A5%E0%B9%8C%20%E0%B8%97%E0%B8%94%E0%B8%AA%E0%B8%AD%E0%B8%9A.pdf", "wo/2026/10/1_ไฟล์ ทดสอบ.pdf"},
		{"memo slash-rooted", "/uploads/memo/2026/10/a b.jpg", "memo/2026/10/a b.jpg"},
	}
	for _, c := range cases {
		got, err := resolveUploadPath(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.module, err)
			continue
		}
		if want := filepath.Join(root, filepath.FromSlash(c.rel)); got != want {
			t.Errorf("%s: got %q want %q", c.module, got, want)
		}
	}
	for _, in := range []string{
		"uploads/../../etc/passwd",
		"uploads/stock/../../../etc/passwd",
		"https://x/erp/uat/uploads/signatures/../../../../etc/shadow",
	} {
		if _, err := resolveUploadPath(in); err == nil {
			t.Errorf("traversal %q should be rejected", in)
		}
	}
}

func TestToAbsoluteFileURL_NoDoubledPrefix(t *testing.T) {
	old := publicBaseURL
	defer func() { publicBaseURL = old }()

	SetPublicBaseURL("https://portal.geniuslp.com/erp/uat")
	want := "https://portal.geniuslp.com/erp/uat/uploads/memo/2026/10/1_a.jpg"
	for _, in := range []string{
		"uploads/memo/2026/10/1_a.jpg", // stored relative
		"/uploads/memo/2026/10/1_a.jpg",
		"https://portal.geniuslp.com/erp/uat/uploads/memo/2026/10/1_a.jpg", // UAT full URL stored by frontend
		"http://localhost:8080/uploads/memo/2026/10/1_a.jpg",               // dev URL
	} {
		if got := toAbsoluteFileURL(in); got != want {
			t.Errorf("uat: toAbsoluteFileURL(%q) = %q, want %q", in, got, want)
		}
	}

	SetPublicBaseURL("http://localhost:8080")
	if got := toAbsoluteFileURL("https://portal.geniuslp.com/erp/uat/uploads/pr/a.pdf"); got != "http://localhost:8080/uploads/pr/a.pdf" {
		t.Errorf("dev: got %q", got)
	}
	if toAbsoluteFileURL("") != "" {
		t.Error("empty must stay empty")
	}
}

func TestUploadFileHelpers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UPLOAD_DIR", dir)

	if got := uploadURLPath(filepath.Join(uploadDiskDir("memo", "2026", "10"), "1_a.jpg")); got != "uploads/memo/2026/10/1_a.jpg" {
		t.Errorf("uploadURLPath = %q", got)
	}

	d := uploadDiskDir("memo", "2026", "10")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "ไฟล์ ทดสอบ.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	const url = "https://portal.geniuslp.com/erp/uat/uploads/memo/2026/10/%E0%B9%84%E0%B8%9F%E0%B8%A5%E0%B9%8C%20%E0%B8%97%E0%B8%94%E0%B8%AA%E0%B8%AD%E0%B8%9A.txt"

	if err := checkAttachmentOnDisk(url, "x"); err != nil {
		t.Errorf("existing file: %v", err)
	}
	if b, err := readUploadFile(url); err != nil || string(b) != "hi" {
		t.Errorf("readUploadFile = %q, %v", b, err)
	}
	if err := checkAttachmentOnDisk("https://h/uploads/memo/missing.jpg", "x"); err == nil {
		t.Error("missing file must error")
	}
	// Traversal must never delete outside the root.
	outside := filepath.Join(filepath.Dir(dir), "outside.txt")
	_ = os.WriteFile(outside, []byte("keep"), 0o644)
	removeUploadFile("https://x/uploads/../outside.txt")
	if _, err := os.Stat(outside); err != nil {
		t.Error("removeUploadFile deleted a file outside the root")
	}
	removeUploadFile(url)
	if _, err := os.Stat(filepath.Join(d, "ไฟล์ ทดสอบ.txt")); err == nil {
		t.Error("removeUploadFile did not delete the file")
	}
}
