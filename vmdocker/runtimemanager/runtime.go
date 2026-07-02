package runtimemanager

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	"github.com/hymatrix/hymx/common"
)

var log = common.NewLog("vmdocker-runtime")

type IRuntimeManager interface {
	CreateInstance(ctx context.Context, pid string, runtimeSpec schema.RuntimeSpec, runtimeEnv []string) (*schema.InstanceInfo, error)
	GetInstance(pid string) (*schema.InstanceInfo, error)
	RemoveInstance(ctx context.Context, pid string) error
	StartInstance(ctx context.Context, pid string) error
	StopInstance(ctx context.Context, pid string) error
	ExecInstance(ctx context.Context, pid string, env []string, command string) (string, error)
	Checkpoint(ctx context.Context, pid, checkpointName string) (string, error)
	Restore(ctx context.Context, pid, checkpointName, snapshot string) error
}

var (
	runtimeOnce = map[string]*sync.Once{
		schema.RuntimeBackendDocker:  {},
		schema.RuntimeBackendSandbox: {},
	}
	runtimeInstance = map[string]IRuntimeManager{}
	runtimeInitErr  = map[string]error{}
)

func GetRuntimeManager(backend string) (IRuntimeManager, error) {
	backend = normalizeRuntimeBackend(backend)
	if backend == "" {
		return nil, schema.ErrNotSupported
	}
	if runtime.GOOS == "linux" && backend == schema.RuntimeBackendSandbox {
		return nil, fmt.Errorf("runtime backend %s is not supported on linux", backend)
	}

	once, ok := runtimeOnce[backend]
	if !ok {
		return nil, schema.ErrNotSupported
	}

	once.Do(func() {
		switch backend {
		case schema.RuntimeBackendSandbox:
			runtimeInstance[backend], runtimeInitErr[backend] = newSandboxManager()
		case schema.RuntimeBackendDocker:
			runtimeInstance[backend], runtimeInitErr[backend] = newDockerManager()
		}
	})
	if err := runtimeInitErr[backend]; err != nil {
		return nil, err
	}

	instance := runtimeInstance[backend]
	if instance == nil {
		return nil, fmt.Errorf("runtime manager initialization failed")
	}
	return instance, nil
}

func normalizeRuntimeBackend(backend string) string {
	if backend == "" {
		switch runtime.GOOS {
		case "darwin", "windows":
			return schema.RuntimeBackendSandbox
		default:
			return schema.RuntimeBackendDocker
		}
	}

	switch backend {
	case schema.RuntimeBackendSandbox, schema.RuntimeBackendDocker:
		return backend
	default:
		return ""
	}
}
