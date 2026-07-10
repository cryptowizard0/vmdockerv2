package runtimemanager

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
)

func waitForEnter(step string) {
	fmt.Printf("=== Step Completed: %s ===\n\n", step)
}

func requireDockerIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("VMDOCKER_INTEGRATION") != "1" {
		t.Skip("set VMDOCKER_INTEGRATION=1 to run docker integration tests")
	}
}

func resetRuntimeManagerForTest() {
	runtimeOnce = map[string]*sync.Once{
		schema.RuntimeBackendDocker:  {},
		schema.RuntimeBackendSandbox: {},
	}
	runtimeInstance = map[string]IRuntimeManager{}
	runtimeInitErr = map[string]error{}
}

func expectedDefaultRuntimeBackend() string {
	switch runtime.GOOS {
	case "darwin", "windows":
		return schema.RuntimeBackendSandbox
	default:
		return schema.RuntimeBackendDocker
	}
}

func TestDockerManager(t *testing.T) {
	requireDockerIntegration(t)
	resetRuntimeManagerForTest()
	ctx := context.Background()

	dm, err := GetRuntimeManager(schema.RuntimeBackendDocker)
	assert.NoError(t, err)
	assert.NotNil(t, dm)
	waitForEnter("Initialize DockerManager")

	t.Run("CreateContainer", func(t *testing.T) {
		requireDockerIntegration(t)
		imageInfo := schema.ImageInfo{
			Name: os.Getenv("VMDOCKER_TEST_IMAGE"),
			SHA:  os.Getenv("VMDOCKER_TEST_IMAGE_SHA"),
		}
		if imageInfo.Name == "" {
			imageInfo.Name = "chriswebber/docker-golua:v0.0.2"
		}
		instanceInfo, err := dm.CreateInstance(ctx, "test-container", schema.RuntimeSpec{
			Backend: schema.RuntimeBackendDocker,
			Image:   imageInfo,
		}, nil)
		if !assert.NoError(t, err) {
			return
		}
		if !assert.NotNil(t, instanceInfo) {
			return
		}
		assert.Equal(t, "test-container", instanceInfo.Name)
		assert.Equal(t, "created", instanceInfo.Status)
		waitForEnter("Create Container")
	})

	t.Run("GetContainer", func(t *testing.T) {
		requireDockerIntegration(t)
		instanceInfo, err := dm.GetInstance("test-container")
		if !assert.NoError(t, err) {
			return
		}
		if !assert.NotNil(t, instanceInfo) {
			return
		}
		assert.Equal(t, "test-container", instanceInfo.Name)
		waitForEnter("Get Container Info")
	})

	t.Run("StartContainer", func(t *testing.T) {
		requireDockerIntegration(t)
		err := dm.StartInstance(ctx, "test-container")
		if !assert.NoError(t, err) {
			return
		}

		instanceInfo, err := dm.GetInstance("test-container")
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, "running", instanceInfo.Status)
		waitForEnter("Start Container")
		time.Sleep(2 * time.Second)
	})

	t.Run("StopContainer", func(t *testing.T) {
		requireDockerIntegration(t)
		err := dm.StopInstance(ctx, "test-container")
		if !assert.NoError(t, err) {
			return
		}

		instanceInfo, err := dm.GetInstance("test-container")
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, "stopped", instanceInfo.Status)
		waitForEnter("Stop Container")
	})

	t.Run("RemoveContainer", func(t *testing.T) {
		requireDockerIntegration(t)
		err := dm.RemoveInstance(ctx, "test-container")
		if !assert.NoError(t, err) {
			return
		}

		_, err = dm.GetInstance("test-container")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "instance not found")
		waitForEnter("Remove Container")
	})

	t.Run("ErrorCases", func(t *testing.T) {
		requireDockerIntegration(t)
		_, err := dm.GetInstance("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "instance not found")

		err = dm.StopInstance(ctx, "non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "instance not found")

		err = dm.RemoveInstance(ctx, "non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "instance not found")
		waitForEnter("Error Test Cases")
	})

	t.Run("CheckpointAndRestoreWorkspace", func(t *testing.T) {
		requireDockerIntegration(t)
		imageInfo := schema.ImageInfo{
			Name: os.Getenv("VMDOCKER_TEST_IMAGE"),
			SHA:  os.Getenv("VMDOCKER_TEST_IMAGE_SHA"),
		}
		if imageInfo.Name == "" {
			imageInfo.Name = "chriswebber/docker-golua:v0.0.2"
		}
		_, err := dm.CreateInstance(ctx, "checkpoint-test", schema.RuntimeSpec{
			Backend: schema.RuntimeBackendDocker,
			Image:   imageInfo,
		}, nil)
		if !assert.NoError(t, err) {
			return
		}
		err = dm.StartInstance(ctx, "checkpoint-test")
		if !assert.NoError(t, err) {
			return
		}
		err = dm.StopInstance(ctx, "checkpoint-test")
		assert.NoError(t, err)

		snapshot, err := dm.Checkpoint(ctx, "checkpoint-test", "test-checkpoint")
		if assert.NoError(t, err) {
			assert.NotEmpty(t, snapshot)
		}
		err = dm.Restore(ctx, "checkpoint-test", "test-checkpoint", snapshot)
		assert.NoError(t, err)

		err = dm.RemoveInstance(ctx, "checkpoint-test")
		assert.NoError(t, err)
	})
}

func TestGetRuntimeManager(t *testing.T) {
	resetRuntimeManagerForTest()
	defaultBackend := expectedDefaultRuntimeBackend()

	defaultManager, err := GetRuntimeManager("")
	assert.NoError(t, err)
	assert.NotNil(t, defaultManager)
	switch defaultBackend {
	case schema.RuntimeBackendSandbox:
		_, ok := defaultManager.(*SandboxManager)
		assert.True(t, ok)
	case schema.RuntimeBackendDocker:
		_, ok := defaultManager.(*DockerManager)
		assert.True(t, ok)
	}

	defaultManagerAgain, err := GetRuntimeManager("")
	assert.NoError(t, err)
	assert.Same(t, defaultManager, defaultManagerAgain)

	if runtime.GOOS != "linux" {
		otherBackend := schema.RuntimeBackendSandbox
		if defaultBackend == schema.RuntimeBackendSandbox {
			otherBackend = schema.RuntimeBackendDocker
		}
		otherManager, err := GetRuntimeManager(otherBackend)
		assert.NoError(t, err)
		assert.NotNil(t, otherManager)
		if otherBackend != defaultBackend {
			assert.NotSame(t, defaultManager, otherManager)
		}
	}

	_, err = GetRuntimeManager("unknown")
	assert.ErrorIs(t, err, schema.ErrNotSupported)
}

func TestGetRuntimeManagerLinuxRejectsSandbox(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only sandbox protection")
	}

	resetRuntimeManagerForTest()
	_, err := GetRuntimeManager(schema.RuntimeBackendSandbox)
	assert.EqualError(t, err, "runtime backend sandbox is not supported on linux")
}

func TestBuildForegroundRuntimeCommandUsesConfiguredRuntimeCommand(t *testing.T) {
	args, err := buildForegroundRuntimeCommand("/app/custom-entrypoint --serve")
	assert.NoError(t, err)
	assert.Equal(t, []string{"/app/custom-entrypoint", "--serve"}, args)

	args, err = buildForegroundRuntimeCommand(`/app/custom-entrypoint --label "hello world"`)
	assert.NoError(t, err)
	assert.Equal(t, []string{"/app/custom-entrypoint", "--label", "hello world"}, args)

	args, err = buildForegroundRuntimeCommand("")
	assert.NoError(t, err)
	assert.Equal(t, []string{defaultRuntimeStartCommand}, args)
}

func TestBuildForegroundRuntimeCommandRejectsInvalidQuotedCommand(t *testing.T) {
	_, err := buildForegroundRuntimeCommand(`"/app/custom-entrypoint`)
	assert.EqualError(t, err, "unterminated quoted string in start command")
}

func TestBuildDockerContainerConfigEnforcesRuntimeUserAndParsedCommand(t *testing.T) {
	startCommand, err := buildForegroundRuntimeCommand(`/app/start-runtime --label "hello world"`)
	assert.NoError(t, err)

	runtimeEnv := appendRuntimePersistenceEnv([]string{"OPENCLAW_GATEWAY_TOKEN=test-token"}, "/tmp/runtime-workspace/pid-1")
	config, err := buildDockerContainerConfig(schema.RuntimeSpec{
		Image: schema.ImageInfo{Name: "example/runtime:test"},
	}, runtimeEnv, startCommand, "/tmp/runtime-workspace/pid-1")
	assert.NoError(t, err)
	if assert.NotNil(t, config) {
		assert.Equal(t, "example/runtime:test", config.Image)
		assert.Equal(t, schema.RuntimeUser, config.User)
		assert.Equal(t, []string{"/app/start-runtime"}, []string(config.Entrypoint))
		assert.Equal(t, []string{"--label", "hello world"}, []string(config.Cmd))
		assert.Equal(t, containerHome, config.WorkingDir)
		assert.Contains(t, config.Env, "OPENCLAW_GATEWAY_TOKEN=test-token")
		assert.Contains(t, config.Env, "OPENCLAW_STATE_DIR=/home/hymx/.openclaw")
		assert.Contains(t, config.Env, "OPENCLAW_HOME=/home/hymx")
		assert.Contains(t, config.Env, "TMPDIR=/home/hymx/.tmp")
	}
}

func TestBuildDockerHostConfigIncludesWorkspaceMount(t *testing.T) {
	hostConfig := buildDockerHostConfig(18080, "/tmp/runtime-workspace/pid-1", "/tmp/runtime-workspace/pid-1-tmp")
	if assert.NotNil(t, hostConfig) {
		assert.True(t, hostConfig.ReadonlyRootfs)
		bindings := hostConfig.PortBindings[nat.Port(schema.ExprotPort)]
		if assert.Len(t, bindings, 1) {
			assert.Equal(t, schema.AllowHost, bindings[0].HostIP)
			assert.Equal(t, "18080", bindings[0].HostPort)
		}
		assert.Contains(t, hostConfig.Mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: "/tmp/runtime-workspace/pid-1",
			Target: containerHome,
		})
		assert.Contains(t, hostConfig.Mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: "/tmp/runtime-workspace/pid-1-tmp",
			Target: containerTmp,
		})
	}
}

func TestEnsureImageExists(t *testing.T) {
	requireDockerIntegration(t)
	resetRuntimeManagerForTest()
	ctx := context.Background()

	dm, err := GetRuntimeManager(schema.RuntimeBackendDocker)
	assert.NoError(t, err)
	assert.NotNil(t, dm)

	dockerManager := dm.(*DockerManager)

	t.Run("EnsureImageExistsWithValidImage", func(t *testing.T) {
		requireDockerIntegration(t)
		imageInfo := schema.ImageInfo{
			Name: os.Getenv("VMDOCKER_TEST_IMAGE"),
			SHA:  os.Getenv("VMDOCKER_TEST_IMAGE_SHA"),
		}
		if imageInfo.Name == "" {
			imageInfo.Name = "chriswebber/docker-golua:v0.0.2"
		}

		err := dockerManager.ensureImageExists(ctx, imageInfo)
		assert.NoError(t, err)
		waitForEnter("Ensure Image Exists - Valid Image")
	})

	t.Run("EnsureImageExistsWithInvalidSHA", func(t *testing.T) {
		requireDockerIntegration(t)
		imageInfo := schema.ImageInfo{
			Name: "alpine:latest",
			SHA:  "sha256:invalid-sha-that-will-not-match",
		}

		err := dockerManager.ensureImageExists(ctx, imageInfo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "image SHA verification failed")
		waitForEnter("Ensure Image Exists - Invalid SHA")
	})
}

func TestVerifyImageSHA(t *testing.T) {
	requireDockerIntegration(t)
	resetRuntimeManagerForTest()
	ctx := context.Background()

	dm, err := GetRuntimeManager(schema.RuntimeBackendDocker)
	assert.NoError(t, err)
	assert.NotNil(t, dm)

	dockerManager := dm.(*DockerManager)

	t.Run("VerifyImageSHAWithExistingImage", func(t *testing.T) {
		requireDockerIntegration(t)
		imageInfo := schema.ImageInfo{
			Name: os.Getenv("VMDOCKER_TEST_IMAGE"),
			SHA:  os.Getenv("VMDOCKER_TEST_IMAGE_SHA"),
		}
		if imageInfo.Name == "" {
			imageInfo.Name = "chriswebber/docker-golua:v0.0.2"
		}
		err := dockerManager.ensureImageExists(ctx, imageInfo)
		assert.NoError(t, err)

		imageInfo.SHA = "sha256:dummy-sha-that-will-not-match"
		err = dockerManager.verifyImageSHA(ctx, imageInfo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "image SHA verification failed")
		waitForEnter("Verify Image SHA - Existing Image with Wrong SHA")
	})

	t.Run("VerifyImageSHAWithNonExistentImage", func(t *testing.T) {
		requireDockerIntegration(t)
		imageInfo := schema.ImageInfo{
			Name: "nonexistent/invalid-image:nonexistent-tag",
			SHA:  "sha256:dummy-sha",
		}

		err := dockerManager.verifyImageSHA(ctx, imageInfo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to inspect image")
		waitForEnter("Verify Image SHA - Non-existent Image")
	})
}

func TestImageInspectID(t *testing.T) {
	requireDockerIntegration(t)
	resetRuntimeManagerForTest()
	ctx := context.Background()

	dm, err := GetRuntimeManager(schema.RuntimeBackendDocker)
	assert.NoError(t, err)
	assert.NotNil(t, dm)

	dockerManager := dm.(*DockerManager)

	t.Run("GetAlpineImageID", func(t *testing.T) {
		requireDockerIntegration(t)
		imageInfo := schema.ImageInfo{
			Name: "chriswebber/docker-golua:latest",
			SHA:  "b2e104cdcb5c09a8f213aefcadd451cbabfda1f16c91107e84eef051f807d45b",
		}

		inspect, err := dockerManager.cli.ImageInspect(ctx, imageInfo.Name)
		assert.NoError(t, err)
		t.Logf("Image Name: %s", imageInfo.Name)
		t.Logf("Image ID: %s", inspect.ID)
		t.Logf("Image Size: %d bytes", inspect.Size)
		t.Logf("Image Created: %s", inspect.Created)
		t.Logf("Image Architecture: %s", inspect.Architecture)
		t.Logf("Image OS: %s", inspect.Os)
		assert.NotEmpty(t, inspect.ID)
		assert.True(t, strings.HasPrefix(inspect.ID, "sha256:"))
		waitForEnter("Alpine Image Inspect ID")
	})
}
