package runtimemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
)

const (
	defaultSandboxAgent = "shell"
)

type SandboxManager struct {
	instances     map[string]*schema.InstanceInfo
	launchSpecs   map[string]sandboxLaunchSpec
	mutex         sync.RWMutex
	portAllocator *portAllocator
	cliBin        string
}

type sandboxLaunchSpec struct {
	runtimeSpec schema.RuntimeSpec
	runtimeEnv  []string
	workspace   string
}

func newSandboxManager() (*SandboxManager, error) {
	cliBin, err := exec.LookPath("docker")
	if err != nil {
		cliBin = "docker"
	}

	return &SandboxManager{
		instances:     make(map[string]*schema.InstanceInfo),
		launchSpecs:   make(map[string]sandboxLaunchSpec),
		portAllocator: newPortAllocator(10000, 20000),
		cliBin:        cliBin,
	}, nil
}

func (sm *SandboxManager) CreateInstance(ctx context.Context, pid string, runtimeSpec schema.RuntimeSpec, runtimeEnv []string) (*schema.InstanceInfo, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	createStart := time.Now()
	log.Info("creating sandbox runtime instance", "pid", pid, "template_image", runtimeSpec.Image.Name, "env_count", len(runtimeEnv))
	if err := sm.ensureSandboxCLI(ctx); err != nil {
		return nil, err
	}
	templateStart := time.Now()
	if err := sm.ensureTemplateExists(ctx, runtimeSpec.Image); err != nil {
		return nil, err
	}
	log.Debug("sandbox template ensured", "pid", pid, "template_image", runtimeSpec.Image.Name, "elapsed", time.Since(templateStart))

	port, err := sm.portAllocator.Allocate()
	if err != nil {
		return nil, err
	}

	sandboxName := runtimeSpec.Sandbox.Name
	if sandboxName == "" {
		sandboxName = defaultSandboxName(pid)
	}

	workspace, err := ensureRuntimeWorkspaceRoot(pid, runtimeSpec.Sandbox.Workspace)
	if err != nil {
		sm.portAllocator.Release(port)
		return nil, err
	}

	agent := runtimeSpec.Sandbox.Agent
	if agent == "" {
		agent = defaultSandboxAgent
	}

	args := []string{
		"sandbox", "create",
		"--name", sandboxName,
		"--pull-template", "missing",
		"-t", runtimeSpec.Image.Name,
		agent, workspace,
	}
	log.Debug("creating sandbox", "pid", pid, "sandbox_name", sandboxName, "agent", agent, "workspace", workspace, "template_image", runtimeSpec.Image.Name)

	sandboxCreateStart := time.Now()
	if _, err := sm.runSandboxCommand(ctx, args...); err != nil {
		sm.portAllocator.Release(port)
		return nil, err
	}
	log.Debug("sandbox create command completed", "pid", pid, "sandbox_name", sandboxName, "elapsed", time.Since(sandboxCreateStart))

	launchRuntimeSpec := runtimeSpec
	launchRuntimeSpec.Sandbox.Workspace = runtimeWorkspaceRootFromPath(workspace)
	launchRuntimeEnv := append([]string(nil), runtimeEnv...)

	instance := &schema.InstanceInfo{
		ID:          sandboxName,
		Name:        pid,
		Port:        port,
		Status:      "created",
		CreateAt:    time.Now(),
		Backend:     schema.RuntimeBackendSandbox,
		Agent:       agent,
		Workspace:   workspace,
		RuntimeSpec: launchRuntimeSpec,
		RuntimeEnv:  launchRuntimeEnv,
	}
	sm.instances[pid] = instance
	sm.launchSpecs[pid] = sandboxLaunchSpec{
		runtimeSpec: runtimeSpec,
		runtimeEnv:  launchRuntimeEnv,
		workspace:   workspace,
	}
	log.Info("sandbox runtime instance created", "pid", pid, "sandbox_name", sandboxName, "port", port)
	log.Debug("sandbox runtime instance create elapsed", "pid", pid, "sandbox_name", sandboxName, "elapsed", time.Since(createStart))
	return instance, nil
}

func defaultSandboxName(pid string) string {
	const maxPIDPrefixLen = 10
	if len(pid) > maxPIDPrefixLen {
		pid = pid[:maxPIDPrefixLen]
	}
	return ContainerNamePrefix + pid
}

func (sm *SandboxManager) GetInstance(pid string) (*schema.InstanceInfo, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	instance, exists := sm.instances[pid]
	if !exists {
		return nil, fmt.Errorf("instance not found")
	}
	return instance, nil
}

func (sm *SandboxManager) StartInstance(ctx context.Context, pid string) error {
	start := time.Now()
	log.Info("starting sandbox runtime instance", "pid", pid)
	sm.mutex.RLock()
	launchSpec, exists := sm.launchSpecs[pid]
	sm.mutex.RUnlock()
	if !exists {
		return fmt.Errorf("sandbox launch spec not found: %s", pid)
	}

	if err := sm.startSandboxRuntime(ctx, pid, launchSpec.runtimeSpec, launchSpec.runtimeEnv, launchSpec.workspace); err != nil {
		return err
	}
	log.Debug("sandbox runtime start command completed", "pid", pid, "elapsed", time.Since(start))
	return nil
}

func (sm *SandboxManager) startSandboxRuntime(ctx context.Context, pid string, runtimeSpec schema.RuntimeSpec, runtimeEnv []string, workspace string) error {
	instance, err := sm.GetInstance(pid)
	if err != nil {
		return err
	}

	lockdownCommand := buildSandboxFilesystemLockdownCommand()
	if _, err := sm.execInstanceAsUser(ctx, instance, "0:0", nil, lockdownCommand, false); err != nil {
		return fmt.Errorf("sandbox filesystem lockdown failed: %w", err)
	}

	command := runtimeSpec.StartCommand
	if command != "" {
		command, err = buildBackgroundRuntimeCommand(runtimeSpec.StartCommand)
		if err != nil {
			return fmt.Errorf("build sandbox start command failed: %w", err)
		}
	} else {
		command = runtimeSpec.Sandbox.Command
	}
	if command == "" {
		command, err = buildBackgroundRuntimeCommand("")
		if err != nil {
			return fmt.Errorf("build sandbox start command failed: %w", err)
		}
	}
	log.Debug("executing sandbox runtime start command", "pid", pid, "sandbox_name", instance.ID, "command", command, "env_count", len(runtimeEnv))

	if _, err := sm.ExecInstance(ctx, pid, appendRuntimePersistenceEnv(runtimeEnv, workspace), command); err != nil {
		return err
	}

	instance.Status = "running"
	log.Info("sandbox runtime instance started", "pid", pid, "sandbox_name", instance.ID)
	return nil
}

func (sm *SandboxManager) StopInstance(ctx context.Context, pid string) error {
	log.Info("stopping sandbox runtime instance", "pid", pid)
	instance, err := sm.GetInstance(pid)
	if err != nil {
		return err
	}

	if _, err := sm.runSandboxCommand(ctx, "sandbox", "stop", instance.ID); err != nil {
		return err
	}

	instance.Status = "stopped"
	log.Info("sandbox runtime instance stopped", "pid", pid, "sandbox_name", instance.ID)
	return nil
}

func (sm *SandboxManager) RemoveInstance(ctx context.Context, pid string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	log.Info("removing sandbox runtime instance", "pid", pid)
	instance, exists := sm.instances[pid]
	if !exists {
		return fmt.Errorf("instance not found: %s", pid)
	}

	if _, err := sm.runSandboxCommand(ctx, "sandbox", "stop", instance.ID); err != nil && !strings.Contains(err.Error(), "not running") {
		return err
	}

	if _, err := sm.runSandboxCommand(ctx, "sandbox", "rm", instance.ID); err != nil {
		return err
	}

	sm.portAllocator.Release(instance.Port)
	delete(sm.instances, pid)
	delete(sm.launchSpecs, pid)
	log.Info("sandbox runtime instance removed", "pid", pid, "sandbox_name", instance.ID)
	return nil
}

func (sm *SandboxManager) Checkpoint(_ context.Context, pid, checkpointName string) (string, error) {
	instance, err := sm.GetInstance(pid)
	if err != nil {
		return "", err
	}
	log.Info("checkpointing sandbox runtime workspace", "pid", pid, "checkpoint_name", checkpointName, "workspace", instance.Workspace)
	return checkpointRuntimeWorkspace(instance.Workspace, checkpointName)
}

func (sm *SandboxManager) Restore(_ context.Context, pid, checkpointName, snapshot string) error {
	instance, err := sm.GetInstance(pid)
	if err != nil {
		return err
	}
	log.Info("restoring sandbox runtime workspace", "pid", pid, "checkpoint_name", checkpointName, "workspace", instance.Workspace)
	return restoreRuntimeWorkspace(instance.Workspace, checkpointName, snapshot)
}

func (sm *SandboxManager) ensureSandboxCLI(ctx context.Context) error {
	log.Debug("checking docker sandbox cli", "cli_bin", sm.cliBin)
	output, err := exec.CommandContext(ctx, sm.cliBin, "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker CLI is not available: %w", err)
	}
	if !strings.Contains(string(output), "sandbox") {
		return fmt.Errorf("docker sandbox CLI is not available on this machine")
	}
	return nil
}

type dockerImageInspectResult struct {
	ID          string   `json:"Id"`
	RepoDigests []string `json:"RepoDigests"`
}

func (sm *SandboxManager) ensureTemplateExists(ctx context.Context, imageInfo schema.ImageInfo) error {
	if imageInfo.Source == schema.ImageSourceModuleData {
		log.Debug("verifying module-backed sandbox template exists locally", "image", imageInfo.Name, "expected_sha", imageInfo.SHA)
		if imageInfo.SHA == "" {
			return fmt.Errorf("Image-ID is empty")
		}
		return sm.verifyTemplateSHA(ctx, imageInfo)
	}

	log.Debug("ensure sandbox template image exists", "image", imageInfo.Name, "expected_sha", imageInfo.SHA)
	if _, err := sm.inspectTemplateImage(ctx, imageInfo.Name); err == nil {
		log.Debug("sandbox template image already present locally", "image", imageInfo.Name)
		return sm.verifyTemplateSHA(ctx, imageInfo)
	}

	log.Info("pulling sandbox template image", "image", imageInfo.Name)
	if _, err := sm.runSandboxCommand(ctx, "pull", imageInfo.Name); err != nil {
		return fmt.Errorf("failed to pull template image %s: %v", imageInfo.Name, err)
	}

	return sm.verifyTemplateSHA(ctx, imageInfo)
}

func (sm *SandboxManager) verifyTemplateSHA(ctx context.Context, imageInfo schema.ImageInfo) error {
	log.Debug("verifying sandbox template image sha", "image", imageInfo.Name, "expected_sha", imageInfo.SHA)
	inspect, err := sm.inspectTemplateImage(ctx, imageInfo.Name)
	if err != nil {
		return fmt.Errorf("failed to inspect template image %s: %v", imageInfo.Name, err)
	}

	for _, digest := range inspect.RepoDigests {
		if strings.Contains(digest, imageInfo.SHA) {
			return nil
		}
	}
	if inspect.ID == imageInfo.SHA {
		log.Debug("sandbox template image sha matched local image id", "image", imageInfo.Name, "image_id", inspect.ID)
		return nil
	}

	return fmt.Errorf("template image SHA verification failed for %s: expected %s, got digests %v and ID %s",
		imageInfo.Name, imageInfo.SHA, inspect.RepoDigests, inspect.ID)
}

func (sm *SandboxManager) inspectTemplateImage(ctx context.Context, imageName string) (*dockerImageInspectResult, error) {
	log.Debug("inspecting sandbox template image", "image", imageName)
	output, err := sm.runSandboxCommand(ctx, "image", "inspect", imageName)
	if err != nil {
		return nil, err
	}

	var inspect []dockerImageInspectResult
	if err := json.Unmarshal([]byte(output), &inspect); err != nil {
		return nil, fmt.Errorf("parse template image inspect output failed: %w", err)
	}
	if len(inspect) == 0 {
		return nil, fmt.Errorf("template image inspect returned no result for %s", imageName)
	}
	return &inspect[0], nil
}

func (sm *SandboxManager) runSandboxCommand(ctx context.Context, args ...string) (string, error) {
	start := time.Now()
	log.Debug("running sandbox command", "args", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, sm.cliBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		log.Error("sandbox command failed", "args", strings.Join(args, " "), "elapsed", time.Since(start), "output", trimmed)
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, trimmed)
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed != "" {
		log.Debug("sandbox command completed", "args", strings.Join(args, " "), "elapsed", time.Since(start), "output", trimmed)
	} else {
		log.Debug("sandbox command completed", "args", strings.Join(args, " "), "elapsed", time.Since(start))
	}
	return trimmed, nil
}

func (sm *SandboxManager) ExecInstance(ctx context.Context, pid string, env []string, command string) (string, error) {
	instance, err := sm.GetInstance(pid)
	if err != nil {
		return "", err
	}

	return sm.execInstanceAsUser(ctx, instance, "", env, command, true)
}

func (sm *SandboxManager) execInstanceAsUser(ctx context.Context, instance *schema.InstanceInfo, user string, env []string, command string, loginShell bool) (string, error) {
	args := []string{"sandbox", "exec"}
	if user != "" {
		args = append(args, "-u", user)
	}
	for _, item := range env {
		if strings.TrimSpace(item) == "" {
			continue
		}
		args = append(args, "-e", item)
	}
	if strings.TrimSpace(instance.Workspace) != "" {
		args = append(args, "-w", instance.Workspace)
	}
	shellFlag := "-c"
	if loginShell {
		shellFlag = "-lc"
	}
	args = append(args, instance.ID, "sh", shellFlag, command)
	log.Debug("exec into sandbox runtime", "sandbox_name", instance.ID, "user", user, "env_count", len(env), "workdir", instance.Workspace, "login_shell", loginShell, "command", command)
	return sm.runSandboxCommand(ctx, args...)
}

func buildSandboxFilesystemLockdownCommand() string {
	return strings.Join([]string{
		"if [ -d /home/agent ]; then chown -R root:root /home/agent 2>/dev/null || true; chmod -R a-w /home/agent 2>/dev/null || true; chmod 0555 /home/agent 2>/dev/null || true; fi",
		"if [ -d /workspace ]; then chown -R root:root /workspace 2>/dev/null || true; chmod -R a-w /workspace 2>/dev/null || true; chmod 0555 /workspace 2>/dev/null || true; fi",
		"if [ -d /tmp ]; then chown root:root /tmp 2>/dev/null || true; chmod 0755 /tmp 2>/dev/null || true; fi",
		"if [ -d /var/tmp ]; then chown root:root /var/tmp 2>/dev/null || true; chmod 0755 /var/tmp 2>/dev/null || true; fi",
	}, " && ")
}
