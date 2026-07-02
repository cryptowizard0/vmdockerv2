package vmdocker

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/capability"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/modulebuild"
	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
	arSchema "github.com/permadao/goar/schema"
	arutils "github.com/permadao/goar/utils"
)

func TestApplyExportDryRunPreview(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "profile.toml"), []byte("[dockerfile]\nFROM=\"openclaw\"\n[vmdocker]\npublic=[\"skills/\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "a.md"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	vm := &VmDocker{
		pid:       "pid-preview",
		closeChan: make(chan struct{}),
		instanceInfo: &runtimeSchema.InstanceInfo{
			Workspace: home,
			Backend:   runtimeSchema.RuntimeBackendDocker,
		},
	}

	res := vm.Apply("caller", vmmSchema.Meta{
		Action: "Export",
		Params: map[string]string{"dry_run": "true"},
	})
	if res.Error != nil {
		t.Fatalf("Apply Export dry_run error: %v", res.Error)
	}
	col, ok := res.Output.(capability.Collection)
	if !ok {
		t.Fatalf("Output = %T, want capability.Collection", res.Output)
	}
	if len(col.Entries) != 1 || col.Entries[0].Path != "skills/a.md" {
		t.Fatalf("preview entries = %+v", col.Entries)
	}
	if res.Data != "" {
		t.Fatalf("dry_run must not return module data")
	}
}

func TestApplyImportHostSide(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "profile.toml"), []byte("[dockerfile]\nFROM=\"openclaw\"\n[vmdocker]\npublic=[\"skills/\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vm := &VmDocker{
		pid:       "pid-import",
		closeChan: make(chan struct{}),
		instanceInfo: &runtimeSchema.InstanceInfo{
			Workspace: home,
			Backend:   runtimeSchema.RuntimeBackendDocker,
		},
	}
	moduleBytes := buildCapabilityTestModule(t, map[string]string{"skills/imported.md": "imported"})

	res := vm.Apply("caller", vmmSchema.Meta{
		Action: "Import",
		Data:   arutils.Base64Encode(moduleBytes),
	})
	if res.Error != nil {
		t.Fatalf("Apply Import error: %v", res.Error)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "skills", "imported.md")); string(b) != "imported" {
		t.Fatalf("imported content = %q", b)
	}
	if _, ok := res.Output.(capability.ImportResult); !ok {
		t.Fatalf("Output = %T, want capability.ImportResult", res.Output)
	}
}

func buildCapabilityTestModule(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
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
	writeM("profile.toml", []byte("[vmdocker]\npublic=[\"skills/\"]\n"))
	writeM("public.zip", zipBuf.Bytes())
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	item := arSchema.BundleItem{
		Tags: []arSchema.Tag{{Name: "Module-Format", Value: modulebuild.ModuleFormat}},
		Data: arutils.Base64Encode(tarBuf.Bytes()),
	}
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
