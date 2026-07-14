package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const testProfileTOML = `
[dockerfile]
FROM = "docker/sandbox-templates:claude-code"
bin = "bin"
CMD = ["claude", "--serve"]

[vmdocker]
public = ["~/skills/*", "~/persona/*"]
`

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCollectProfilePublicZip verifies the build flow collects the profile
// directory's [vmdocker].public files into public.zip (the bug: they used to be
// dropped, so a built module shipped empty public state).
func TestCollectProfilePublicZip(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "skills", "soul.md"), "MY-SOUL")
	mustWriteFile(t, filepath.Join(dir, "persona", "style.md"), "terse")
	// a file outside the allowlist must NOT be collected
	mustWriteFile(t, filepath.Join(dir, "secret.txt"), "nope")

	zipBytes, col, err := collectProfilePublicZip(dir, []byte(testProfileTOML))
	if err != nil {
		t.Fatalf("collectProfilePublicZip: %v", err)
	}

	// collection preview lists exactly the two public files
	got := map[string]bool{}
	for _, e := range col.Entries {
		got[e.Path] = true
	}
	if len(got) != 2 || !got["skills/soul.md"] || !got["persona/style.md"] {
		t.Fatalf("collection entries = %+v, want skills/soul.md + persona/style.md", col.Entries)
	}
	if got["secret.txt"] {
		t.Fatal("secret.txt outside allowlist must not be collected")
	}

	// public.zip contains exactly those two members
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if len(names) != 2 || !names["skills/soul.md"] || !names["persona/style.md"] {
		t.Fatalf("zip members = %v, want skills/soul.md + persona/style.md", names)
	}
}

// TestCollectProfilePublicZip_Empty: no matching files -> empty zip, no error.
func TestCollectProfilePublicZip_Empty(t *testing.T) {
	dir := t.TempDir()
	zipBytes, col, err := collectProfilePublicZip(dir, []byte(testProfileTOML))
	if err != nil {
		t.Fatalf("collectProfilePublicZip: %v", err)
	}
	if len(col.Entries) != 0 {
		t.Fatalf("expected no entries, got %+v", col.Entries)
	}
	if zipBytes == nil {
		t.Fatal("expected a (possibly empty) zip, got nil")
	}
}
