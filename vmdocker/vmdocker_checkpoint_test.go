package vmdocker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager"
	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	goarSchema "github.com/permadao/goar/schema"
)

type fakeRuntimeManager struct {
	instance     *runtimeSchema.InstanceInfo
	createCalls  []runtimeLaunchConfig
	removeCalls  int
	startCalls   int
	execCommands []string
}

func (f *fakeRuntimeManager) CreateInstance(_ context.Context, pid string, runtimeSpec runtimeSchema.RuntimeSpec, runtimeEnv []string) (*runtimeSchema.InstanceInfo, error) {
	f.createCalls = append(f.createCalls, runtimeLaunchConfig{
		runtimeSpec: cloneRuntimeSpec(runtimeSpec),
		runtimeEnv:  cloneRuntimeEnv(runtimeEnv),
	})

	workspace := filepath.Join(runtimeSpec.Sandbox.Workspace, "sandbox_workspace", pid)
	if err := os.MkdirAll(filepath.Join(workspace, ".tmp"), 0o755); err != nil {
		return nil, err
	}
	f.instance = &runtimeSchema.InstanceInfo{
		ID:          "fake-" + pid,
		Name:        pid,
		Status:      "created",
		Backend:     runtimeSpec.Backend,
		Workspace:   workspace,
		RuntimeSpec: cloneRuntimeSpec(runtimeSpec),
		RuntimeEnv:  cloneRuntimeEnv(runtimeEnv),
	}
	return f.instance, nil
}

func (f *fakeRuntimeManager) GetInstance(pid string) (*runtimeSchema.InstanceInfo, error) {
	if f.instance == nil || f.instance.Name != pid {
		return nil, fmt.Errorf("instance not found")
	}
	return f.instance, nil
}

func (f *fakeRuntimeManager) RemoveInstance(_ context.Context, pid string) error {
	if f.instance == nil || f.instance.Name != pid {
		return fmt.Errorf("instance not found")
	}
	f.removeCalls++
	f.instance = nil
	return nil
}

func (f *fakeRuntimeManager) StartInstance(_ context.Context, pid string) error {
	if f.instance == nil || f.instance.Name != pid {
		return fmt.Errorf("instance not found")
	}
	f.startCalls++
	f.instance.Status = "running"
	return nil
}

func (f *fakeRuntimeManager) StopInstance(context.Context, string) error {
	return nil
}

func (f *fakeRuntimeManager) ExecInstance(_ context.Context, _ string, _ []string, command string) (string, error) {
	f.execCommands = append(f.execCommands, command)
	switch {
	case strings.Contains(command, "/vmm/checkpoint"):
		return `{"status":"ok","state":"runtime-state-1"}` + "\n__STATUS__:200", nil
	case strings.Contains(command, "/vmm/restore"):
		return `{"status":"ok"}` + "\n__STATUS__:200", nil
	case strings.Contains(command, "/vmm/health"):
		return `ok` + "\n__STATUS__:200", nil
	default:
		return "", fmt.Errorf("unexpected command: %s", command)
	}
}

func (f *fakeRuntimeManager) Checkpoint(context.Context, string, string) (string, error) {
	return "", fmt.Errorf("unexpected checkpoint call")
}

func (f *fakeRuntimeManager) Restore(context.Context, string, string, string) error {
	return fmt.Errorf("unexpected restore call")
}

var _ runtimemanager.IRuntimeManager = (*fakeRuntimeManager)(nil)

func TestResolveRestoreLaunchConfigPrefersCheckpointLaunchConfig(t *testing.T) {
	targetWorkspace := filepath.Join(t.TempDir(), "sandbox_workspace", "pid-1")
	checkpoint := workspaceCheckpoint{
		Backend: "sandbox",
		RuntimeSpec: runtimeSchema.RuntimeSpec{
			Backend: "sandbox",
			Image: runtimeSchema.ImageInfo{
				Name: "claude-image:test",
				SHA:  "sha256:claude",
			},
			Sandbox: runtimeSchema.SandboxSpec{
				Agent: "shell",
			},
		},
		RuntimeEnv: []string{
			"RUNTIME_TYPE=claude",
			"ANTHROPIC_MODEL=claude-sonnet",
		},
	}
	fallbackSpec := runtimeSchema.RuntimeSpec{
		Backend: "docker",
		Image: runtimeSchema.ImageInfo{
			Name: "other-image:test",
			SHA:  "sha256:other",
		},
	}

	launchConfig, err := resolveRestoreLaunchConfig(checkpoint, fallbackSpec, []goarSchema.Tag{
		{Name: "ignored", Value: "ignored"},
	}, targetWorkspace)
	if err != nil {
		t.Fatalf("resolveRestoreLaunchConfig failed: %v", err)
	}

	if launchConfig.runtimeSpec.Backend != "sandbox" {
		t.Fatalf("backend = %q, want %q", launchConfig.runtimeSpec.Backend, "sandbox")
	}
	if launchConfig.runtimeSpec.Image.Name != "claude-image:test" {
		t.Fatalf("image = %q, want %q", launchConfig.runtimeSpec.Image.Name, "claude-image:test")
	}
	if launchConfig.runtimeSpec.Sandbox.Workspace != filepath.Dir(filepath.Dir(targetWorkspace)) {
		t.Fatalf("workspace root = %q, want %q", launchConfig.runtimeSpec.Sandbox.Workspace, filepath.Dir(filepath.Dir(targetWorkspace)))
	}
	if len(launchConfig.runtimeEnv) != 2 || launchConfig.runtimeEnv[1] != "ANTHROPIC_MODEL=claude-sonnet" {
		t.Fatalf("runtime env = %v", launchConfig.runtimeEnv)
	}
}

func TestResolveRestoreLaunchConfigUsesCheckpointBackendForLegacyPayload(t *testing.T) {
	targetWorkspace := filepath.Join(t.TempDir(), "sandbox_workspace", "pid-2")
	launchConfig, err := resolveRestoreLaunchConfig(
		workspaceCheckpoint{Backend: "sandbox"},
		runtimeSchema.RuntimeSpec{
			Backend: "docker",
			Image: runtimeSchema.ImageInfo{
				Name: "fallback:test",
				SHA:  "sha256:fallback",
			},
		},
		nil,
		targetWorkspace,
	)
	if err != nil {
		t.Fatalf("resolveRestoreLaunchConfig failed: %v", err)
	}
	if launchConfig.runtimeSpec.Backend != "sandbox" {
		t.Fatalf("backend = %q, want %q", launchConfig.runtimeSpec.Backend, "sandbox")
	}
}

func TestCheckpointRecreatesRuntimeForConsistentWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "sandbox_workspace", "pid-checkpoint")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace failed: %v", err)
	}
	stateFile := filepath.Join(workspace, ".claude.json")
	if err := os.WriteFile(stateFile, []byte("checkpoint-state"), 0o644); err != nil {
		t.Fatalf("write state file failed: %v", err)
	}

	fakeManager := &fakeRuntimeManager{
		instance: &runtimeSchema.InstanceInfo{
			ID:        "fake-pid-checkpoint",
			Name:      "pid-checkpoint",
			Status:    "running",
			Backend:   runtimeSchema.RuntimeBackendSandbox,
			Workspace: workspace,
			RuntimeSpec: runtimeSchema.RuntimeSpec{
				Backend: runtimeSchema.RuntimeBackendSandbox,
				Image: runtimeSchema.ImageInfo{
					Name: "claude-image:test",
					SHA:  "sha256:claude",
				},
				Sandbox: runtimeSchema.SandboxSpec{
					Workspace: root,
					Agent:     "shell",
				},
			},
			RuntimeEnv: []string{"RUNTIME_TYPE=claude"},
		},
	}

	vm := &VmDocker{
		pid:            "pid-checkpoint",
		runtimeManager: fakeManager,
		instanceInfo:   fakeManager.instance,
		closeChan:      make(chan struct{}),
	}

	raw, err := vm.Checkpoint()
	if err != nil {
		t.Fatalf("Checkpoint failed: %v", err)
	}
	if fakeManager.removeCalls != 1 {
		t.Fatalf("removeCalls = %d, want %d", fakeManager.removeCalls, 1)
	}
	if len(fakeManager.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want %d", len(fakeManager.createCalls), 1)
	}
	if fakeManager.startCalls != 1 {
		t.Fatalf("startCalls = %d, want %d", fakeManager.startCalls, 1)
	}
	if vm.instanceInfo == nil {
		t.Fatalf("expected vm.instanceInfo to be restored after checkpoint")
	}

	var checkpoint workspaceCheckpoint
	if err := json.Unmarshal([]byte(raw), &checkpoint); err != nil {
		t.Fatalf("unmarshal checkpoint failed: %v", err)
	}
	if checkpoint.WorkspaceCheckpointName != "workspace" {
		t.Fatalf("workspace checkpoint name = %q, want %q", checkpoint.WorkspaceCheckpointName, "workspace")
	}
	if checkpoint.RuntimeSpec.Backend != runtimeSchema.RuntimeBackendSandbox {
		t.Fatalf("checkpoint runtime backend = %q, want %q", checkpoint.RuntimeSpec.Backend, runtimeSchema.RuntimeBackendSandbox)
	}
	if len(checkpoint.RuntimeEnv) != 1 || checkpoint.RuntimeEnv[0] != "RUNTIME_TYPE=claude" {
		t.Fatalf("checkpoint runtime env = %v", checkpoint.RuntimeEnv)
	}

	restoreWorkspace := filepath.Join(t.TempDir(), "sandbox_workspace", "pid-restored")
	if err := os.MkdirAll(filepath.Dir(restoreWorkspace), 0o755); err != nil {
		t.Fatalf("mkdir restore workspace parent failed: %v", err)
	}
	staged, cleanup, err := runtimemanager.StageRuntimeWorkspaceRestore(restoreWorkspace, checkpoint.WorkspaceCheckpointName, checkpoint.WorkspaceArchive)
	if err != nil {
		t.Fatalf("StageRuntimeWorkspaceRestore failed: %v", err)
	}
	defer cleanup()
	data, err := os.ReadFile(filepath.Join(staged, ".claude.json"))
	if err != nil {
		t.Fatalf("read restored workspace file failed: %v", err)
	}
	if string(data) != "checkpoint-state" {
		t.Fatalf("restored state = %q, want %q", string(data), "checkpoint-state")
	}
}

func TestRestorePreviousRuntimeUsesRollbackLaunchConfig(t *testing.T) {
	root := t.TempDir()
	fakeManager := &fakeRuntimeManager{}
	vm := &VmDocker{
		pid:            "pid-rollback",
		runtimeManager: fakeManager,
		closeChan:      make(chan struct{}),
	}

	err := vm.restorePreviousRuntime(context.Background(), &restoreRollbackState{
		runtimeSpec: runtimeSchema.RuntimeSpec{
			Backend: runtimeSchema.RuntimeBackendSandbox,
			Image: runtimeSchema.ImageInfo{
				Name: "claude-image:test",
				SHA:  "sha256:claude",
			},
			Sandbox: runtimeSchema.SandboxSpec{
				Workspace: root,
			},
		},
		runtimeEnv:    []string{"RUNTIME_TYPE=claude", "ANTHROPIC_MODEL=claude-sonnet"},
		runtimeState:  "runtime-state-rollback",
		runtimeManger: fakeManager,
	})
	if err != nil {
		t.Fatalf("restorePreviousRuntime failed: %v", err)
	}
	if len(fakeManager.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want %d", len(fakeManager.createCalls), 1)
	}
	if len(fakeManager.createCalls[0].runtimeEnv) != 2 || fakeManager.createCalls[0].runtimeEnv[1] != "ANTHROPIC_MODEL=claude-sonnet" {
		t.Fatalf("rollback runtime env = %v", fakeManager.createCalls[0].runtimeEnv)
	}
}
