package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHowToSignature(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Unix(1_000_000, 0)
	exp := now.Add(15 * time.Minute).Unix()
	sig := howToSign(secret, 7, 42, exp)

	if !howToVerify(secret, 7, 42, exp, sig, now) {
		t.Fatal("valid signature rejected")
	}
	if howToVerify(secret, 7, 42, exp, sig, now.Add(16*time.Minute)) {
		t.Fatal("expired signature accepted")
	}
	if howToVerify(secret, 8, 42, exp, sig, now) {
		t.Fatal("signature for another video accepted")
	}
	if howToVerify(secret, 7, 43, exp, sig, now) {
		t.Fatal("signature for another user accepted")
	}
	if howToVerify(secret, 7, 42, exp+60, sig, now) {
		t.Fatal("tampered expiry accepted")
	}
	if howToVerify([]byte("other"), 7, 42, exp, sig, now) {
		t.Fatal("signature under another secret accepted")
	}
	if howToVerify(secret, 7, 42, exp, "", now) {
		t.Fatal("empty signature accepted")
	}
}

func TestHowToVideoKind(t *testing.T) {
	mp4 := append([]byte{0, 0, 0, 0x18}, []byte("ftypmp42\x00\x00\x00\x00mp42isom")...)
	webm := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01, 0x42, 0xF7, 0x81, 0x01, 0x42, 0xF2, 0x81, 0x04, 0x42, 0xF3, 0x81, 0x08, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'}

	if ext, mime, ok := howToVideoKind(mp4, "a.MP4"); !ok || ext != ".mp4" || mime != "video/mp4" {
		t.Fatalf("mp4 not recognised: %v %v %v", ext, mime, ok)
	}
	if ext, mime, ok := howToVideoKind(webm, "a.webm"); !ok || ext != ".webm" || mime != "video/webm" {
		t.Fatalf("webm not recognised: %v %v %v", ext, mime, ok)
	}
	if _, _, ok := howToVideoKind([]byte("%PDF-1.4 not a video"), "fake.mp4"); ok {
		t.Fatal("a PDF renamed to .mp4 was accepted")
	}
	if _, _, ok := howToVideoKind(mp4, "a.mov"); ok {
		t.Fatal("unsupported extension accepted")
	}
	if _, _, ok := howToVideoKind(mp4, "a.webm"); ok {
		t.Fatal("mp4 bytes with .webm extension accepted")
	}
}

func TestHowToDiskPathStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("UPLOAD_DIR", root)
	if p, err := howToDiskPath("howto/1/x.mp4"); err != nil || p != filepath.Join(root, "howto", "1", "x.mp4") {
		t.Fatalf("normal path: %v %v", p, err)
	}
	for _, bad := range []string{"../etc/passwd", "howto/../../x", "..", "../" + filepath.Base(root) + "-evil/x"} {
		if _, err := howToDiskPath(bad); err == nil {
			t.Fatalf("path %q escaped the upload root", bad)
		}
	}
	_ = os.Remove(root)
}
