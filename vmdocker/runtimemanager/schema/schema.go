package schema

import (
	"errors"
	"time"
)

const (
	ModuleFormat = "hymx.vmdockerv2.v0.0.1"
)

var ErrNotSupported = errors.New("not supported in stateless runtime mode")

const (
	RuntimeBackendDocker  = "docker"
	RuntimeBackendSandbox = "sandbox"
)

const (
	RuntimeBackendTag = "Runtime-Backend"
	StartCommandTag   = "Start-Command"
	SandboxAgentTag   = "Sandbox-Agent"
	SandboxNetworkTag = "Sandbox-Network"
	SandboxNameTag    = "Sandbox-Name"
	SandboxCommandTag = "Sandbox-Command"
)

const (
	ImageNameTag    = "Image-Name"
	ImageIDTag      = "Image-ID"
	ImageSourceTag  = "Image-Source"
	ImageArchiveTag = "Image-Archive-Format"
)

const (
	ImageSourceModuleData      = "module-data"
	ImageArchiveDockerSaveGZ   = "docker-save+gzip"
	ImageArchiveContainerTarGZ = "container-tar+image.tar.gz"
)

// ImageInfo contains image name and verification information
type ImageInfo struct {
	Name          string // Docker image name
	SHA           string // Image SHA256 digest for verification
	Source        string // how the image should be sourced
	ArchiveFormat string // payload encoding for module-backed images
}

type SandboxSpec struct {
	Agent     string
	Workspace string
	Network   string
	Name      string
	Command   string
}

type RuntimeSpec struct {
	Backend      string
	StartCommand string
	Image        ImageInfo
	Sandbox      SandboxSpec
}

type InstanceInfo struct {
	ID          string
	Name        string
	Port        int
	Status      string
	CreateAt    time.Time
	Backend     string
	Agent       string
	Workspace   string
	RuntimeSpec RuntimeSpec
	RuntimeEnv  []string
}
