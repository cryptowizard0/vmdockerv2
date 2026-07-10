package runtimemanager

import (
	"testing"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	"github.com/docker/docker/api/types/mount"
)

func TestBuildDockerHostConfigPinsSwapToMemory(t *testing.T) {
	hc := buildDockerHostConfig(12345, t.TempDir(), t.TempDir())
	if hc.Resources.MemorySwap != int64(schema.MaxMem) {
		t.Fatalf("MemorySwap = %d, want %d (== Memory)", hc.Resources.MemorySwap, int64(schema.MaxMem))
	}
}

func TestBuildDockerHostConfigMountsWritableTmp(t *testing.T) {
	workspace := t.TempDir()
	tmpDir := t.TempDir()
	hc := buildDockerHostConfig(12345, workspace, tmpDir)

	if !hc.ReadonlyRootfs {
		t.Fatalf("ReadonlyRootfs = false, want true (the /tmp mount is what makes /tmp writable)")
	}

	var tmpMount *mount.Mount
	for i := range hc.Mounts {
		if hc.Mounts[i].Target == containerTmp {
			tmpMount = &hc.Mounts[i]
			break
		}
	}
	if tmpMount == nil {
		t.Fatalf("no bind mount targeting %q; mounts = %+v", containerTmp, hc.Mounts)
	}
	if tmpMount.Type != mount.TypeBind {
		t.Fatalf("/tmp mount type = %q, want bind", tmpMount.Type)
	}
	if tmpMount.Source != tmpDir {
		t.Fatalf("/tmp mount source = %q, want %q", tmpMount.Source, tmpDir)
	}
	if tmpMount.ReadOnly {
		t.Fatalf("/tmp mount is read-only, want writable")
	}
}

func TestBuildDockerContainerConfigRunsNonRoot(t *testing.T) {
	spec := schema.RuntimeSpec{Image: schema.ImageInfo{Name: "img"}}
	cfg, err := buildDockerContainerConfig(spec, nil, []string{"/app/main"}, t.TempDir())
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	if cfg.User != schema.RuntimeUser {
		t.Fatalf("User = %q, want %q", cfg.User, schema.RuntimeUser)
	}
	if schema.RuntimeUser == "" || schema.RuntimeUser == "root" || schema.RuntimeUser == "0" {
		t.Fatalf("RuntimeUser must be a non-root user, got %q", schema.RuntimeUser)
	}
}
