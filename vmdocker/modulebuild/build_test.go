package modulebuild

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageBuildContext(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "bin", "mybin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "startup.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	profileTOML := []byte("[dockerfile]\nFROM=\"openclaw\"\nbin=\"bin\"\nstartup=\"startup.sh\"\n")
	agentBin := filepath.Join(src, "platform-agent")
	wrapper := filepath.Join(src, "platform-wrapper.sh")
	if err := os.WriteFile(agentBin, []byte("BIN"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctxDir, err := stageBuildContext(BuildOptions{
		ProfileTOML:  profileTOML,
		ProfileDir:   src,
		AgentBinPath: agentBin,
		WrapperPath:  wrapper,
	})
	if err != nil {
		t.Fatalf("stageBuildContext: %v", err)
	}
	defer os.RemoveAll(ctxDir)

	for _, rel := range []string{"Dockerfile", "profile.toml", "bin/mybin", "startup.sh", "platform/vmdocker-agent", "platform/start-vmdocker-agent.sh"} {
		if _, err := os.Stat(filepath.Join(ctxDir, rel)); err != nil {
			t.Errorf("staged context missing %s: %v", rel, err)
		}
	}
}
