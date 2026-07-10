package vmdocker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/capability"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/modulebuild"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager"
	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	vmdockerSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/schema"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/utils"
	"github.com/hymatrix/hymx/common"
	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
	goarSchema "github.com/permadao/goar/schema"
	arutils "github.com/permadao/goar/utils"
)

var log = common.NewLog("vmdocker")

const defaultRuntimeReadyTimeout = 10 * time.Minute
const workspaceCheckpointFormatV1 = "vmdocker.workspace.v1"

type workspaceCheckpoint struct {
	Format                  string                    `json:"format"`
	WorkspaceArchive        string                    `json:"workspaceArchive"`
	WorkspaceCheckpointName string                    `json:"workspaceCheckpointName,omitempty"`
	RuntimeState            string                    `json:"runtimeState"`
	Backend                 string                    `json:"backend"`
	RuntimeSpec             runtimeSchema.RuntimeSpec `json:"runtimeSpec,omitempty"`
	RuntimeEnv              []string                  `json:"runtimeEnv,omitempty"`
}

type restoreRollbackState struct {
	instance      runtimeSchema.InstanceInfo
	runtimeSpec   runtimeSchema.RuntimeSpec
	runtimeEnv    []string
	runtimeState  string
	runtimeManger runtimemanager.IRuntimeManager
}

type runtimeLaunchConfig struct {
	runtimeSpec runtimeSchema.RuntimeSpec
	runtimeEnv  []string
}

func handleRestoreFailure(rollbackWorkspace func() error, restorePreviousRuntime func() error, shouldRestorePreviousRuntime bool, previousRuntimeRestored *bool) {
	if rollbackWorkspace != nil {
		_ = rollbackWorkspace()
	}
	if !shouldRestorePreviousRuntime || previousRuntimeRestored == nil || *previousRuntimeRestored {
		return
	}
	if err := restorePreviousRuntime(); err != nil {
		log.Error("restore previous runtime failed", "err", err)
		return
	}
	*previousRuntimeRestored = true
}

func cloneRuntimeSpec(spec runtimeSchema.RuntimeSpec) runtimeSchema.RuntimeSpec {
	return runtimeSchema.RuntimeSpec{
		Backend:      spec.Backend,
		StartCommand: spec.StartCommand,
		Image: runtimeSchema.ImageInfo{
			Name:          spec.Image.Name,
			SHA:           spec.Image.SHA,
			Source:        spec.Image.Source,
			ArchiveFormat: spec.Image.ArchiveFormat,
		},
		Sandbox: runtimeSchema.SandboxSpec{
			Agent:     spec.Sandbox.Agent,
			Workspace: spec.Sandbox.Workspace,
			Network:   spec.Sandbox.Network,
			Name:      spec.Sandbox.Name,
			Command:   spec.Sandbox.Command,
		},
	}
}

func cloneRuntimeEnv(env []string) []string {
	return append([]string(nil), env...)
}

func checkpointWorkspaceName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "workspace"
	}
	return name
}

func normalizeRuntimeSpecWorkspaceRoot(spec runtimeSchema.RuntimeSpec, targetWorkspace string) runtimeSchema.RuntimeSpec {
	spec = cloneRuntimeSpec(spec)
	if strings.TrimSpace(targetWorkspace) != "" {
		spec.Sandbox.Workspace = runtimeWorkspaceRootFromPath(targetWorkspace)
	}
	return spec
}

func hasRuntimeSpec(spec runtimeSchema.RuntimeSpec) bool {
	return strings.TrimSpace(spec.Backend) != "" ||
		strings.TrimSpace(spec.StartCommand) != "" ||
		strings.TrimSpace(spec.Image.Name) != "" ||
		strings.TrimSpace(spec.Image.SHA) != "" ||
		strings.TrimSpace(spec.Sandbox.Agent) != "" ||
		strings.TrimSpace(spec.Sandbox.Command) != "" ||
		strings.TrimSpace(spec.Sandbox.Network) != "" ||
		strings.TrimSpace(spec.Sandbox.Name) != ""
}

func Spawn(env vmmSchema.Env) (vm vmmSchema.Vm, err error) {
	vmd, err := New(env, env.Process.Scheduler, env.Process.Tags)
	if err != nil {
		return
	}

	err = vmd.Run(env.Process.Scheduler, []byte(env.Meta.Data), env.Process.Tags)
	if err != nil {
		return
	}
	log.Info("spawn process success", "pid", env.Meta.Pid, "from", env.Meta.AccId)
	return vmd, nil
}

type VmDocker struct {
	pid string
	Env vmmSchema.Env
	// runtime info
	instanceInfo *runtimeSchema.InstanceInfo
	// selected runtime manager for this vm instance
	runtimeManager runtimemanager.IRuntimeManager
	// http client
	client *http.Client
	// close channel to signal container shutdown
	closeChan chan struct{}
}

// todo: add cpu, mem
func New(env vmmSchema.Env, nodeAddr string, tags []goarSchema.Tag) (*VmDocker, error) {
	var err error
	env.Process, err = utils.BuildProcessTags(env.Process, nodeAddr, tags)
	if err != nil {
		log.Error("BuildProcessTags failed", "err", err)
		return nil, err
	}
	v := &VmDocker{
		pid: env.Meta.ItemId,
		Env: env,
		client: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true, // No keep-alive
			},
			Timeout: 10 * 60 * time.Second,
		},
		closeChan: make(chan struct{}),
	}
	return v, nil
}

func (v *VmDocker) Run(cuAddr string, data []byte, tags []goarSchema.Tag) error {
	log.Info("starting vm runtime spawn flow", "pid", v.pid, "owner", v.Env.Meta.AccId, "module_format", v.Env.Module.ModuleFormat)
	ctx := context.Background()

	runtimeSpec, err := utils.RuntimeSpecFromModuleAndSpawnTags(v.Env.Module.ModuleFormat, v.Env.Module.Tags, tags)
	if err != nil {
		log.Error("build runtime spec failed", "pid", v.pid, "err", err)
		return err
	}
	runtimeManager, err := runtimemanager.GetRuntimeManager(runtimeSpec.Backend)
	if err != nil {
		log.Error("get runtime manager failed", "pid", v.pid, "backend", runtimeSpec.Backend, "err", err)
		return err
	}
	v.runtimeManager = runtimeManager
	log.Debug("runtime spec resolved", "pid", v.pid, "backend", runtimeSpec.Backend, "image", runtimeSpec.Image.Name, "sandbox_agent", runtimeSpec.Sandbox.Agent, "sandbox_workspace", runtimeSpec.Sandbox.Workspace)
	if err := ensureModuleImageAvailable(ctx, v.Env.Process.Module, runtimeSpec.Image); err != nil {
		log.Error("prepare module image failed", "pid", v.pid, "module", v.Env.Process.Module, "image", runtimeSpec.Image.Name, "err", err)
		return err
	}
	containerEnv := utils.ContainerEnvFromTags(tags)
	log.Debug("runtime env extracted", "pid", v.pid, "env_count", len(containerEnv), "tag_count", len(tags))
	instanceInfo, err := runtimeManager.CreateInstance(ctx, v.pid, runtimeSpec, containerEnv)
	if err != nil {
		log.Error("create runtime failed", "pid", v.pid, "backend", runtimeSpec.Backend, "image", runtimeSpec.Image.Name, "err", err)
		return err
	}
	v.instanceInfo = instanceInfo

	// The instance now holds a container + allocated port. Any failure below
	// (seed, start, readiness, spawn) must tear it down, otherwise the container
	// and port leak — Spawn discards this VmDocker on error and never calls
	// RemoveInstance. Mirrors the restore path's rollback guard.
	spawnFailed := true
	defer func() {
		if spawnFailed && v.instanceInfo != nil {
			_ = runtimeManager.RemoveInstance(ctx, v.pid)
			v.instanceInfo = nil
		}
	}()

	if err := seedWorkspaceFromModule(v.Env.Process.Module, instanceInfo.Workspace, runtimeSpec.Image.ArchiveFormat); err != nil {
		log.Error("seed workspace from module failed", "pid", v.pid, "module", v.Env.Process.Module, "workspace", instanceInfo.Workspace, "err", err)
		return err
	}
	log.Info("runtime instance created", "pid", v.pid, "port", instanceInfo.Port, "runtime_id", instanceInfo.ID, "backend", instanceInfo.Backend)

	log.Debug("starting runtime instance", "pid", v.pid, "runtime_id", instanceInfo.ID)
	startRuntimeStart := time.Now()
	err = runtimeManager.StartInstance(ctx, v.pid)
	if err != nil {
		log.Error("start runtime failed", "pid", v.pid, "runtime_id", instanceInfo.ID, "backend", instanceInfo.Backend, "err", err)
		return err
	}
	log.Info("runtime instance start requested", "pid", v.pid, "runtime_id", instanceInfo.ID)
	log.Debug("runtime instance start elapsed", "pid", v.pid, "runtime_id", instanceInfo.ID, "elapsed", time.Since(startRuntimeStart))

	readyStart := time.Now()
	err = v.waitForContainerReady(ctx, defaultRuntimeReadyTimeout)
	if err != nil {
		log.Error("runtime readiness check failed", "pid", v.pid, "runtime_id", instanceInfo.ID, "backend", instanceInfo.Backend, "err", err)
		return fmt.Errorf("runtime not ready: %v", err)
	}
	log.Debug("runtime readiness confirmed", "pid", v.pid, "runtime_id", instanceInfo.ID, "elapsed", time.Since(readyStart))

	// create ao process
	log.Debug("sending spawn request to runtime", "pid", v.pid, "cu_addr", cuAddr)
	err = v.spawn(vmdockerSchema.SpawnRequest{
		Pid:    v.pid,
		Owner:  v.Env.Meta.AccId,
		CuAddr: cuAddr,
		Data:   data,
		Tags:   tags,
		Evn:    v.Env,
	})
	if err != nil {
		log.Error("runtime spawn request failed", "pid", v.pid, "runtime_id", instanceInfo.ID, "err", err)
		return err
	}
	log.Info("runtime spawn request completed", "pid", v.pid, "runtime_id", instanceInfo.ID)
	spawnFailed = false
	return nil
}

func (v *VmDocker) Apply(from string, meta vmmSchema.Meta) vmmSchema.Result {
	res, err := v.apply(vmdockerSchema.ApplyRequest{
		From:   from,
		Meta:   meta,
		Params: meta.Params,
	})

	if err != nil {
		return vmmSchema.Result{Error: err}
	}
	if res == nil {
		return vmmSchema.Result{Error: fmt.Errorf("apply returned nil result")}
	}
	return *res
}

func (v *VmDocker) Checkpoint() (string, error) {
	if v.instanceInfo == nil {
		return "", fmt.Errorf("instanceInfo is nil, pid: %s", v.pid)
	}

	launchConfig, err := v.currentLaunchConfig()
	if err != nil {
		return "", err
	}

	runtimeManager, err := v.getRuntimeManager()
	if err != nil {
		return "", err
	}

	checkpointName := checkpointWorkspaceName("")
	runtimeState, err := v.runtimeCheckpoint()
	if err != nil {
		return "", err
	}
	workspaceArchive, err := v.checkpointWorkspaceArchive(context.Background(), runtimeManager, checkpointName, launchConfig, runtimeState)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(workspaceCheckpoint{
		Format:                  workspaceCheckpointFormatV1,
		WorkspaceArchive:        workspaceArchive,
		WorkspaceCheckpointName: checkpointName,
		RuntimeState:            runtimeState,
		Backend:                 launchConfig.runtimeSpec.Backend,
		RuntimeSpec:             cloneRuntimeSpec(launchConfig.runtimeSpec),
		RuntimeEnv:              cloneRuntimeEnv(launchConfig.runtimeEnv),
	})
	if err != nil {
		return "", fmt.Errorf("marshal workspace checkpoint failed: %w", err)
	}
	return string(payload), nil
}

func (v *VmDocker) currentLaunchConfig() (runtimeLaunchConfig, error) {
	if v.instanceInfo == nil {
		return runtimeLaunchConfig{}, fmt.Errorf("instanceInfo is nil, pid: %s", v.pid)
	}
	if hasRuntimeSpec(v.instanceInfo.RuntimeSpec) {
		return runtimeLaunchConfig{
			runtimeSpec: normalizeRuntimeSpecWorkspaceRoot(v.instanceInfo.RuntimeSpec, v.instanceInfo.Workspace),
			runtimeEnv:  cloneRuntimeEnv(v.instanceInfo.RuntimeEnv),
		}, nil
	}

	runtimeSpec, err := utils.RuntimeSpecFromModuleAndSpawnTags(v.Env.Module.ModuleFormat, v.Env.Module.Tags, v.Env.Process.Tags)
	if err != nil {
		return runtimeLaunchConfig{}, err
	}
	runtimeSpec.Backend = v.instanceInfo.Backend
	runtimeSpec = normalizeRuntimeSpecWorkspaceRoot(runtimeSpec, v.instanceInfo.Workspace)
	return runtimeLaunchConfig{
		runtimeSpec: runtimeSpec,
		runtimeEnv:  utils.ContainerEnvFromTags(v.Env.Process.Tags),
	}, nil
}

func resolveRestoreLaunchConfig(checkpoint workspaceCheckpoint, fallbackSpec runtimeSchema.RuntimeSpec, fallbackEnv []goarSchema.Tag, targetWorkspace string) (runtimeLaunchConfig, error) {
	spec := checkpoint.RuntimeSpec
	fromCheckpointSpec := hasRuntimeSpec(spec)
	if !fromCheckpointSpec {
		spec = cloneRuntimeSpec(fallbackSpec)
	}
	if checkpoint.Backend != "" {
		if fromCheckpointSpec && spec.Backend != "" && spec.Backend != checkpoint.Backend {
			return runtimeLaunchConfig{}, fmt.Errorf("checkpoint backend mismatch: runtimeSpec=%s backend=%s", spec.Backend, checkpoint.Backend)
		}
		spec.Backend = checkpoint.Backend
	}
	if spec.Backend == "" {
		return runtimeLaunchConfig{}, fmt.Errorf("checkpoint runtime backend is empty")
	}
	spec = normalizeRuntimeSpecWorkspaceRoot(spec, targetWorkspace)

	runtimeEnv := checkpoint.RuntimeEnv
	if len(runtimeEnv) == 0 {
		runtimeEnv = utils.ContainerEnvFromTags(fallbackEnv)
	}
	return runtimeLaunchConfig{
		runtimeSpec: spec,
		runtimeEnv:  cloneRuntimeEnv(runtimeEnv),
	}, nil
}

func (v *VmDocker) restoreRuntimeLaunch(ctx context.Context, runtimeManager runtimemanager.IRuntimeManager, launchConfig runtimeLaunchConfig, runtimeState string) error {
	instanceInfo, err := runtimeManager.CreateInstance(ctx, v.pid, launchConfig.runtimeSpec, launchConfig.runtimeEnv)
	if err != nil {
		return fmt.Errorf("create runtime failed: %w", err)
	}
	v.instanceInfo = instanceInfo

	restoreFailed := true
	defer func() {
		if restoreFailed && v.instanceInfo != nil {
			_ = runtimeManager.RemoveInstance(ctx, v.pid)
			v.instanceInfo = nil
		}
	}()

	if err := runtimeManager.StartInstance(ctx, v.pid); err != nil {
		return fmt.Errorf("start runtime failed: %w", err)
	}
	if err := v.waitForContainerReady(ctx, defaultRuntimeReadyTimeout); err != nil {
		return fmt.Errorf("runtime not ready: %w", err)
	}
	if err := v.runtimeRestore(runtimeState); err != nil {
		return fmt.Errorf("restore runtime state failed: %w", err)
	}

	restoreFailed = false
	return nil
}

func (v *VmDocker) checkpointWorkspaceArchive(ctx context.Context, runtimeManager runtimemanager.IRuntimeManager, checkpointName string, launchConfig runtimeLaunchConfig, runtimeState string) (string, error) {
	if v.instanceInfo == nil {
		return "", fmt.Errorf("instanceInfo is nil, pid: %s", v.pid)
	}

	currentInstance := *v.instanceInfo
	if err := runtimeManager.RemoveInstance(ctx, v.pid); err != nil {
		return "", fmt.Errorf("remove runtime for checkpoint failed: %w", err)
	}
	v.instanceInfo = nil

	restoreCurrentRuntime := func(cause error) error {
		if restoreErr := v.restoreRuntimeLaunch(ctx, runtimeManager, launchConfig, runtimeState); restoreErr != nil {
			return fmt.Errorf("%w; restore current runtime failed: %v", cause, restoreErr)
		}
		return cause
	}

	workspaceArchive, err := runtimemanager.CheckpointRuntimeWorkspace(currentInstance.Workspace, checkpointName)
	if err != nil {
		return "", restoreCurrentRuntime(err)
	}
	if err := v.restoreRuntimeLaunch(ctx, runtimeManager, launchConfig, runtimeState); err != nil {
		return "", fmt.Errorf("restore current runtime after checkpoint failed: %w", err)
	}
	return workspaceArchive, nil
}

func (v *VmDocker) Restore(snapshot string) error {
	var checkpoint workspaceCheckpoint
	if err := json.Unmarshal([]byte(snapshot), &checkpoint); err != nil {
		return fmt.Errorf("decode workspace checkpoint failed: %w", err)
	}
	if checkpoint.Format != workspaceCheckpointFormatV1 {
		return fmt.Errorf("unsupported workspace checkpoint format: %s", checkpoint.Format)
	}

	ctx := context.Background()
	fallbackRuntimeSpec, err := utils.RuntimeSpecFromModuleAndSpawnTags(v.Env.Module.ModuleFormat, v.Env.Module.Tags, v.Env.Process.Tags)
	if err != nil {
		return err
	}

	targetWorkspace, err := v.restoreTargetWorkspace(fallbackRuntimeSpec)
	if err != nil {
		return err
	}
	restoreLaunchConfig, err := resolveRestoreLaunchConfig(checkpoint, fallbackRuntimeSpec, v.Env.Process.Tags, targetWorkspace)
	if err != nil {
		return err
	}
	runtimeManager, err := runtimemanager.GetRuntimeManager(restoreLaunchConfig.runtimeSpec.Backend)
	if err != nil {
		return err
	}
	if err := ensureModuleImageAvailable(ctx, v.Env.Process.Module, restoreLaunchConfig.runtimeSpec.Image); err != nil {
		return fmt.Errorf("prepare module image failed: %w", err)
	}

	stagedWorkspace, cleanupStagedWorkspace, err := runtimemanager.StageRuntimeWorkspaceRestore(
		targetWorkspace,
		checkpointWorkspaceName(checkpoint.WorkspaceCheckpointName),
		checkpoint.WorkspaceArchive,
	)
	if err != nil {
		return err
	}
	defer cleanupStagedWorkspace()

	var rollbackState *restoreRollbackState
	var rollbackWorkspace func() error
	workspaceCommitted := false
	previousRuntimeRestored := false
	previousRuntimeRemoved := false
	defer func() {
		if workspaceCommitted {
			return
		}
		handleRestoreFailure(
			rollbackWorkspace,
			func() error {
				return v.restorePreviousRuntime(ctx, rollbackState)
			},
			rollbackState != nil && previousRuntimeRemoved,
			&previousRuntimeRestored,
		)
	}()

	if v.instanceInfo != nil {
		currentRuntimeState, err := v.runtimeCheckpoint()
		if err != nil {
			return fmt.Errorf("checkpoint current runtime before restore failed: %w", err)
		}
		currentRuntimeManager, err := v.getRuntimeManager()
		if err != nil {
			return err
		}
		currentInstance := *v.instanceInfo
		rollbackLaunchConfig, err := v.currentLaunchConfig()
		if err != nil {
			return err
		}
		rollbackState = &restoreRollbackState{
			instance:      currentInstance,
			runtimeSpec:   rollbackLaunchConfig.runtimeSpec,
			runtimeEnv:    rollbackLaunchConfig.runtimeEnv,
			runtimeState:  currentRuntimeState,
			runtimeManger: currentRuntimeManager,
		}
		if err := currentRuntimeManager.RemoveInstance(ctx, v.pid); err != nil {
			return fmt.Errorf("remove provisional runtime failed: %w", err)
		}
		v.instanceInfo = nil
		previousRuntimeRemoved = true
	}

	rollbackWorkspace, commitWorkspace, err := runtimemanager.PromoteRuntimeWorkspace(targetWorkspace, stagedWorkspace)
	if err != nil {
		return err
	}

	v.runtimeManager = runtimeManager
	instanceInfo, err := runtimeManager.CreateInstance(ctx, v.pid, restoreLaunchConfig.runtimeSpec, restoreLaunchConfig.runtimeEnv)
	if err != nil {
		return err
	}
	v.instanceInfo = instanceInfo

	restoreFailed := true
	defer func() {
		if restoreFailed && v.instanceInfo != nil {
			_ = runtimeManager.RemoveInstance(ctx, v.pid)
			v.instanceInfo = nil
		}
	}()

	if err := runtimeManager.StartInstance(ctx, v.pid); err != nil {
		return err
	}
	if err := v.waitForContainerReady(ctx, defaultRuntimeReadyTimeout); err != nil {
		return fmt.Errorf("runtime not ready after restore: %v", err)
	}
	if err := v.runtimeRestore(checkpoint.RuntimeState); err != nil {
		return err
	}
	restoreFailed = false
	workspaceCommitted = true
	if err := commitWorkspace(); err != nil {
		log.Warn("remove restore workspace backup failed", "pid", v.pid, "err", err)
	}
	return nil
}

func (v *VmDocker) getRuntimeManager() (runtimemanager.IRuntimeManager, error) {
	if v.runtimeManager != nil {
		return v.runtimeManager, nil
	}

	backend := ""
	if v.instanceInfo != nil {
		backend = v.instanceInfo.Backend
	}

	runtimeManager, err := runtimemanager.GetRuntimeManager(backend)
	if err != nil {
		return nil, err
	}
	v.runtimeManager = runtimeManager
	return runtimeManager, nil
}

func (v *VmDocker) Close() error {
	// Signal waitForContainerReady to exit immediately
	select {
	case v.closeChan <- struct{}{}:
	default:
		// Channel might be full or closed, ignore
	}

	runtimeManager, err := v.getRuntimeManager()
	if err != nil {
		log.Error("get runtime manager failed", "err", err)
		return err
	}
	log.Info("closing vm runtime", "pid", v.pid, "runtime_id", func() string {
		if v.instanceInfo == nil {
			return ""
		}
		return v.instanceInfo.ID
	}())
	return runtimeManager.RemoveInstance(context.Background(), v.pid)
}

// waitForContainerReady waits for the runtime to be ready by checking health endpoint.
func (v *VmDocker) waitForContainerReady(ctx context.Context, timeout time.Duration) error {
	if v.instanceInfo == nil {
		return fmt.Errorf("instanceInfo is nil")
	}

	startTime := time.Now()
	log.Debug("waiting for runtime to be ready", "pid", v.pid, "port", v.instanceInfo.Port)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			elapsedTime := time.Since(startTime)
			log.Debug("runtime health check timeout", "pid", v.pid, "elapsed_time", elapsedTime)
			return fmt.Errorf("timeout waiting for runtime to be ready")
		case <-v.closeChan:
			elapsedTime := time.Since(startTime)
			log.Debug("runtime closed during health check", "pid", v.pid, "elapsed_time", elapsedTime)
			return fmt.Errorf("runtime was closed")
		case <-ticker.C:
			statusCode, err := v.healthStatusCode(ctx)
			if err != nil {
				log.Debug("runtime health check failed", "pid", v.pid, "err", err)
				continue
			}
			log.Debug("runtime health check returned", "pid", v.pid, "status_code", statusCode)

			if statusCode == http.StatusOK {
				elapsedTime := time.Since(startTime)
				log.Debug("runtime ready", "pid", v.pid, "elapsed_time", elapsedTime)
				return nil
			}
		}
	}
}

func (v *VmDocker) spawn(msg vmdockerSchema.SpawnRequest) error {
	log.Debug("spawn process", "pid", v.pid, "owner", msg.Owner, "tag_count", len(msg.Tags))

	// safe check
	if v.instanceInfo == nil {
		return fmt.Errorf("instanceInfo is nil, pid: %s", v.pid)
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal request failed: %v", err)
	}

	statusCode, body, err := v.callRuntimeEndpoint(context.Background(), "/vmm/spawn", jsonData)
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d: %s", statusCode, string(body))
	}
	log.Debug("spawn request accepted", "pid", v.pid, "status_code", statusCode, "body", string(body))

	return nil
}

func (v *VmDocker) apply(msg vmdockerSchema.ApplyRequest) (outbox *vmmSchema.Result, err error) {
	// safe check
	if v.instanceInfo == nil {
		err = fmt.Errorf("instanceInfo is nil, pid: %s", v.pid)
		return
	}
	if result, handled := v.applyCapabilityAction(msg.Meta); handled {
		return result, nil
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		err = fmt.Errorf("marshal request failed: %v", err)
		return
	}
	log.Debug("===> apply request", "pid", v.pid, "msg", string(jsonData))

	statusCode, body, err := v.callRuntimeEndpoint(context.Background(), "/vmm/apply", jsonData)
	if err != nil {
		return
	}
	if statusCode != http.StatusOK {
		err = fmt.Errorf("request failed with status %d: %s", statusCode, string(body))
		return
	}

	var resOutbox vmdockerSchema.OutboxResponse
	err = json.Unmarshal(body, &resOutbox)
	if err != nil {
		log.Error("unmarshal response failed", "err", err)
		return
	}
	log.Debug("===> apply success", "result", resOutbox)

	outbox = &vmmSchema.Result{}
	if err = json.Unmarshal([]byte(resOutbox.Result), outbox); err != nil {
		log.Error("unmarshal response outbox failed", "err", err)
	}

	return
}

func (v *VmDocker) applyCapabilityAction(meta vmmSchema.Meta) (*vmmSchema.Result, bool) {
	switch strings.ToLower(strings.TrimSpace(meta.Action)) {
	case "export":
		return v.applyCapabilityExport(meta), true
	default:
		return nil, false
	}
}

func (v *VmDocker) applyCapabilityExport(meta vmmSchema.Meta) *vmmSchema.Result {
	home := v.instanceInfo.Workspace
	profileTOML, err := os.ReadFile(filepath.Join(home, "profile.toml"))
	if err != nil {
		return &vmmSchema.Result{Error: err}
	}
	profile, err := modulebuild.ParseProfile(profileTOML)
	if err != nil {
		return &vmmSchema.Result{Error: err}
	}
	if truthyParam(meta.Params, "dry_run", "Dry-Run") {
		col, err := capability.Preview(home, profile.Vmdocker.Public)
		if err != nil {
			return &vmmSchema.Result{Error: err}
		}
		return &vmmSchema.Result{Output: col}
	}
	// Option A: reuse the running agent's existing image (the one it was spawned
	// from) instead of rebuilding — the build inputs (bin/, start.sh) are baked in
	// the image, not the runtime workspace. RuntimeSpec.Image.SHA carries the
	// original Image-ID; both tags must be non-empty for the exported module to
	// spawn again.
	image := v.instanceInfo.RuntimeSpec.Image
	imageArchive, err := readModuleImageArchive(v.Env.Process.Module, image.ArchiveFormat)
	if err != nil {
		return &vmmSchema.Result{Error: err}
	}
	exported, err := capability.Export(home, capability.ExportOptions{
		ImageArchive: imageArchive,
		ImageName:    image.Name,
		ImageID:      image.SHA,
		SignerKey:    os.Getenv("VMDOCKER_MODULE_SIGNER_KEY"),
	})
	if err != nil {
		return &vmmSchema.Result{Error: err}
	}
	return &vmmSchema.Result{
		Output: exported.Collection,
		Data:   arutils.Base64Encode(exported.ModuleBytes),
	}
}

func truthyParam(params map[string]string, keys ...string) bool {
	value := strings.ToLower(strings.TrimSpace(paramValue(params, keys...)))
	return value == "1" || value == "true" || value == "yes"
}

func paramValue(params map[string]string, keys ...string) string {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			return value
		}
	}
	return ""
}

func (v *VmDocker) healthStatusCode(ctx context.Context) (int, error) {
	statusCode, _, err := v.callRuntimeEndpoint(ctx, "/vmm/health", nil)
	return statusCode, err
}

func (v *VmDocker) callRuntimeEndpoint(ctx context.Context, path string, payload []byte) (int, []byte, error) {
	if v.instanceInfo == nil {
		return 0, nil, fmt.Errorf("instanceInfo is nil, pid: %s", v.pid)
	}

	if v.instanceInfo.Backend == runtimeSchema.RuntimeBackendSandbox {
		return v.callSandboxRuntimeEndpoint(ctx, path, payload)
	}
	return v.callDockerRuntimeEndpoint(path, payload)
}

func (v *VmDocker) callDockerRuntimeEndpoint(path string, payload []byte) (int, []byte, error) {
	url := fmt.Sprintf("http://%s:%d%s", runtimeSchema.AllowHost, v.instanceInfo.Port, path)
	log.Debug("calling docker runtime endpoint", "pid", v.pid, "path", path, "url", url)

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	resp, err := v.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("send request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read response body failed: %v", err)
	}
	log.Debug("docker runtime endpoint returned", "pid", v.pid, "path", path, "status_code", resp.StatusCode, "body", string(body))
	return resp.StatusCode, body, nil
}

func (v *VmDocker) callSandboxRuntimeEndpoint(ctx context.Context, path string, payload []byte) (int, []byte, error) {
	runtimeManager, err := v.getRuntimeManager()
	if err != nil {
		return 0, nil, fmt.Errorf("get runtime manager failed: %v", err)
	}

	command := ""
	cleanup := func() {}
	if len(payload) > 0 {
		payloadPath, err := v.writeSandboxPayloadFile(payload)
		if err != nil {
			return 0, nil, err
		}
		cleanup = func() {
			_ = os.Remove(payloadPath)
		}
		command = buildSandboxCurlCommandFromFile(path, payloadPath)
	} else {
		command = buildSandboxCurlCommand(path, payload)
	}
	defer cleanup()
	log.Debug("calling sandbox runtime endpoint", "pid", v.pid, "path", path, "command", command)
	output, err := runtimeManager.ExecInstance(ctx, v.pid, nil, command)
	if err != nil {
		return 0, nil, fmt.Errorf("sandbox exec failed: %v", err)
	}

	statusCode, body, err := parseSandboxCurlOutput(output)
	if err != nil {
		return 0, nil, err
	}
	log.Debug("sandbox runtime endpoint returned", "pid", v.pid, "path", path, "status_code", statusCode, "body", string(body))
	return statusCode, body, nil
}

func (v *VmDocker) writeSandboxPayloadFile(payload []byte) (string, error) {
	if v.instanceInfo == nil {
		return "", fmt.Errorf("instanceInfo is nil, pid: %s", v.pid)
	}
	if strings.TrimSpace(v.instanceInfo.Workspace) == "" {
		return "", fmt.Errorf("sandbox workspace is empty, pid: %s", v.pid)
	}
	payloadDir := filepath.Join(v.instanceInfo.Workspace, ".tmp")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		return "", fmt.Errorf("create sandbox payload dir failed: %w", err)
	}
	payloadPath := filepath.Join(payloadDir, fmt.Sprintf("runtime-request-%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		return "", fmt.Errorf("write sandbox payload failed: %w", err)
	}
	return payloadPath, nil
}

func buildSandboxCurlCommand(path string, payload []byte) string {
	url := "http://127.0.0.1:8080" + path
	body := ""
	if payload != nil {
		body = string(payload)
	}
	return fmt.Sprintf("curl -sS -X POST -H %s --data-raw %s %s -w '\\n__STATUS__:%%{http_code}'",
		shellEscape("Content-Type: application/json"),
		shellEscape(body),
		shellEscape(url),
	)
}

func buildSandboxCurlCommandFromFile(path, payloadPath string) string {
	url := "http://127.0.0.1:8080" + path
	return fmt.Sprintf("curl -sS -X POST -H %s --data-binary @%s %s -w '\\n__STATUS__:%%{http_code}'",
		shellEscape("Content-Type: application/json"),
		shellEscape(payloadPath),
		shellEscape(url),
	)
}

func parseSandboxCurlOutput(output string) (int, []byte, error) {
	idx := strings.LastIndex(output, "\n__STATUS__:")
	if idx == -1 {
		return 0, nil, fmt.Errorf("sandbox response missing status marker: %s", output)
	}
	statusText := strings.TrimSpace(output[idx+len("\n__STATUS__:"):])
	statusCode, err := strconv.Atoi(statusText)
	if err != nil {
		return 0, nil, fmt.Errorf("parse sandbox status failed: %w", err)
	}
	body := []byte(output[:idx])
	return statusCode, body, nil
}

func (v *VmDocker) runtimeCheckpoint() (string, error) {
	statusCode, body, err := v.callRuntimeEndpoint(context.Background(), "/vmm/checkpoint", nil)
	if err != nil {
		return "", err
	}
	if statusCode != http.StatusOK {
		return "", fmt.Errorf("checkpoint request failed with status %d: %s", statusCode, string(body))
	}

	var response vmdockerSchema.RuntimeCheckpointResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode runtime checkpoint response failed: %w", err)
	}
	if response.Status != "ok" {
		return "", fmt.Errorf("runtime checkpoint failed: %s", string(body))
	}
	return response.State, nil
}

func (v *VmDocker) runtimeRestore(state string) error {
	payload, err := json.Marshal(vmdockerSchema.RuntimeRestoreRequest{
		Env:   v.Env,
		Tags:  v.Env.Process.Tags,
		State: state,
	})
	if err != nil {
		return fmt.Errorf("marshal runtime restore request failed: %w", err)
	}

	statusCode, body, err := v.callRuntimeEndpoint(context.Background(), "/vmm/restore", payload)
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("restore request failed with status %d: %s", statusCode, string(body))
	}

	var response vmdockerSchema.RuntimeRestoreResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("decode runtime restore response failed: %w", err)
	}
	if response.Status != "ok" {
		return fmt.Errorf("runtime restore failed: %s", string(body))
	}
	return nil
}

func (v *VmDocker) restoreTargetWorkspace(runtimeSpec runtimeSchema.RuntimeSpec) (string, error) {
	if v.instanceInfo != nil && strings.TrimSpace(v.instanceInfo.Workspace) != "" {
		return v.instanceInfo.Workspace, nil
	}

	root := runtimeSpec.Sandbox.Workspace
	var err error
	if root == "" {
		root, err = os.Getwd()
		if err != nil {
			return "", err
		}
	} else {
		root, err = filepath.Abs(root)
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(root, "sandbox_workspace", v.pid), nil
}

func runtimeWorkspaceRootFromPath(workspace string) string {
	return filepath.Dir(filepath.Dir(workspace))
}

func (v *VmDocker) restorePreviousRuntime(ctx context.Context, rollbackState *restoreRollbackState) error {
	if rollbackState == nil {
		return nil
	}

	v.runtimeManager = rollbackState.runtimeManger
	return v.restoreRuntimeLaunch(ctx, rollbackState.runtimeManger, runtimeLaunchConfig{
		runtimeSpec: rollbackState.runtimeSpec,
		runtimeEnv:  rollbackState.runtimeEnv,
	}, rollbackState.runtimeState)
}

func shellEscape(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
