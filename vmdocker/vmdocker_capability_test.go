package vmdocker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/capability"
	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
)

func TestApplyExportDryRunPreview(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "profile.toml"), []byte("[dockerfile]\nFROM=\"docker/sandbox-templates:shell\"\n[vmdocker]\npublic=[\"~/skills/*\"]\n"), 0o644); err != nil {
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
