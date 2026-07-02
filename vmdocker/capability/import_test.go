package capability

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	arSchema "github.com/permadao/goar/schema"
	arutils "github.com/permadao/goar/utils"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZipEntries(t *testing.T, entries ...struct{ name, body string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		w, err := zw.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestApplyPublicZip_WhitelistAndConflict(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "keep.md"), []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}

	zb := makeZip(t, map[string]string{
		"skills/new.md":  "new",
		"skills/keep.md": "overwritten",
	})

	res, err := applyPublicZip(home, zb, []string{"skills/"}, ImportOptions{OnConflict: "skip"})
	if err != nil {
		t.Fatalf("applyPublicZip: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "skills", "keep.md")); string(b) != "orig" {
		t.Errorf("keep.md should be skipped, got %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "skills", "new.md")); string(b) != "new" {
		t.Errorf("new.md not imported: %q", b)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "skills/keep.md" {
		t.Errorf("skipped = %v", res.Skipped)
	}
}

func TestApplyPublicZip_RejectsOutsideWhitelist(t *testing.T) {
	home := t.TempDir()
	zb := makeZip(t, map[string]string{"private/secret": "x"})
	_, err := applyPublicZip(home, zb, []string{"skills/"}, ImportOptions{OnConflict: "skip"})
	if err == nil {
		t.Fatal("expected UNAUTHORIZED_PATH for path outside target public whitelist")
	}
}

func TestApplyPublicZip_RejectsTraversal(t *testing.T) {
	home := t.TempDir()
	zb := makeZip(t, map[string]string{"skills/../../etc/x": "x"})
	_, err := applyPublicZip(home, zb, []string{"skills/"}, ImportOptions{OnConflict: "skip"})
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestApplyPublicZip_AllowsWhitelistedFile(t *testing.T) {
	home := t.TempDir()
	zb := makeZip(t, map[string]string{"note.md": "N"})
	res, err := applyPublicZip(home, zb, []string{"note.md"}, ImportOptions{OnConflict: "skip"})
	if err != nil {
		t.Fatalf("applyPublicZip: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "note.md")); string(b) != "N" {
		t.Fatalf("note.md not imported: %q", b)
	}
	if len(res.Imported) != 1 || res.Imported[0] != "note.md" {
		t.Fatalf("imported = %v", res.Imported)
	}
}

func TestApplyPublicZip_RejectsFileOutsideWhitelist(t *testing.T) {
	home := t.TempDir()
	zb := makeZip(t, map[string]string{"note.md": "N"})
	_, err := applyPublicZip(home, zb, []string{"skills/"}, ImportOptions{OnConflict: "skip"})
	if err == nil {
		t.Fatal("expected UNAUTHORIZED_PATH for file not covered by any public entry")
	}
}

func TestApplyPublicZip_OverwriteRollbackRestoresOriginal(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "keep.md"), []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}

	zb := makeZipEntries(t,
		struct{ name, body string }{"skills/keep.md", "new"},
		struct{ name, body string }{"private/secret", "x"},
	)
	_, err := applyPublicZip(home, zb, []string{"skills/"}, ImportOptions{OnConflict: "overwrite"})
	if err == nil {
		t.Fatal("expected unauthorized path error")
	}
	if b, _ := os.ReadFile(filepath.Join(home, "skills", "keep.md")); string(b) != "orig" {
		t.Fatalf("rollback content = %q, want orig", b)
	}
}

func TestApplyPublicZip_RejectsExpandedContentOverLimit(t *testing.T) {
	home := t.TempDir()
	body := strings.Repeat("x", 128*1024)
	zb := makeZip(t, map[string]string{"skills/large.md": body})
	if len(zb) > 4096 {
		t.Fatalf("test zip compressed size = %d, want <= 4096", len(zb))
	}

	_, err := applyPublicZip(home, zb, []string{"skills/"}, ImportOptions{
		OnConflict: "skip",
		MaxBytes:   4096,
	})
	if err == nil {
		t.Fatal("expected TOO_LARGE for expanded zip content")
	}
	if _, statErr := os.Stat(filepath.Join(home, "skills", "large.md")); !os.IsNotExist(statErr) {
		t.Fatalf("large file should not be written, stat err = %v", statErr)
	}
}

func buildTestModule(t *testing.T, publicZip []byte, format string) []byte {
	return buildTestModuleWithProfile(t, publicZip, format, []byte("[vmdocker]\npublic=[\"skills/\"]\n"))
}

func buildTestModuleWithProfile(t *testing.T, publicZip []byte, format string, profile []byte) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)
	writeM := func(name string, data []byte) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	writeM("profile.toml", profile)
	writeM("public.zip", publicZip)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	item := arSchema.BundleItem{
		Tags: []arSchema.Tag{
			{Name: "Data-Protocol", Value: "hymx"},
			{Name: "Variant", Value: "v0.1.0"},
			{Name: "Type", Value: "Module"},
			{Name: "Module-Format", Value: format},
		},
		Data: arutils.Base64Encode(tarBuf.Bytes()),
	}
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestImport_HappyPath(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "profile.toml"), []byte("[vmdocker]\npublic=[\"skills/\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	moduleBytes := buildTestModule(t, makeZip(t, map[string]string{"skills/x.md": "X"}), moduleFormat)
	res, err := Import(home, moduleBytes, ImportOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "skills", "x.md")); string(b) != "X" {
		t.Fatalf("imported content = %q", b)
	}
	if len(res.Imported) != 1 {
		t.Fatalf("imported = %v", res.Imported)
	}
	if res.ProfileUpdated {
		t.Fatal("profile must not be updated by import by default")
	}
}

func TestImport_DoesNotOverwriteTargetProfile(t *testing.T) {
	home := t.TempDir()
	targetProfile := []byte("[vmdocker]\npublic=[\"skills/\"]\n")
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "profile.toml"), targetProfile, 0o644); err != nil {
		t.Fatal(err)
	}
	moduleProfile := []byte("[vmdocker]\npublic=[\"skills/\",\"private/\"]\n")
	moduleBytes := buildTestModuleWithProfile(t, makeZip(t, map[string]string{"skills/x.md": "X"}), moduleFormat, moduleProfile)

	res, err := Import(home, moduleBytes, ImportOptions{OnConflict: "overwrite"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.ProfileUpdated {
		t.Fatal("profile must not be updated")
	}
	if got, _ := os.ReadFile(filepath.Join(home, "profile.toml")); !bytes.Equal(got, targetProfile) {
		t.Fatalf("target profile was overwritten:\n%s", string(got))
	}
}

func TestImport_RejectsModuleMemberOverLimit(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "profile.toml"), []byte("[vmdocker]\npublic=[\"skills/\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moduleProfile := []byte(strings.Repeat("x", 8192))
	moduleBytes := buildTestModuleWithProfile(t, makeZip(t, map[string]string{"skills/x.md": "X"}), moduleFormat, moduleProfile)

	_, err := Import(home, moduleBytes, ImportOptions{MaxModuleBytes: 4096})
	if err == nil {
		t.Fatal("expected TOO_LARGE for oversized module member")
	}
}

func TestImport_FormatMismatch(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "profile.toml"), []byte("[vmdocker]\npublic=[\"skills/\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moduleBytes := buildTestModule(t, makeZip(t, map[string]string{"skills/x.md": "X"}), "wrong")
	if _, err := Import(home, moduleBytes, ImportOptions{}); err == nil {
		t.Fatal("expected FORMAT_MISMATCH")
	}
}
