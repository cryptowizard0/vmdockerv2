package runtimemanager

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	ContainerNamePrefix = "hymatrix_"
)

type DockerManager struct {
	cli           *client.Client
	cliBin        string
	instances     map[string]*schema.InstanceInfo
	mutex         sync.RWMutex
	portAllocator *portAllocator
}

func newDockerManager() (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithVersion(schema.DockerVersion),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %v", err)
	}

	cliBin, err := exec.LookPath("docker")
	if err != nil {
		cliBin = "docker"
	}

	return &DockerManager{
		cli:           cli,
		cliBin:        cliBin,
		instances:     make(map[string]*schema.InstanceInfo),
		portAllocator: newPortAllocator(10000, 20000),
	}, nil
}

func (dm *DockerManager) ensureImageExists(ctx context.Context, imageInfo schema.ImageInfo) error {
	if imageInfo.Source == schema.ImageSourceModuleData {
		log.Debug("verifying module-backed docker image exists locally", "image", imageInfo.Name, "expected_sha", imageInfo.SHA)
		if imageInfo.SHA == "" {
			return fmt.Errorf("Image-ID is empty")
		}
		return dm.verifyImageSHA(ctx, imageInfo)
	}

	log.Debug("ensure docker image exists", "image", imageInfo.Name, "expected_sha", imageInfo.SHA)
	_, err := dm.cli.ImageInspect(ctx, imageInfo.Name)
	if err == nil {
		log.Debug("docker image already present locally", "image", imageInfo.Name)
		if imageInfo.SHA != "" {
			return dm.verifyImageSHA(ctx, imageInfo)
		}
		return nil
	}

	log.Info("pulling docker image", "image", imageInfo.Name)
	reader, err := dm.cli.ImagePull(ctx, imageInfo.Name, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %v", imageInfo.Name, err)
	}
	defer reader.Close()

	if _, err = io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("failed to read pull output: %v", err)
	}

	if imageInfo.SHA != "" {
		return dm.verifyImageSHA(ctx, imageInfo)
	}

	return nil
}

func (dm *DockerManager) verifyImageSHA(ctx context.Context, imageInfo schema.ImageInfo) error {
	log.Debug("verifying docker image sha", "image", imageInfo.Name, "expected_sha", imageInfo.SHA)
	inspect, err := dm.cli.ImageInspect(ctx, imageInfo.Name)
	if err != nil {
		return fmt.Errorf("failed to inspect image %s: %v", imageInfo.Name, err)
	}

	for _, digest := range inspect.RepoDigests {
		if strings.Contains(digest, imageInfo.SHA) {
			return nil
		}
	}

	if inspect.ID == imageInfo.SHA {
		log.Debug("docker image sha matched local image id", "image", imageInfo.Name, "image_id", inspect.ID)
		return nil
	}

	return fmt.Errorf("image SHA verification failed for %s: expected %s, got digests %v and ID %s",
		imageInfo.Name, imageInfo.SHA, inspect.RepoDigests, inspect.ID)
}

func (dm *DockerManager) CreateInstance(ctx context.Context, pid string, runtimeSpec schema.RuntimeSpec, runtimeEnv []string) (*schema.InstanceInfo, error) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	log.Info("creating docker runtime instance", "pid", pid, "image", runtimeSpec.Image.Name, "env_count", len(runtimeEnv))
	if err := dm.ensureImageExists(ctx, runtimeSpec.Image); err != nil {
		return nil, fmt.Errorf("failed to ensure image exists: %v", err)
	}

	port, err := dm.portAllocator.Allocate()
	if err != nil {
		return nil, err
	}

	workspace, err := ensureRuntimeWorkspace(pid, runtimeSpec.Sandbox.Workspace)
	if err != nil {
		dm.portAllocator.Release(port)
		return nil, err
	}

	launchRuntimeEnv := append([]string(nil), runtimeEnv...)
	launchRuntimeSpec := runtimeSpec
	launchRuntimeSpec.Sandbox.Workspace = runtimeWorkspaceRootFromPath(workspace)

	runtimeEnv = appendRuntimePersistenceEnv(runtimeEnv, workspace)

	hostConfig := buildDockerHostConfig(port, workspace)

	startCommand, err := buildForegroundRuntimeCommand(runtimeSpec.StartCommand)
	if err != nil {
		dm.portAllocator.Release(port)
		return nil, fmt.Errorf("build docker start command failed: %w", err)
	}

	config, err := buildDockerContainerConfig(runtimeSpec, runtimeEnv, startCommand, workspace)
	if err != nil {
		dm.portAllocator.Release(port)
		return nil, err
	}

	resp, err := dm.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, ContainerNamePrefix+pid)
	if err != nil {
		dm.portAllocator.Release(port)
		return nil, err
	}
	log.Info("docker runtime instance created", "pid", pid, "container_id", resp.ID, "port", port)

	instanceInfo := &schema.InstanceInfo{
		ID:          resp.ID,
		Name:        pid,
		Port:        port,
		Status:      "created",
		CreateAt:    time.Now(),
		Backend:     schema.RuntimeBackendDocker,
		Workspace:   workspace,
		RuntimeSpec: launchRuntimeSpec,
		RuntimeEnv:  launchRuntimeEnv,
	}
	dm.instances[pid] = instanceInfo
	return instanceInfo, nil
}

func (dm *DockerManager) GetInstance(pid string) (*schema.InstanceInfo, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	instance, exists := dm.instances[pid]
	if !exists {
		return nil, fmt.Errorf("instance not found")
	}
	return instance, nil
}

func (dm *DockerManager) RemoveInstance(ctx context.Context, pid string) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	log.Info("removing docker runtime instance", "pid", pid)
	instance, exists := dm.instances[pid]
	if !exists {
		return fmt.Errorf("instance not found: %s", pid)
	}

	if err := dm.cli.ContainerRemove(ctx, instance.ID, container.RemoveOptions{Force: true}); err != nil {
		return err
	}
	log.Info("docker runtime instance removed", "pid", pid, "container_id", instance.ID)
	dm.portAllocator.Release(instance.Port)
	delete(dm.instances, pid)
	return nil
}

func (dm *DockerManager) StartInstance(ctx context.Context, pid string) error {
	log.Info("starting docker runtime instance", "pid", pid)
	instance, err := dm.GetInstance(pid)
	if err != nil {
		return err
	}
	if err := dm.cli.ContainerStart(ctx, instance.ID, container.StartOptions{}); err != nil {
		return err
	}
	instance.Status = "running"
	log.Info("docker runtime instance started", "pid", pid, "container_id", instance.ID, "port", instance.Port)
	return nil
}

func (dm *DockerManager) StopInstance(ctx context.Context, pid string) error {
	log.Info("stopping docker runtime instance", "pid", pid)
	instance, err := dm.GetInstance(pid)
	if err != nil {
		return err
	}
	timeoutSeconds := 10
	if err := dm.cli.ContainerStop(ctx, instance.ID, container.StopOptions{Timeout: &timeoutSeconds}); err != nil {
		return err
	}
	instance.Status = "stopped"
	log.Info("docker runtime instance stopped", "pid", pid, "container_id", instance.ID)
	return nil
}

func buildDockerContainerConfig(runtimeSpec schema.RuntimeSpec, runtimeEnv, startCommand []string, workspace string) (*container.Config, error) {
	if len(startCommand) == 0 {
		return nil, fmt.Errorf("docker start command is empty")
	}

	return &container.Config{
		Image:      runtimeSpec.Image.Name,
		Entrypoint: []string{startCommand[0]},
		ExposedPorts: nat.PortSet{
			nat.Port(schema.ExprotPort): struct{}{},
		},
		Cmd:        startCommand[1:],
		Env:        runtimeEnv,
		WorkingDir: containerHome,
	}, nil
}

func buildDockerHostConfig(port int, workspace string) *container.HostConfig {
	pidsLimit := int64(256)
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port(schema.ExprotPort): []nat.PortBinding{{
				HostIP:   schema.AllowHost,
				HostPort: fmt.Sprintf("%d", port),
			}},
		},
		ReadonlyRootfs: true,
		SecurityOpt:    []string{"no-new-privileges:true"},
		CapDrop:        []string{"ALL"},
		MaskedPaths: []string{
			"/proc/acpi",
			"/proc/kcore",
			"/proc/keys",
			"/proc/latency_stats",
			"/proc/timer_list",
			"/proc/timer_stats",
			"/proc/sched_debug",
			"/proc/scsi",
			"/sys/firmware",
		},
		ReadonlyPaths: []string{
			"/etc/hosts",
			"/etc/hostname",
			"/etc/resolv.conf",
			"/proc/asound",
			"/proc/bus",
			"/proc/fs",
			"/proc/irq",
			"/proc/sys",
			"/proc/sysrq-trigger",
		},
		Resources: container.Resources{
			Memory:     int64(schema.MaxMem),
			MemorySwap: -1,
			PidsLimit:  &pidsLimit,
			CPUPeriod:  100000,
			CPUQuota:   200000,
			CPUShares:  1024,
		},
	}
	if schema.UseMount {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   schema.MountSource,
			Target:   schema.MountTarget,
			ReadOnly: true,
		})
	}
	// NOTE (spec §4.2): the Docker Sandbox backend enforces same-path mounts at
	// the platform level; the fixed /home/hymx target applies to the Docker
	// backend. Sandbox-backend reconciliation is tracked separately.
	hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
		Type:   mount.TypeBind,
		Source: workspace,
		Target: containerHome,
	})
	return hostConfig
}

func (dm *DockerManager) ExecInstance(context.Context, string, []string, string) (string, error) {
	return "", schema.ErrNotSupported
}

func (dm *DockerManager) Checkpoint(_ context.Context, pid, checkpointName string) (string, error) {
	instance, err := dm.GetInstance(pid)
	if err != nil {
		return "", err
	}
	log.Info("checkpointing docker runtime workspace", "pid", pid, "checkpoint_name", checkpointName, "workspace", instance.Workspace)
	return checkpointRuntimeWorkspace(instance.Workspace, checkpointName)
}

func (dm *DockerManager) Restore(_ context.Context, pid, checkpointName, snapshot string) error {
	instance, err := dm.GetInstance(pid)
	if err != nil {
		return err
	}
	log.Info("restoring docker runtime workspace", "pid", pid, "checkpoint_name", checkpointName, "workspace", instance.Workspace)
	return restoreRuntimeWorkspace(instance.Workspace, checkpointName, snapshot)
}
