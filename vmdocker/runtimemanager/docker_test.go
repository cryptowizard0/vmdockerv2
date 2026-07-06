package runtimemanager

import (
	"testing"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
)

func TestBuildDockerHostConfigPinsSwapToMemory(t *testing.T) {
	hc := buildDockerHostConfig(12345, t.TempDir())
	if hc.Resources.MemorySwap != int64(schema.MaxMem) {
		t.Fatalf("MemorySwap = %d, want %d (== Memory)", hc.Resources.MemorySwap, int64(schema.MaxMem))
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
