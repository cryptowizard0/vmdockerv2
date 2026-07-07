package capability

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPublicZip_RoundTrip(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "a.md"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}

	zb, err := BuildPublicZip(home, []string{"~/skills/*"})
	if err != nil {
		t.Fatalf("BuildPublicZip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		t.Fatalf("zip open: %v", err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "skills/a.md" {
		t.Fatalf("zip files = %v", zr.File)
	}
	f, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A" {
		t.Fatalf("content = %q", got)
	}
}

func TestPreview_NoBuild(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "a.md"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := Preview(home, []string{"~/skills/*"})
	if err != nil {
		t.Fatal(err)
	}
	if col.TotalBytes != 1 || len(col.Entries) != 1 {
		t.Fatalf("preview = %+v", col)
	}
}

func zipWith(t *testing.T, build func(*zip.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	build(zw)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var ce *CodedError
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func TestUnpackPublicZip_HappyPath(t *testing.T) {
	dst := t.TempDir()
	z := zipWith(t, func(zw *zip.Writer) {
		w, _ := zw.Create("skills/a.md")
		_, _ = w.Write([]byte("A"))
		w2, _ := zw.Create("note.md")
		_, _ = w2.Write([]byte("N"))
	})
	if err := UnpackPublicZip(dst, z); err != nil {
		t.Fatalf("UnpackPublicZip: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "skills", "a.md")); string(b) != "A" {
		t.Fatalf("skills/a.md = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "note.md")); string(b) != "N" {
		t.Fatalf("note.md = %q", b)
	}
}

func TestUnpackPublicZip_RejectsParentEscape(t *testing.T) {
	dst := t.TempDir()
	z := zipWith(t, func(zw *zip.Writer) {
		w, _ := zw.Create("../escape.md")
		_, _ = w.Write([]byte("x"))
	})
	assertCode(t, UnpackPublicZip(dst, z), "PATH_ESCAPE")
	if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "escape.md")); err == nil {
		t.Fatal("escape file was written outside dst")
	}
}

func TestUnpackPublicZip_RejectsAbsolute(t *testing.T) {
	dst := t.TempDir()
	z := zipWith(t, func(zw *zip.Writer) {
		w, _ := zw.Create("/etc/evil")
		_, _ = w.Write([]byte("x"))
	})
	assertCode(t, UnpackPublicZip(dst, z), "PATH_ESCAPE")
}

func TestUnpackPublicZip_RejectsSymlink(t *testing.T) {
	dst := t.TempDir()
	z := zipWith(t, func(zw *zip.Writer) {
		h := &zip.FileHeader{Name: "link"}
		h.SetMode(os.ModeSymlink | 0o777)
		w, _ := zw.CreateHeader(h)
		_, _ = w.Write([]byte("/etc/passwd"))
	})
	assertCode(t, UnpackPublicZip(dst, z), "PATH_ESCAPE")
}

func TestUnpackPublicZip_RejectsCorrupt(t *testing.T) {
	assertCode(t, UnpackPublicZip(t.TempDir(), []byte("not a zip")), "CORRUPT")
}

func TestUnpackPublicZip_RejectsOversize(t *testing.T) {
	dst := t.TempDir()
	z := zipWith(t, func(zw *zip.Writer) {
		w, _ := zw.Create("big")
		_, _ = w.Write(bytes.Repeat([]byte("a"), 500))
	})
	assertCode(t, unpackPublicZip(dst, z, 100), "TOO_LARGE")
}
