package runtimemanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
)

func TestSandboxManagerCreateAndStartInstanceUsesTemplateWorkflow(t *testing.T) {
	sm, logPath, tempDir := newTestSandboxManager(t)

	spec := schema.RuntimeSpec{
		Backend: schema.RuntimeBackendSandbox,
		Image: schema.ImageInfo{
			Name: "chriswebber/docker-openclaw-sandbox:test",
			SHA:  "sha256:expected",
		},
		Sandbox: schema.SandboxSpec{
			Agent:     "shell",
			Workspace: filepath.Join(tempDir, "workspace"),
			Name:      "runtime-pid-1",
		},
	}

	if _, err := sm.CreateInstance(context.Background(), "pid-1", spec, []string{"RUNTIME_TYPE=openclaw"}); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	if err := sm.StartInstance(context.Background(), "pid-1"); err != nil {
		t.Fatalf("StartInstance failed: %v", err)
	}

	expectedWorkspace := filepath.Join(tempDir, "workspace", "sandbox_workspace", "pid-1")
	if _, err := os.Stat(expectedWorkspace); err != nil {
		t.Fatalf("expected workspace directory to exist: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	log := string(raw)

	if !strings.Contains(log, "sandbox create --name runtime-pid-1 --pull-template missing -t chriswebber/docker-openclaw-sandbox:test shell "+expectedWorkspace) {
		t.Fatalf("expected sandbox create command in log, got:\n%s", log)
	}
	if !strings.Contains(log, "image inspect chriswebber/docker-openclaw-sandbox:test") {
		t.Fatalf("expected template image inspect in log, got:\n%s", log)
	}
	if !strings.Contains(log, "sandbox exec -e RUNTIME_TYPE=openclaw") {
		t.Fatalf("expected sandbox exec with runtime env in log, got:\n%s", log)
	}
	if !strings.Contains(log, "sandbox exec -u 0:0 -w "+expectedWorkspace+" runtime-pid-1 sh -c ") {
		t.Fatalf("expected sandbox exec to harden filesystem as root in workspace, got:\n%s", log)
	}
	if !strings.Contains(log, "runtime-pid-1 sh -lc") {
		t.Fatalf("expected sandbox exec target and shell command in log, got:\n%s", log)
	}
	if !strings.Contains(log, "-e OPENCLAW_STATE_DIR="+filepath.Join(containerHome, ".openclaw")) {
		t.Fatalf("expected sandbox exec to inject OPENCLAW_STATE_DIR, got:\n%s", log)
	}
	if !strings.Contains(log, "-e OPENCLAW_HOME="+containerHome) {
		t.Fatalf("expected sandbox exec to inject OPENCLAW_HOME, got:\n%s", log)
	}
	if !strings.Contains(log, "-e OPENCLAW_CONFIG_PATH="+filepath.Join(containerHome, ".openclaw", "openclaw.json")) {
		t.Fatalf("expected sandbox exec to inject OPENCLAW_CONFIG_PATH, got:\n%s", log)
	}
	if !strings.Contains(log, "-e OPENCLAW_AGENT_WORKSPACE="+filepath.Join(containerHome, ".openclaw", "workspace")) {
		t.Fatalf("expected sandbox exec to inject OPENCLAW_AGENT_WORKSPACE, got:\n%s", log)
	}
	if !strings.Contains(log, "-e VMDOCKER_RUNTIME_WORKSPACE="+containerHome) {
		t.Fatalf("expected sandbox exec to inject VMDOCKER_RUNTIME_WORKSPACE, got:\n%s", log)
	}
	if !strings.Contains(log, "-e VMDOCKER_RUNTIME_HOME="+containerHome) {
		t.Fatalf("expected sandbox exec to inject VMDOCKER_RUNTIME_HOME, got:\n%s", log)
	}
	if !strings.Contains(log, "-e VMDOCKER_AGENT_WORKSPACE="+filepath.Join(containerHome, "workspace")) {
		t.Fatalf("expected sandbox exec to inject VMDOCKER_AGENT_WORKSPACE, got:\n%s", log)
	}
	if !strings.Contains(log, "-e HOME="+containerHome) {
		t.Fatalf("expected sandbox exec to inject HOME, got:\n%s", log)
	}
	if !strings.Contains(log, "-e TMPDIR="+filepath.Join(containerHome, ".tmp")) {
		t.Fatalf("expected sandbox exec to inject TMPDIR, got:\n%s", log)
	}
	if !strings.Contains(log, "-e XDG_CONFIG_HOME="+filepath.Join(containerHome, ".xdg", "config")) {
		t.Fatalf("expected sandbox exec to inject XDG_CONFIG_HOME, got:\n%s", log)
	}
	if !strings.Contains(log, "-e XDG_CACHE_HOME="+filepath.Join(containerHome, ".xdg", "cache")) {
		t.Fatalf("expected sandbox exec to inject XDG_CACHE_HOME, got:\n%s", log)
	}
	if !strings.Contains(log, "-e XDG_STATE_HOME="+filepath.Join(containerHome, ".xdg", "state")) {
		t.Fatalf("expected sandbox exec to inject XDG_STATE_HOME, got:\n%s", log)
	}
	if !strings.Contains(log, defaultRuntimeStartCommand) {
		t.Fatalf("expected default runtime start command in log, got:\n%s", log)
	}
	defaultCommand, err := buildBackgroundRuntimeCommand("")
	if err != nil {
		t.Fatalf("buildBackgroundRuntimeCommand failed: %v", err)
	}
	if !strings.Contains(log, defaultCommand) {
		t.Fatalf("expected sandbox start command to precreate TMPDIR, got:\n%s", log)
	}
	if !strings.Contains(log, buildSandboxFilesystemLockdownCommand()) {
		t.Fatalf("expected sandbox filesystem lockdown command in log, got:\n%s", log)
	}
	if strings.Contains(log, "docker run") {
		t.Fatalf("unexpected nested docker run in log:\n%s", log)
	}
}

func TestSandboxManagerCreateInstancePullsAndVerifiesMissingTemplate(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "docker.log")
	statePath := filepath.Join(tempDir, "inspect-state")
	fakeDocker := filepath.Join(tempDir, "docker")
	script := "#!/bin/sh\nprintf '%s\n' \"$*\" >>" + shellEscapeForTest(logPath) + "\nif [ \"$1\" = \"--help\" ]; then\n  echo sandbox\n  exit 0\nfi\nif [ \"$1\" = \"image\" ] && [ \"$2\" = \"inspect\" ]; then\n  if [ ! -f " + shellEscapeForTest(statePath) + " ]; then\n    echo 'Error: No such image' >&2\n    exit 1\n  fi\n  echo '[{\"Id\":\"sha256:template-id\",\"RepoDigests\":[\"chriswebber/docker-openclaw-sandbox@test-sha256:expected\"]}]'\n  exit 0\nfi\nif [ \"$1\" = \"pull\" ]; then\n  : > " + shellEscapeForTest(statePath) + "\n  exit 0\nfi\nexit 0\n"
	if err := os.WriteFile(fakeDocker, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker failed: %v", err)
	}

	sm, err := newSandboxManager()
	if err != nil {
		t.Fatalf("newSandboxManager failed: %v", err)
	}
	sm.cliBin = fakeDocker

	spec := schema.RuntimeSpec{
		Backend: schema.RuntimeBackendSandbox,
		Image: schema.ImageInfo{
			Name: "chriswebber/docker-openclaw-sandbox:test",
			SHA:  "sha256:expected",
		},
		Sandbox: schema.SandboxSpec{
			Agent:     "shell",
			Workspace: filepath.Join(tempDir, "workspace"),
			Name:      "runtime-pid-2",
		},
	}

	if _, err := sm.CreateInstance(context.Background(), "pid-2", spec, nil); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	expectedWorkspace := filepath.Join(tempDir, "workspace", "sandbox_workspace", "pid-2")
	if _, err := os.Stat(expectedWorkspace); err != nil {
		t.Fatalf("expected workspace directory to exist: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "pull chriswebber/docker-openclaw-sandbox:test") {
		t.Fatalf("expected template image pull in log, got:\n%s", log)
	}
	if strings.Count(log, "image inspect chriswebber/docker-openclaw-sandbox:test") < 2 {
		t.Fatalf("expected template image inspect before and after pull, got:\n%s", log)
	}
}

func TestSandboxManagerCreateInstanceDefaultsWorkspacePerPid(t *testing.T) {
	sm, logPath, tempDir := newTestSandboxManager(t)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	spec := schema.RuntimeSpec{
		Backend: schema.RuntimeBackendSandbox,
		Image: schema.ImageInfo{
			Name: "chriswebber/docker-openclaw-sandbox:test",
			SHA:  "sha256:expected",
		},
		Sandbox: schema.SandboxSpec{
			Agent: "shell",
			Name:  "runtime-pid-3",
		},
	}

	if _, err := sm.CreateInstance(context.Background(), "pid-3", spec, nil); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	realTempDir, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		t.Fatalf("eval symlinks temp dir failed: %v", err)
	}
	expectedWorkspace := filepath.Join(realTempDir, "sandbox_workspace", "pid-3")
	if _, err := os.Stat(expectedWorkspace); err != nil {
		t.Fatalf("expected workspace directory to exist: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "sandbox create --name runtime-pid-3 --pull-template missing -t chriswebber/docker-openclaw-sandbox:test shell "+expectedWorkspace) {
		t.Fatalf("expected sandbox create command with default workspace in log, got:\n%s", log)
	}
}

func TestSandboxManagerCreateInstanceDefaultsSandboxNameToPidPrefix(t *testing.T) {
	sm, logPath, tempDir := newTestSandboxManager(t)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	pid := "4qIDQKNWm6kF4aDI-rpz_2VLkKWSLVxCJVw2RGJayoQ"
	spec := schema.RuntimeSpec{
		Backend: schema.RuntimeBackendSandbox,
		Image: schema.ImageInfo{
			Name: "chriswebber/docker-openclaw-sandbox:test",
			SHA:  "sha256:expected",
		},
		Sandbox: schema.SandboxSpec{
			Agent: "shell",
		},
	}

	instance, err := sm.CreateInstance(context.Background(), pid, spec, nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	if instance.ID != "hymatrix_4qIDQKNWm6" {
		t.Fatalf("expected shortened sandbox name, got %q", instance.ID)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "sandbox create --name hymatrix_4qIDQKNWm6 --pull-template missing -t chriswebber/docker-openclaw-sandbox:test shell ") {
		t.Fatalf("expected shortened sandbox name in log, got:\n%s", log)
	}
}

func TestSandboxManagerStartInstanceRespectsExplicitOpenclawStateDir(t *testing.T) {
	sm, logPath, tempDir := newTestSandboxManager(t)

	spec := schema.RuntimeSpec{
		Backend: schema.RuntimeBackendSandbox,
		Image: schema.ImageInfo{
			Name: "chriswebber/docker-openclaw-sandbox:test",
			SHA:  "sha256:expected",
		},
		Sandbox: schema.SandboxSpec{
			Agent:     "shell",
			Workspace: filepath.Join(tempDir, "workspace"),
			Name:      "runtime-pid-4",
		},
	}

	env := []string{
		"OPENCLAW_STATE_DIR=/custom/state",
		"OPENCLAW_CONFIG_PATH=/custom/state/custom.json",
	}
	if _, err := sm.CreateInstance(context.Background(), "pid-4", spec, env); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	if err := sm.StartInstance(context.Background(), "pid-4"); err != nil {
		t.Fatalf("StartInstance failed: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "-e OPENCLAW_STATE_DIR=/custom/state") {
		t.Fatalf("expected explicit OPENCLAW_STATE_DIR in log, got:\n%s", log)
	}
	if !strings.Contains(log, "-e OPENCLAW_CONFIG_PATH=/custom/state/custom.json") {
		t.Fatalf("expected explicit OPENCLAW_CONFIG_PATH in log, got:\n%s", log)
	}
	if strings.Contains(log, filepath.Join(tempDir, "workspace", "sandbox_workspace", "pid-4", ".openclaw")) {
		t.Fatalf("unexpected default workspace-backed OpenClaw path in log:\n%s", log)
	}
}

func TestAppendRuntimePersistenceEnv(t *testing.T) {
	workspace := "/tmp/workspace/sandbox_workspace/pid-1"
	env := appendRuntimePersistenceEnv([]string{"RUNTIME_TYPE=openclaw"}, workspace)
	expected := []string{
		"RUNTIME_TYPE=openclaw",
		"OPENCLAW_STATE_DIR=/home/hymx/.openclaw",
		"OPENCLAW_HOME=/home/hymx",
		"OPENCLAW_CONFIG_PATH=/home/hymx/.openclaw/openclaw.json",
		"OPENCLAW_AGENT_WORKSPACE=/home/hymx/.openclaw/workspace",
		"VMDOCKER_RUNTIME_WORKSPACE=/home/hymx",
		"VMDOCKER_RUNTIME_HOME=/home/hymx",
		"VMDOCKER_AGENT_WORKSPACE=/home/hymx/workspace",
		"HOME=/home/hymx",
		"TMPDIR=/home/hymx/.tmp",
		"XDG_CONFIG_HOME=/home/hymx/.xdg/config",
		"XDG_CACHE_HOME=/home/hymx/.xdg/cache",
		"XDG_STATE_HOME=/home/hymx/.xdg/state",
	}
	if len(env) != len(expected) {
		t.Fatalf("env length = %d, want %d (%v)", len(env), len(expected), env)
	}
	for i, item := range expected {
		if env[i] != item {
			t.Fatalf("env[%d] = %q, want %q (full=%v)", i, env[i], item, env)
		}
	}
}

func TestAppendRuntimePersistenceEnvRespectsExplicitDirectoryEnv(t *testing.T) {
	workspace := "/tmp/workspace/sandbox_workspace/pid-1"
	env := appendRuntimePersistenceEnv([]string{
		"OPENCLAW_HOME=/custom/openclaw-home",
		"HOME=/custom/home",
		"TMPDIR=/custom/tmp",
		"XDG_CONFIG_HOME=/custom/xdg/config",
		"XDG_CACHE_HOME=/custom/xdg/cache",
		"XDG_STATE_HOME=/custom/xdg/state",
		"OPENCLAW_AGENT_WORKSPACE=/custom/agent-workspace",
	}, workspace)
	full := strings.Join(env, "\n")
	for _, item := range []string{
		"OPENCLAW_HOME=/custom/openclaw-home",
		"HOME=/custom/home",
		"TMPDIR=/custom/tmp",
		"XDG_CONFIG_HOME=/custom/xdg/config",
		"XDG_CACHE_HOME=/custom/xdg/cache",
		"XDG_STATE_HOME=/custom/xdg/state",
		"OPENCLAW_AGENT_WORKSPACE=/custom/agent-workspace",
		"VMDOCKER_RUNTIME_WORKSPACE=/custom/home",
		"VMDOCKER_RUNTIME_HOME=/custom/home",
		"VMDOCKER_AGENT_WORKSPACE=/custom/home/workspace",
	} {
		if !strings.Contains(full, item) {
			t.Fatalf("expected explicit env %q in %v", item, env)
		}
	}
	if containsEnv(env, "HOME", workspace+"/.home") {
		t.Fatalf("unexpected default HOME in %v", env)
	}
	if containsEnv(env, "OPENCLAW_HOME", workspace) {
		t.Fatalf("unexpected default OPENCLAW_HOME in %v", env)
	}
}

func TestAppendRuntimePersistenceEnvDerivesConfigFromExplicitStateDir(t *testing.T) {
	env := appendRuntimePersistenceEnv([]string{
		"OPENCLAW_STATE_DIR=/custom/state",
	}, "/tmp/workspace/sandbox_workspace/pid-1")
	full := strings.Join(env, "\n")
	if !strings.Contains(full, "OPENCLAW_CONFIG_PATH=/custom/state/openclaw.json") {
		t.Fatalf("expected config path to derive from explicit state dir, got %v", env)
	}
}

func containsEnv(env []string, key, value string) bool {
	want := key + "=" + value
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}

func TestSandboxManagerRemoveInstancePreservesWorkspace(t *testing.T) {
	sm, logPath, tempDir := newTestSandboxManager(t)
	_ = logPath

	spec := schema.RuntimeSpec{
		Backend: schema.RuntimeBackendSandbox,
		Image: schema.ImageInfo{
			Name: "chriswebber/docker-openclaw-sandbox:test",
			SHA:  "sha256:expected",
		},
		Sandbox: schema.SandboxSpec{
			Agent:     "shell",
			Workspace: filepath.Join(tempDir, "workspace"),
			Name:      "runtime-pid-5",
		},
	}

	if _, err := sm.CreateInstance(context.Background(), "pid-5", spec, nil); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	expectedWorkspace := filepath.Join(tempDir, "workspace", "sandbox_workspace", "pid-5")
	markerPath := filepath.Join(expectedWorkspace, "persist.txt")
	if err := os.WriteFile(markerPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write marker failed: %v", err)
	}

	if err := sm.RemoveInstance(context.Background(), "pid-5"); err != nil {
		t.Fatalf("RemoveInstance failed: %v", err)
	}

	raw, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("expected marker to survive sandbox removal: %v", err)
	}
	if string(raw) != "keep" {
		t.Fatalf("marker contents = %q, want keep", string(raw))
	}
}

func TestBuildBackgroundRuntimeCommandUsesConfiguredRuntimeCommand(t *testing.T) {
	command, err := buildBackgroundRuntimeCommand("/app/custom-entrypoint --serve")
	if err != nil {
		t.Fatalf("buildBackgroundRuntimeCommand failed: %v", err)
	}
	expected := "mkdir -p \"${TMPDIR:-/tmp}\" && '/app/custom-entrypoint' '--serve' >\"${TMPDIR:-/tmp}/vmdocker-agent.log\" 2>&1 &"
	if command != expected {
		t.Fatalf("buildBackgroundRuntimeCommand() = %q, want %q", command, expected)
	}
}

func TestSandboxManagerStartInstancePrefersStartCommandOverSandboxCommand(t *testing.T) {
	sm, logPath, tempDir := newTestSandboxManager(t)

	spec := schema.RuntimeSpec{
		Backend:      schema.RuntimeBackendSandbox,
		StartCommand: "/app/start-runtime.sh --foreground",
		Image: schema.ImageInfo{
			Name: "chriswebber/docker-openclaw-sandbox:test",
			SHA:  "sha256:expected",
		},
		Sandbox: schema.SandboxSpec{
			Agent:     "shell",
			Workspace: filepath.Join(tempDir, "workspace"),
			Name:      "runtime-pid-6",
			Command:   "legacy-sandbox-command",
		},
	}

	if _, err := sm.CreateInstance(context.Background(), "pid-6", spec, nil); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	if err := sm.StartInstance(context.Background(), "pid-6"); err != nil {
		t.Fatalf("StartInstance failed: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	log := string(raw)
	expectedCommand, err := buildBackgroundRuntimeCommand("/app/start-runtime.sh --foreground")
	if err != nil {
		t.Fatalf("buildBackgroundRuntimeCommand failed: %v", err)
	}
	if !strings.Contains(log, expectedCommand) {
		t.Fatalf("expected Start-Command based sandbox command in log, got:\n%s", log)
	}
	if strings.Contains(log, "legacy-sandbox-command") {
		t.Fatalf("did not expect Sandbox-Command to override Start-Command, got:\n%s", log)
	}
}

func TestBuildSandboxFilesystemLockdownCommand(t *testing.T) {
	command := buildSandboxFilesystemLockdownCommand()
	for _, snippet := range []string{
		"chown -R root:root /home/agent",
		"chmod 0555 /home/agent",
		"chown -R root:root /workspace",
		"chmod 0555 /workspace",
		"chmod 0755 /tmp",
		"chmod 0755 /var/tmp",
	} {
		if !strings.Contains(command, snippet) {
			t.Fatalf("expected %q in %q", snippet, command)
		}
	}
}

// newTestSandboxManager creates a SandboxManager backed by a fake docker binary
// that logs all invocations and returns canned image-inspect responses.
// It returns the manager, the path to the invocation log file, and the temp directory.
func newTestSandboxManager(t *testing.T) (*SandboxManager, string, string) {
	t.Helper()
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "docker.log")
	fakeDocker := filepath.Join(tempDir, "docker")
	script := "#!/bin/sh\nprintf '%s\n' \"$*\" >>" + shellEscapeForTest(logPath) + "\nif [ \"$1\" = \"--help\" ]; then\n  echo sandbox\n  exit 0\nfi\nif [ \"$1\" = \"image\" ] && [ \"$2\" = \"inspect\" ]; then\n  echo '[{\"Id\":\"sha256:template-id\",\"RepoDigests\":[\"chriswebber/docker-openclaw-sandbox@test-sha256:expected\"]}]'\n  exit 0\nfi\nexit 0\n"
	if err := os.WriteFile(fakeDocker, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker failed: %v", err)
	}
	sm, err := newSandboxManager()
	if err != nil {
		t.Fatalf("newSandboxManager failed: %v", err)
	}
	sm.cliBin = fakeDocker
	return sm, logPath, tempDir
}

func shellEscapeForTest(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
