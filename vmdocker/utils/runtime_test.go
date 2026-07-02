package utils

import (
	"testing"

	vmdockerSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	goarSchema "github.com/permadao/goar/schema"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSpecFromTags_DoesNotSetBackend(t *testing.T) {
	spec, err := RuntimeSpecFromTags(vmdockerSchema.ModuleFormat, []goarSchema.Tag{
		{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw-sandbox:v0.0.1"},
		{Name: vmdockerSchema.ImageIDTag, Value: "sha256:sandbox-template"},
		{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
		{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveDockerSaveGZ},
		{Name: vmdockerSchema.StartCommandTag, Value: "/app/start-runtime.sh"},
	})
	require.NoError(t, err)
	require.Equal(t, "", spec.Backend)
	require.Equal(t, "/app/start-runtime.sh", spec.StartCommand)
	require.Equal(t, "shell", spec.Sandbox.Agent)
	require.Equal(t, "", spec.Sandbox.Workspace)
	require.Equal(t, "", spec.Sandbox.Network)
	require.Equal(t, vmdockerSchema.ImageSourceModuleData, spec.Image.Source)
	require.Equal(t, vmdockerSchema.ImageArchiveDockerSaveGZ, spec.Image.ArchiveFormat)
}

func TestRuntimeSpecFromTags_AcceptsContainerTarArchive(t *testing.T) {
	tags := []goarSchema.Tag{
		{Name: vmdockerSchema.ImageNameTag, Value: "vmdocker-module:abc"},
		{Name: vmdockerSchema.ImageIDTag, Value: "sha256:deadbeef"},
		{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
		{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveContainerTarGZ},
	}
	spec, err := RuntimeSpecFromTags(vmdockerSchema.ModuleFormat, tags)
	if err != nil {
		t.Fatalf("RuntimeSpecFromTags(container-tar) error: %v", err)
	}
	if spec.Image.Name != "vmdocker-module:abc" {
		t.Fatalf("Image.Name = %q", spec.Image.Name)
	}
	if spec.Image.ArchiveFormat != vmdockerSchema.ImageArchiveContainerTarGZ {
		t.Fatalf("ArchiveFormat = %q", spec.Image.ArchiveFormat)
	}
}

func TestRuntimeSpecFromTags_RejectsUnknownFormat(t *testing.T) {
	if _, err := RuntimeSpecFromTags("bogus.format", nil); err == nil {
		t.Fatal("expected error for unknown module format")
	}
}

func TestRuntimeSpecFromTags_IgnoresRuntimeBackendTag(t *testing.T) {
	spec, err := RuntimeSpecFromTags(vmdockerSchema.ModuleFormat, []goarSchema.Tag{
		{Name: vmdockerSchema.RuntimeBackendTag, Value: vmdockerSchema.RuntimeBackendDocker},
		{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw:v0.0.4"},
		{Name: vmdockerSchema.ImageIDTag, Value: "sha256:docker-template"},
		{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
		{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveDockerSaveGZ},
	})
	require.NoError(t, err)
	require.Equal(t, "", spec.Backend)
	require.Equal(t, "chriswebber/docker-openclaw:v0.0.4", spec.Image.Name)
}

func TestRuntimeSpecFromTags_IgnoresSandboxWorkspaceTag(t *testing.T) {
	spec, err := RuntimeSpecFromTags(vmdockerSchema.ModuleFormat, []goarSchema.Tag{
		{Name: "Sandbox-Workspace", Value: "/tmp/override"},
		{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw:v0.0.4"},
		{Name: vmdockerSchema.ImageIDTag, Value: "sha256:docker-template"},
		{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
		{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveDockerSaveGZ},
	})
	require.NoError(t, err)
	require.Equal(t, "", spec.Sandbox.Workspace)
}

func TestRuntimeSpecFromTags_RejectsLegacyBuildModules(t *testing.T) {
	_, err := RuntimeSpecFromTags(vmdockerSchema.ModuleFormat, []goarSchema.Tag{
		{Name: "Build-Type", Value: "dockerfile"},
		{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw-sandbox:v0.0.1"},
		{Name: vmdockerSchema.ImageIDTag, Value: "sha256:sandbox-template"},
	})
	require.EqualError(t, err, "Build-Type modules are no longer supported")
}

func TestRuntimeSpecFromTags_RequiresModuleDataSource(t *testing.T) {
	_, err := RuntimeSpecFromTags(vmdockerSchema.ModuleFormat, []goarSchema.Tag{
		{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw-sandbox:v0.0.1"},
		{Name: vmdockerSchema.ImageIDTag, Value: "sha256:sandbox-template"},
	})
	require.EqualError(t, err, "Image-Source must be module-data")
}

func TestRuntimeSpecFromModuleAndSpawnTags_SpawnOverridesBackend(t *testing.T) {
	spec, err := RuntimeSpecFromModuleAndSpawnTags(
		vmdockerSchema.ModuleFormat,
		[]goarSchema.Tag{
			{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw-sandbox:v0.0.1"},
			{Name: vmdockerSchema.ImageIDTag, Value: "sha256:sandbox-template"},
			{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
			{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveDockerSaveGZ},
		},
		[]goarSchema.Tag{
			{Name: vmdockerSchema.RuntimeBackendTag, Value: vmdockerSchema.RuntimeBackendDocker},
		},
	)
	require.NoError(t, err)
	require.Equal(t, vmdockerSchema.RuntimeBackendDocker, spec.Backend)
	require.Equal(t, "chriswebber/docker-openclaw-sandbox:v0.0.1", spec.Image.Name)
}

func TestRuntimeSpecFromModuleAndSpawnTags_SpawnOverridesStartCommand(t *testing.T) {
	spec, err := RuntimeSpecFromModuleAndSpawnTags(
		vmdockerSchema.ModuleFormat,
		[]goarSchema.Tag{
			{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw-sandbox:v0.0.1"},
			{Name: vmdockerSchema.ImageIDTag, Value: "sha256:sandbox-template"},
			{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
			{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveDockerSaveGZ},
			{Name: vmdockerSchema.StartCommandTag, Value: "/module/start.sh"},
		},
		[]goarSchema.Tag{
			{Name: vmdockerSchema.StartCommandTag, Value: "/spawn/start.sh --flag"},
		},
	)
	require.NoError(t, err)
	require.Equal(t, "/spawn/start.sh --flag", spec.StartCommand)
}

func TestRuntimeSpecFromModuleAndSpawnTags_NoSpawnBackendLeavesBackendEmpty(t *testing.T) {
	spec, err := RuntimeSpecFromModuleAndSpawnTags(
		vmdockerSchema.ModuleFormat,
		[]goarSchema.Tag{
			{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw-sandbox:v0.0.1"},
			{Name: vmdockerSchema.ImageIDTag, Value: "sha256:sandbox-template"},
			{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
			{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveDockerSaveGZ},
		},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, "", spec.Backend)
}

func TestRuntimeSpecFromModuleAndSpawnTags_InvalidSpawnBackend(t *testing.T) {
	_, err := RuntimeSpecFromModuleAndSpawnTags(
		vmdockerSchema.ModuleFormat,
		[]goarSchema.Tag{
			{Name: vmdockerSchema.ImageNameTag, Value: "chriswebber/docker-openclaw-sandbox:v0.0.1"},
			{Name: vmdockerSchema.ImageIDTag, Value: "sha256:sandbox-template"},
			{Name: vmdockerSchema.ImageSourceTag, Value: vmdockerSchema.ImageSourceModuleData},
			{Name: vmdockerSchema.ImageArchiveTag, Value: vmdockerSchema.ImageArchiveDockerSaveGZ},
		},
		[]goarSchema.Tag{
			{Name: vmdockerSchema.RuntimeBackendTag, Value: "invalid"},
		},
	)
	require.EqualError(t, err, "unsupported runtime backend: invalid")
}
