package runtimemanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRuntimeWorkspaceDefaultsPerPID(t *testing.T) {
	tempDir := t.TempDir()
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

	workspace, err := resolveRuntimeWorkspace("pid-1", "")
	if err != nil {
		t.Fatalf("resolveRuntimeWorkspace failed: %v", err)
	}

	realTempDir, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		t.Fatalf("eval symlinks temp dir failed: %v", err)
	}
	expected := filepath.Join(realTempDir, runtimeWorkspaceDir, "pid-1")
	if workspace != expected {
		t.Fatalf("workspace = %q, want %q", workspace, expected)
	}
}

func TestResolveRuntimeWorkspaceUsesAbsoluteRoot(t *testing.T) {
	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "workspace-root")

	workspace, err := resolveRuntimeWorkspace("pid-2", root)
	if err != nil {
		t.Fatalf("resolveRuntimeWorkspace failed: %v", err)
	}

	expectedRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root failed: %v", err)
	}
	expected := filepath.Join(expectedRoot, runtimeWorkspaceDir, "pid-2")
	if workspace != expected {
		t.Fatalf("workspace = %q, want %q", workspace, expected)
	}
}

func TestEnsureRuntimeWorkspaceCreatesFullLayout(t *testing.T) {
	tempDir := t.TempDir()
	workspace, err := ensureRuntimeWorkspace("pid-3", filepath.Join(tempDir, "workspace-root"))
	if err != nil {
		t.Fatalf("ensureRuntimeWorkspace failed: %v", err)
	}
	for _, dir := range runtimeWorkspaceLayoutDirs(workspace) {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected runtime dir %s to exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}
}

func TestEnsureRuntimeWorkspaceRootCreatesOnlyWorkspaceRoot(t *testing.T) {
	tempDir := t.TempDir()
	workspace, err := ensureRuntimeWorkspaceRoot("pid-4", filepath.Join(tempDir, "workspace-root"))
	if err != nil {
		t.Fatalf("ensureRuntimeWorkspaceRoot failed: %v", err)
	}

	info, err := os.Stat(workspace)
	if err != nil {
		t.Fatalf("expected workspace root %s to exist: %v", workspace, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", workspace)
	}

	for _, dir := range runtimeWorkspaceLayoutDirs(workspace)[1:] {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected runtime dir %s to be absent, got err=%v", dir, err)
		}
	}
}

func TestDockerAndSandboxShareRuntimeEnvironmentContract(t *testing.T) {
	workspace := "/tmp/workspace/sandbox_workspace/pid-1"
	env := appendRuntimePersistenceEnv([]string{"RUNTIME_TYPE=openclaw"}, workspace)
	full := strings.Join(env, "\n")

	for _, item := range []string{
		"RUNTIME_TYPE=openclaw",
		"HOME=/home/hymx",
		"OPENCLAW_HOME=/home/hymx",
		"OPENCLAW_STATE_DIR=/home/hymx/.openclaw",
		"OPENCLAW_CONFIG_PATH=/home/hymx/.openclaw/openclaw.json",
		"OPENCLAW_AGENT_WORKSPACE=/home/hymx/.openclaw/workspace",
		"VMDOCKER_RUNTIME_WORKSPACE=/home/hymx",
		"VMDOCKER_RUNTIME_HOME=/home/hymx",
		"VMDOCKER_AGENT_WORKSPACE=/home/hymx/workspace",
		"TMPDIR=/home/hymx/.tmp",
		"XDG_CONFIG_HOME=/home/hymx/.xdg/config",
		"XDG_CACHE_HOME=/home/hymx/.xdg/cache",
		"XDG_STATE_HOME=/home/hymx/.xdg/state",
	} {
		if !strings.Contains(full, item) {
			t.Fatalf("expected env %q in %v", item, env)
		}
	}
	if strings.Contains(full, "HOME=/home/hymx/.home") {
		t.Fatal("HOME must be /home/hymx, not a .home subdir")
	}
}

func TestRuntimeWorkspaceLayoutDirs_NoHomeSubdir(t *testing.T) {
	dirs := runtimeWorkspaceLayoutDirs("/ws")
	for _, d := range dirs {
		if d == "/ws/.home" {
			t.Fatal("layout must not create a .home subdir after migration M")
		}
	}
	want := map[string]bool{"/ws": false, "/ws/.openclaw": false, "/ws/.tmp": false, "/ws/.xdg/config": false}
	for _, d := range dirs {
		if _, ok := want[d]; ok {
			want[d] = true
		}
	}
	for d, seen := range want {
		if !seen {
			t.Errorf("layout missing expected dir %s", d)
		}
	}
}

func TestCheckpointAndRestoreRuntimeWorkspaceRoundTrip(t *testing.T) {
	root := t.TempDir()
	workspace, err := ensureRuntimeWorkspace("pid-checkpoint", root)
	if err != nil {
		t.Fatalf("ensureRuntimeWorkspace failed: %v", err)
	}

	filePath := filepath.Join(workspace, ".openclaw", "workspace", "state.txt")
	if err := os.WriteFile(filePath, []byte("restored-state"), 0o644); err != nil {
		t.Fatalf("write workspace file failed: %v", err)
	}

	snapshot, err := checkpointRuntimeWorkspace(workspace, "test-checkpoint")
	if err != nil {
		t.Fatalf("checkpointRuntimeWorkspace failed: %v", err)
	}
	if snapshot == "" {
		t.Fatalf("expected non-empty workspace snapshot")
	}

	if err := os.WriteFile(filePath, []byte("mutated"), 0o644); err != nil {
		t.Fatalf("mutate workspace file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "extra.txt"), []byte("to be removed"), 0o644); err != nil {
		t.Fatalf("write extra file failed: %v", err)
	}

	if err := restoreRuntimeWorkspace(workspace, "test-checkpoint", snapshot); err != nil {
		t.Fatalf("restoreRuntimeWorkspace failed: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read restored file failed: %v", err)
	}
	if string(data) != "restored-state" {
		t.Fatalf("restored file = %q, want %q", string(data), "restored-state")
	}
	if _, err := os.Stat(filepath.Join(workspace, "extra.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected extra file to be removed during restore, got err=%v", err)
	}
}

func TestRestoreRuntimeWorkspaceRejectsEmptySnapshot(t *testing.T) {
	if err := restoreRuntimeWorkspace(filepath.Join(t.TempDir(), "workspace"), "workspace", ""); err == nil {
		t.Fatalf("expected empty snapshot restore to fail")
	}
}

func TestRestoreRuntimeWorkspaceRejectsCheckpointNameMismatch(t *testing.T) {
	root := t.TempDir()
	workspace, err := ensureRuntimeWorkspace("pid-checkpoint-name", root)
	if err != nil {
		t.Fatalf("ensureRuntimeWorkspace failed: %v", err)
	}

	snapshot, err := checkpointRuntimeWorkspace(workspace, "first")
	if err != nil {
		t.Fatalf("checkpointRuntimeWorkspace failed: %v", err)
	}

	if err := restoreRuntimeWorkspace(workspace, "second", snapshot); err == nil {
		t.Fatalf("expected checkpoint name mismatch to fail")
	}
}

func TestStageRuntimeWorkspaceRestoreDoesNotModifyOriginalOnInvalidSnapshot(t *testing.T) {
	root := t.TempDir()
	workspace, err := ensureRuntimeWorkspace("pid-invalid", root)
	if err != nil {
		t.Fatalf("ensureRuntimeWorkspace failed: %v", err)
	}

	originalFile := filepath.Join(workspace, "original.txt")
	if err := os.WriteFile(originalFile, []byte("keep-me"), 0o644); err != nil {
		t.Fatalf("write original file failed: %v", err)
	}

	if _, _, err := stageRuntimeWorkspaceRestore(workspace, "workspace", "not-base64"); err == nil {
		t.Fatalf("expected invalid snapshot to fail")
	}

	data, err := os.ReadFile(originalFile)
	if err != nil {
		t.Fatalf("read original file failed: %v", err)
	}
	if string(data) != "keep-me" {
		t.Fatalf("original file = %q, want %q", string(data), "keep-me")
	}
}

func TestPromoteRuntimeWorkspaceRollbackRestoresBackup(t *testing.T) {
	root := t.TempDir()
	workspace, err := ensureRuntimeWorkspace("pid-rollback", root)
	if err != nil {
		t.Fatalf("ensureRuntimeWorkspace failed: %v", err)
	}
	oldFile := filepath.Join(workspace, "old.txt")
	if err := os.WriteFile(oldFile, []byte("old-state"), 0o644); err != nil {
		t.Fatalf("write old workspace file failed: %v", err)
	}

	stagedWorkspace := filepath.Join(root, "staged")
	if err := os.MkdirAll(stagedWorkspace, 0o755); err != nil {
		t.Fatalf("mkdir staged workspace failed: %v", err)
	}
	if err := ensureRuntimeWorkspaceLayout(stagedWorkspace); err != nil {
		t.Fatalf("ensure staged workspace layout failed: %v", err)
	}
	newFile := filepath.Join(stagedWorkspace, "new.txt")
	if err := os.WriteFile(newFile, []byte("new-state"), 0o644); err != nil {
		t.Fatalf("write staged workspace file failed: %v", err)
	}

	rollback, commit, err := promoteRuntimeWorkspace(workspace, stagedWorkspace)
	if err != nil {
		t.Fatalf("promoteRuntimeWorkspace failed: %v", err)
	}
	if err := rollback(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit after rollback should be harmless, got: %v", err)
	}

	data, err := os.ReadFile(oldFile)
	if err != nil {
		t.Fatalf("read restored old file failed: %v", err)
	}
	if string(data) != "old-state" {
		t.Fatalf("restored old file = %q, want %q", string(data), "old-state")
	}
	if _, err := os.Stat(filepath.Join(workspace, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected staged file to be absent after rollback, got err=%v", err)
	}
}
