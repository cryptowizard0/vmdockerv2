//go:build e2e_realspawn

// Heavyweight, opt-in end-to-end test: real `docker build` -> real signed module
// -> real in-process vmdocker.Spawn -> a real container boots with the adapter
// serving /vmm/health, the image CMD runs, and the spawned agent's HOME carries
// the module's public content. No hymx node involved.
//
// It is excluded from `go test ./...` by the build tag and skips cleanly when its
// heavy dependencies (docker, a pullable base image, the adapter binary) are
// absent. Run explicitly (point VMDOCKER_AGENT_BIN at an adapter built from the
// ticket-02 branch of the sibling vmdocker_agent repo):
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
	realSpawnCmdMarker = "CMD-real-build-spawn"
)

// TestRealBuildSpawn proves a profile [dockerfile].CMD actually executes at
// runtime: the image is built with a CMD that writes a marker into HOME, and the
// host observes both the seeded public content (spawn + serve OK) and the marker
// the CMD produced (the command ran under the adapter).
func TestRealBuildSpawn(t *testing.T) {
	ctx := context.Background()
	agentBin := realSpawnPreconditions(t, ctx)

	ws := buildSpawnAndWorkspace(t, ctx, agentBin, "cmd", func(dir string) {
		// The CMD writes a marker into HOME (the bind-mounted per-pid workspace),
		// so the host can observe that the adapter actually ran the image CMD.
		writeRealSpawnFixture(t, dir, `CMD = ["sh", "-c", "echo `+realSpawnCmdMarker+` > $HOME/cmd-ran.txt"]`)
	})

	// spawn + serve: the module's public content is seeded into HOME.
	assertFileEventually(t, filepath.Join(ws, "skills", "soul.md"), realSpawnMarker)
	// the image CMD executed: it wrote its marker into HOME.
	assertFileEventually(t, filepath.Join(ws, "cmd-ran.txt"), realSpawnCmdMarker)
}

// TestRealBuildSpawnNoCMD proves a module that declares no CMD still spawns and
// serves: the adapter runs no user command, and the public content is still seeded.
func TestRealBuildSpawnNoCMD(t *testing.T) {
	ctx := context.Background()
	agentBin := realSpawnPreconditions(t, ctx)

	ws := buildSpawnAndWorkspace(t, ctx, agentBin, "nocmd", func(dir string) {
		writeRealSpawnFixture(t, dir, "") // no CMD
	})
	assertFileEventually(t, filepath.Join(ws, "skills", "soul.md"), realSpawnMarker)
}

// realSpawnPreconditions skips (never fails) when the heavy deps are absent and
// returns the linux adapter binary path.
func realSpawnPreconditions(t *testing.T, ctx context.Context) string {
	t.Helper()
	if _, err := run(ctx, "docker", "info"); err != nil {
		t.Skip("docker daemon not reachable; skipping real build+spawn e2e")
	}
	agentBin := resolveAdapterBinary(t) // may t.Skip
	if out, err := run(ctx, "docker", "pull", realSpawnBaseImage); err != nil {
		t.Skipf("cannot pull base image %s; skipping. docker said:\n%s", realSpawnBaseImage, out)
	}
	return agentBin
}

// buildSpawnAndWorkspace runs the full real path (docker build -> sign -> in-process
// spawn) for the given profile fixture and returns the host-side per-pid workspace
// directory (HOME), where seeded and CMD-written files are observable.
func buildSpawnAndWorkspace(t *testing.T, ctx context.Context, agentBin, label string, writeFixture func(dir string)) string {
	t.Helper()

	// isolate CWD: mod/<id>.json and sandbox_workspace/<pid> resolve here.
	runDir := t.TempDir()
	t.Chdir(runDir)
	if err := os.MkdirAll("mod", 0o755); err != nil {
		t.Fatalf("mkdir mod: %v", err)
	}

	profileDir := t.TempDir()
	writeFixture(profileDir)
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
	pid := "realspawn-" + label + "-" + time.Now().Format("150405.000")
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

	return filepath.Join(runDir, "sandbox_workspace", pid)
}

// writeRealSpawnFixture lays out a minimal buildable profile whose public content
// is a single skills/soul.md marker. cmdLine, when non-empty, is inserted verbatim
// into [dockerfile] (e.g. `CMD = [...]`); empty omits CMD entirely.
func writeRealSpawnFixture(t *testing.T, dir, cmdLine string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	profile := "[dockerfile]\n" +
		"FROM = \"docker/sandbox-templates:claude-code\"\n" +
		"bin = \"bin\"\n"
	if cmdLine != "" {
		profile += cmdLine + "\n"
	}
	profile += "\n[vmdocker]\npublic = [\"~/skills/*\"]\n"

	must(os.WriteFile(filepath.Join(dir, "profile.toml"), []byte(profile), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "bin", ".keep"), []byte("keep\n"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "skills"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "skills", "soul.md"), []byte(realSpawnMarker), 0o644))
}

// assertFileEventually polls for path to exist and (trimmed) equal want, up to a
// timeout — the CMD runs in the background under the adapter, so its marker may
// appear shortly after spawn returns.
func assertFileEventually(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			got := strings.TrimSpace(string(b))
			if got == want {
				return
			}
			last = got
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s did not become %q within timeout (last read: %q)", path, want, last)
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
		t.Logf("adapter logs for %s:\n%s", container, out)
	}
}
