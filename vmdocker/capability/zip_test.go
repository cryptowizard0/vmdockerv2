package capability

import (
	"archive/zip"
	"bytes"
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
