package capability

import (
	"os"
	"path/filepath"
	"testing"
)

const claudeExportProfileTOML = `
[dockerfile]
FROM = "docker/sandbox-templates:claude-code"
bin = "bin"
startup = "start.sh"

[vmdocker]
public = ["~/skills/*"]
`

func writeExportFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestExport_ReusesProvidedImage verifies Option A: Export packages the given
// image archive as-is (no docker rebuild — note there is no bin/ or start.sh in
// the workspace) together with a public.zip freshly collected from the live
// workspace.
func TestExport_ReusesProvidedImage(t *testing.T) {
	home := t.TempDir()
	writeExportFile(t, filepath.Join(home, "profile.toml"), claudeExportProfileTOML)
	writeExportFile(t, filepath.Join(home, "skills", "soul.md"), "MY-SOUL")

	res, err := Export(home, ExportOptions{
		ImageArchive: []byte("FAKE-IMAGE-BYTES"),
		ImageName:    "vmdocker-module:test",
		ImageID:      "sha256:deadbeef",
		SignerKey:    "", // empty -> ephemeral signer
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(res.ModuleBytes) == 0 {
		t.Fatal("ModuleBytes empty")
	}
	// public.zip is collected fresh from the live workspace at export time.
	if len(res.Collection.Entries) != 1 || res.Collection.Entries[0].Path != "skills/soul.md" {
		t.Fatalf("collection entries = %+v", res.Collection.Entries)
	}
}

// TestExport_RequiresImageArchive: without an image to reuse there is nothing to
// export (Option A never rebuilds), so it must error rather than produce a
// broken module.
func TestExport_RequiresImageArchive(t *testing.T) {
	home := t.TempDir()
	writeExportFile(t, filepath.Join(home, "profile.toml"), claudeExportProfileTOML)

	_, err := Export(home, ExportOptions{
		ImageName: "x",
		ImageID:   "y",
		SignerKey: "",
	})
	if err == nil {
		t.Fatal("expected error when ImageArchive is empty")
	}
}
