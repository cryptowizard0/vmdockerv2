package utils

import (
	"testing"

	goarSchema "github.com/permadao/goar/schema"
	"github.com/stretchr/testify/assert"
)

func TestContainerEnvFromTags(t *testing.T) {
	tags := []goarSchema.Tag{
		{Name: "Foo", Value: "bar"},
		{Name: ContainerEnvTagPrefix + "RUNTIME_TYPE", Value: "openclaw"},
		{Name: ContainerEnvTagPrefix + "OPENCLAW_GATEWAY_URL", Value: "http://127.0.0.1:18789"},
		{Name: ContainerEnvTagPrefix + "RUNTIME_TYPE", Value: "evm"},
		{Name: ContainerEnvTagPrefix, Value: "should-be-ignored"},
		{Name: ContainerEnvTagPrefix + "  ", Value: "should-be-ignored"},
	}

	env := ContainerEnvFromTags(tags)
	assert.Equal(t, []string{
		"OPENCLAW_GATEWAY_URL=http://127.0.0.1:18789",
		"RUNTIME_TYPE=evm",
	}, env)
}

