//go:build e2e_realspawn

// Heavyweight, opt-in end-to-end test: real `docker build` -> real signed module
// -> real in-process vmdocker.Spawn -> a real container boots with the adapter
// serving /vmm/health -> the spawned agent's HOME carries the module's public
// content. No hymx node involved.
//
// It is excluded from `go test ./...` by the build tag and skips cleanly when its
// heavy dependencies (docker, a pullable base image, the adapter binary) are
// absent. Run explicitly:
//
//	go test -tags e2e_realspawn ./vmdocker/ -run TestRealBuildSpawn -v
//
// or, via the capability e2e script:
//
//	RUN_REAL_SPAWN=1 bash scripts/e2e_capability.sh
package vmdocker_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cryptowizard0/vmdockerv2/vmdocker"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/capability"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/modulebuild"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager"
	hymxSchema "github.com/hymatrix/hymx/schema"
	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
)

const (
	realSpawnBaseImage = "docker/sandbox-templates:claude-code"
	realSpawnModuleID  = "realspawn"
	realSpawnMarker    = "SOUL-real-build-spawn"
)

func TestRealBuildSpawn(t *testing.T) {
	ctx := context.Background()

	// --- preconditions: skip (never fail) when the heavy deps are absent ---
	if _, err := run(ctx, "docker", "info"); err != nil {
		t.Skip("docker daemon not reachable; skipping real build+spawn e2e")
	}
	agentBin := resolveAdapterBinary(t) // may t.Skip
	if out, err := run(ctx, "docker", "pull", realSpawnBaseImage); err != nil {
		t.Skipf("cannot pull base image %s; skipping. docker said:\n%s", realSpawnBaseImage, out)
	}

	// --- isolate CWD: mod/<id>.json and sandbox_workspace/<pid> resolve here ---
	runDir := t.TempDir()
	t.Chdir(runDir)
	if err := os.MkdirAll("mod", 0o755); err != nil {
		t.Fatalf("mkdir mod: %v", err)
	}

	// --- profile fixture carrying public content ---
	profileDir := t.TempDir()
	writeRealSpawnFixture(t, profileDir)
	profileTOML, err := os.ReadFile(filepath.Join(profileDir, "profile.toml"))
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	publicZip, err := capability.BuildPublicZip(profileDir, []string{"~/skills/*"})
	if err != nil {
		t.Fatalf("build public.zip: %v", err)
	}

	// --- REAL docker build of the module ---
	artifact, err := modulebuild.BuildModuleArtifact(ctx, modulebuild.BuildOptions{
		ProfileTOML:  profileTOML,
		ProfileDir:   profileDir,
		AgentBinPath: agentBin,
		PublicZip:    publicZip,
	})
	if err != nil {
		t.Fatalf("real docker build failed: %v", err)
	}
	moduleBytes, err := capability.SignModuleArtifact(artifact, "")
	if err != nil {
		t.Fatalf("sign module: %v", err)
	}
	if err := os.WriteFile(filepath.Join("mod", "mod-"+realSpawnModuleID+".json"), moduleBytes, 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}

	// --- REAL in-process spawn ---
	pid := "realspawn-" + time.Now().Format("20060102-150405")
	container := runtimemanager.ContainerNamePrefix + pid
	t.Cleanup(func() { _, _ = run(context.Background(), "docker", "rm", "-f", container) })

	env := vmmSchema.Env{
		Meta:    vmmSchema.Meta{ItemId: pid, AccId: "0xE2Etester"},
		Module:  hymxSchema.Module{ModuleFormat: modulebuild.ModuleFormat, Tags: artifact.Tags},
		Process: hymxSchema.Process{Module: realSpawnModuleID, Scheduler: "0xE2Etester"},
	}

	vm, err := vmdocker.Spawn(env)
	if err != nil {
		dumpAdapterLogs(t, container)
		t.Fatalf("real spawn failed: %v", err)
	}
	t.Cleanup(func() { _ = vm.Close() })

	// --- assert: the spawned agent's HOME carries the module's public content ---
	// Read it host-side from the bind-mounted per-pid workspace — backend-agnostic
	// (works for both the Docker and Docker Sandbox backends; a plain `docker exec`
	// would not, since the Sandbox backend uses a truncated name + `docker sandbox exec`).
	seeded := filepath.Join(runDir, "sandbox_workspace", pid, "skills", "soul.md")
	b, err := os.ReadFile(seeded)
	if err != nil {
		dumpAdapterLogs(t, container)
		t.Fatalf("read seeded public file (host workspace %s): %v", seeded, err)
	}
	if got := strings.TrimSpace(string(b)); got != realSpawnMarker {
		t.Fatalf("public content in spawned agent = %q, want %q", got, realSpawnMarker)
	}
	t.Logf("real build + spawn OK: pid %s served /vmm/health 200 and carries skills/soul.md == %q", pid, realSpawnMarker)
}

// writeRealSpawnFixture lays out a minimal buildable profile whose public
// content is a single skills/soul.md marker.
func writeRealSpawnFixture(t *testing.T, dir string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "profile.toml"), []byte(`[dockerfile]
FROM = "docker/sandbox-templates:claude-code"
bin = "bin"
startup = "start.sh"

[vmdocker]
public = ["~/skills/*"]
`), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "bin", ".keep"), []byte("keep\n"), 0o644))
	// start.sh is the author hook; claude readiness is CLI-on-PATH, so a no-op is fine.
	must(os.WriteFile(filepath.Join(dir, "start.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "skills"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "skills", "soul.md"), []byte(realSpawnMarker), 0o644))
}

// resolveAdapterBinary returns a linux adapter binary path, or skips the test.
// It prefers VMDOCKER_AGENT_BIN; otherwise it builds the sibling vmdocker_agent
// repo for linux. Building can fail on that repo's known cross-dev `replace`
// snag — set VMDOCKER_AGENT_BIN to a prebuilt binary to bypass it.
func resolveAdapterBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("VMDOCKER_AGENT_BIN"); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("VMDOCKER_AGENT_BIN=%s not found: %v", p, err)
		}
		return p
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repo := os.Getenv("VMDOCKER_AGENT_REPO")
	candidates := []string{repo}
	if repo == "" {
		candidates = []string{
			filepath.Join(cwd, "..", "..", "vmdocker_agent"),
			filepath.Join(cwd, "..", "vmdocker_agent"),
		}
	}
	var repoDir string
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "main.go")); err == nil {
			repoDir = c
			break
		}
	}
	if repoDir == "" {
		t.Skip("no VMDOCKER_AGENT_BIN and sibling vmdocker_agent repo not found; skipping")
	}

	arch := os.Getenv("VMDOCKER_AGENT_GOARCH")
	if arch == "" {
		arch = runtime.GOARCH
	}
	out := filepath.Join(t.TempDir(), "vmdocker-agent")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("adapter build failed (set VMDOCKER_AGENT_BIN to a prebuilt linux binary): %v\n%s", err, b)
	}
	return out
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(b), err
}

func dumpAdapterLogs(t *testing.T, container string) {
	t.Helper()
	if out, err := run(context.Background(), "docker", "logs", "--tail", "80", container); err == nil {
		t.Logf("adapter/start.sh logs for %s:\n%s", container, out)
	}
}
