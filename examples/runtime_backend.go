package main

import (
	"github.com/cryptowizard0/vmdockerv2/vmdocker/utils"
	goarSchema "github.com/permadao/goar/schema"
)

func runtimeBackendTags(runtimeBackend string) []goarSchema.Tag {
	if runtimeBackend == "" {
		return nil
	}
	return []goarSchema.Tag{
		{Name: "Runtime-Backend", Value: runtimeBackend},
	}
}

// runtimeTypeTags passes RUNTIME_TYPE into the container as a spawn tag. It is
// no longer baked into the image (FROM is a raw image name now), so the adapter
// gets its runtime/health-gate from this env; empty -> adapter defaults to test.
func runtimeTypeTags(runtimeType string) []goarSchema.Tag {
	if runtimeType == "" {
		return nil
	}
	return []goarSchema.Tag{
		{Name: utils.ContainerEnvTagPrefix + "RUNTIME_TYPE", Value: runtimeType},
	}
}
