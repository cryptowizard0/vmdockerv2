package main

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadNodeConfigReadsAdminPort(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("adminPort", "127.0.0.1:8081")
	viper.Set("keyfilePath", "./test_keyfile.json")

	_, adminPort, _, _, _, _, _, _, _, err := LoadNodeConfig()

	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8081", adminPort)
}
