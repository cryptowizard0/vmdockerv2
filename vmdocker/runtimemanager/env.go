package runtimemanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/utils"
)

const (
	openclawStateDirName               = ".openclaw"
	openclawConfigFile                 = "openclaw.json"
	openclawWorkspaceDir               = "workspace"
	sandboxTmpDirName                  = ".tmp"
	sandboxXDGDirName                  = ".xdg"
	containerHome                      = "/home/hymx"
	envOpenclawHome                    = "OPENCLAW_HOME"
	envOpenclawStateDir                = "OPENCLAW_STATE_DIR"
	envOpenclawConfigPath              = "OPENCLAW_CONFIG_PATH"
	envOpenclawWorkspace               = "OPENCLAW_AGENT_WORKSPACE"
	envRuntimeWorkspace                = "VMDOCKER_RUNTIME_WORKSPACE"
	envRuntimeHome                     = "VMDOCKER_RUNTIME_HOME"
	envRuntimeAgentWork                = "VMDOCKER_AGENT_WORKSPACE"
	envHome                            = "HOME"
	envTmpDir                          = "TMPDIR"
	envXDGConfigHome                   = "XDG_CONFIG_HOME"
	envXDGCacheHome                    = "XDG_CACHE_HOME"
	envXDGStateHome                    = "XDG_STATE_HOME"
	runtimeWorkspaceDir                = "sandbox_workspace"
	runtimeWorkspaceCheckpointFormatV1 = "vmdocker.runtime-workspace.v1"
)

type runtimeWorkspaceCheckpoint struct {
	Format  string `json:"format"`
	Name    string `json:"name"`
	Archive string `json:"archive"`
}

func ensureRuntimeWorkspace(pid, root string) (string, error) {
	workspace, err := resolveRuntimeWorkspace(pid, root)
	if err != nil {
		return "", err
	}
	if err := ensureRuntimeWorkspaceLayout(workspace); err != nil {
		return "", err
	}
	return workspace, nil
}

func ensureRuntimeWorkspaceRoot(pid, root string) (string, error) {
	workspace, err := resolveRuntimeWorkspace(pid, root)
	if err != nil {
		return "", err
	}
	if err := ensureRuntimeWorkspaceDirs([]string{workspace}); err != nil {
		return "", err
	}
	return workspace, nil
}

func ensureRuntimeWorkspaceLayout(workspace string) error {
	return ensureRuntimeWorkspaceDirs(runtimeWorkspaceLayoutDirs(workspace))
}

func resolveRuntimeWorkspace(pid, root string) (string, error) {
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
	return filepath.Join(root, runtimeWorkspaceDir, pid), nil
}

func runtimeWorkspaceRootFromPath(workspace string) string {
	return filepath.Dir(filepath.Dir(workspace))
}

func appendRuntimePersistenceEnv(runtimeEnv []string, workspace string) []string {
	env := append([]string(nil), runtimeEnv...)
	if workspace == "" {
		return env
	}

	home := envValue(env, envHome, containerHome)
	stateDir := envValue(env, envOpenclawStateDir, filepath.Join(home, openclawStateDirName))
	agentWorkspace := envValue(env, envOpenclawWorkspace, filepath.Join(stateDir, openclawWorkspaceDir))
	tmpDir := envValue(env, envTmpDir, filepath.Join(home, sandboxTmpDirName))
	xdgConfigHome := envValue(env, envXDGConfigHome, filepath.Join(home, sandboxXDGDirName, "config"))
	xdgCacheHome := envValue(env, envXDGCacheHome, filepath.Join(home, sandboxXDGDirName, "cache"))
	xdgStateHome := envValue(env, envXDGStateHome, filepath.Join(home, sandboxXDGDirName, "state"))
	runtimeAgentWorkspace := envValue(env, envRuntimeAgentWork, filepath.Join(home, openclawWorkspaceDir))

	set := func(key, val string) {
		if !hasEnvKey(env, key) {
			env = append(env, key+"="+val)
		}
	}
	set(envOpenclawStateDir, stateDir)
	set(envOpenclawHome, home)
	set(envOpenclawConfigPath, filepath.Join(stateDir, openclawConfigFile))
	set(envOpenclawWorkspace, agentWorkspace)
	set(envRuntimeWorkspace, home)
	set(envRuntimeHome, home)
	set(envRuntimeAgentWork, runtimeAgentWorkspace)
	set(envHome, home)
	set(envTmpDir, tmpDir)
	set(envXDGConfigHome, xdgConfigHome)
	set(envXDGCacheHome, xdgCacheHome)
	set(envXDGStateHome, xdgStateHome)
	return env
}

func runtimeWorkspaceLayoutDirs(workspace string) []string {
	return []string{
		workspace,
		filepath.Join(workspace, openclawStateDirName),
		filepath.Join(workspace, openclawStateDirName, openclawWorkspaceDir),
		filepath.Join(workspace, openclawWorkspaceDir),
		filepath.Join(workspace, sandboxTmpDirName),
		filepath.Join(workspace, sandboxXDGDirName, "config"),
		filepath.Join(workspace, sandboxXDGDirName, "cache"),
		filepath.Join(workspace, sandboxXDGDirName, "state"),
	}
}

func ensureRuntimeWorkspaceDirs(dirs []string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return fmt.Errorf("create runtime workspace dir %s failed: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o777); err != nil {
			return fmt.Errorf("chmod runtime workspace dir %s failed: %w", dir, err)
		}
	}
	return nil
}

func normalizeWorkspaceCheckpointName(checkpointName string) string {
	if strings.TrimSpace(checkpointName) == "" {
		return "workspace"
	}
	return checkpointName
}

func checkpointRuntimeWorkspace(workspace, checkpointName string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("runtime workspace is empty")
	}
	archive, err := utils.CompressDirectory(workspace)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(runtimeWorkspaceCheckpoint{
		Format:  runtimeWorkspaceCheckpointFormatV1,
		Name:    normalizeWorkspaceCheckpointName(checkpointName),
		Archive: archive,
	})
	if err != nil {
		return "", fmt.Errorf("marshal runtime workspace checkpoint failed: %w", err)
	}
	return string(payload), nil
}

func normalizeRuntimeWorkspacePath(workspace string) (string, error) {
	cleanedWorkspace, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return "", err
	}
	if cleanedWorkspace == string(os.PathSeparator) {
		return "", fmt.Errorf("refusing to use root path as runtime workspace")
	}
	return cleanedWorkspace, nil
}

func decodeRuntimeWorkspaceSnapshot(snapshot, checkpointName string) (string, error) {
	if strings.TrimSpace(snapshot) == "" {
		return "", fmt.Errorf("runtime workspace snapshot is empty")
	}

	var payload runtimeWorkspaceCheckpoint
	if err := json.Unmarshal([]byte(snapshot), &payload); err != nil || payload.Format == "" {
		return snapshot, nil
	}
	if payload.Format != runtimeWorkspaceCheckpointFormatV1 {
		return "", fmt.Errorf("unsupported runtime workspace checkpoint format: %s", payload.Format)
	}
	expectedName := normalizeWorkspaceCheckpointName(checkpointName)
	if payload.Name != expectedName {
		return "", fmt.Errorf("runtime workspace checkpoint name mismatch: got %s want %s", payload.Name, expectedName)
	}
	if strings.TrimSpace(payload.Archive) == "" {
		return "", fmt.Errorf("runtime workspace checkpoint archive is empty")
	}
	return payload.Archive, nil
}

// CheckpointRuntimeWorkspace creates a named workspace snapshot payload.
func CheckpointRuntimeWorkspace(workspace, checkpointName string) (string, error) {
	return checkpointRuntimeWorkspace(workspace, checkpointName)
}

func stageRuntimeWorkspaceRestore(workspace, checkpointName, snapshot string) (string, func(), error) {
	cleanedWorkspace, err := normalizeRuntimeWorkspacePath(workspace)
	if err != nil {
		return "", nil, err
	}
	archive, err := decodeRuntimeWorkspaceSnapshot(snapshot, checkpointName)
	if err != nil {
		return "", nil, err
	}

	parentDir := filepath.Dir(cleanedWorkspace)
	prefix := filepath.Base(cleanedWorkspace) + ".restore-"
	stagedWorkspace, err := os.MkdirTemp(parentDir, prefix)
	if err != nil {
		return "", nil, fmt.Errorf("create staged runtime workspace failed: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(stagedWorkspace)
	}

	if err := utils.DecompressToDirectory(archive, stagedWorkspace); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("restore staged runtime workspace %s failed: %w", stagedWorkspace, err)
	}
	if err := ensureRuntimeWorkspaceLayout(stagedWorkspace); err != nil {
		cleanup()
		return "", nil, err
	}
	return stagedWorkspace, cleanup, nil
}

// StageRuntimeWorkspaceRestore validates and unpacks a workspace snapshot into a temporary sibling directory.
// The caller owns the returned cleanup function and should remove the staged directory if it is not promoted.
func StageRuntimeWorkspaceRestore(workspace, checkpointName, snapshot string) (string, func(), error) {
	return stageRuntimeWorkspaceRestore(workspace, checkpointName, snapshot)
}

func restoreRuntimeWorkspace(workspace, checkpointName, snapshot string) error {
	stagedWorkspace, cleanup, err := stageRuntimeWorkspaceRestore(workspace, checkpointName, snapshot)
	if err != nil {
		return err
	}
	defer cleanup()

	rollback, commit, err := promoteRuntimeWorkspace(workspace, stagedWorkspace)
	if err != nil {
		return err
	}
	if err := commit(); err != nil {
		_ = rollback()
		return err
	}
	return nil
}

func promoteRuntimeWorkspace(workspace, stagedWorkspace string) (rollback func() error, commit func() error, err error) {
	cleanedWorkspace, err := normalizeRuntimeWorkspacePath(workspace)
	if err != nil {
		return nil, nil, err
	}
	cleanedStagedWorkspace, err := normalizeRuntimeWorkspacePath(stagedWorkspace)
	if err != nil {
		return nil, nil, err
	}
	parentDir := filepath.Dir(cleanedWorkspace)
	backupWorkspace := filepath.Join(parentDir, fmt.Sprintf("%s.backup-%d", filepath.Base(cleanedWorkspace), time.Now().UnixNano()))
	hasBackup := false

	if _, err := os.Stat(cleanedWorkspace); err == nil {
		if err := os.Rename(cleanedWorkspace, backupWorkspace); err != nil {
			return nil, nil, fmt.Errorf("backup runtime workspace %s failed: %w", cleanedWorkspace, err)
		}
		hasBackup = true
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("inspect runtime workspace %s failed: %w", cleanedWorkspace, err)
	}

	if err := os.Rename(cleanedStagedWorkspace, cleanedWorkspace); err != nil {
		if hasBackup {
			_ = os.Rename(backupWorkspace, cleanedWorkspace)
		}
		return nil, nil, fmt.Errorf("activate staged runtime workspace %s failed: %w", cleanedStagedWorkspace, err)
	}

	rollback = func() error {
		if err := os.RemoveAll(cleanedWorkspace); err != nil {
			return fmt.Errorf("remove activated runtime workspace %s failed: %w", cleanedWorkspace, err)
		}
		if hasBackup {
			if err := os.Rename(backupWorkspace, cleanedWorkspace); err != nil {
				return fmt.Errorf("restore runtime workspace backup %s failed: %w", backupWorkspace, err)
			}
		}
		return nil
	}

	commit = func() error {
		if !hasBackup {
			return nil
		}
		if err := os.RemoveAll(backupWorkspace); err != nil {
			return fmt.Errorf("remove runtime workspace backup %s failed: %w", backupWorkspace, err)
		}
		return nil
	}
	return rollback, commit, nil
}

// PromoteRuntimeWorkspace swaps a staged workspace into the active location.
// The caller should invoke commit after the restored runtime is healthy, or rollback on failure.
func PromoteRuntimeWorkspace(workspace, stagedWorkspace string) (rollback func() error, commit func() error, err error) {
	return promoteRuntimeWorkspace(workspace, stagedWorkspace)
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func envValue(env []string, key, fallback string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return fallback
}
