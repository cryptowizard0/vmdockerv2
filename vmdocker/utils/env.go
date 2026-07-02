package utils

import (
	"sort"
	"strings"

	goarSchema "github.com/permadao/goar/schema"
)

// ContainerEnvTagPrefix defines the tag naming convention for passing container environment variables.
//
// Format:
//   - Tag Name:  "Container-Env-" + <ENV_KEY>
//   - Tag Value: <ENV_VALUE>
//
// Example:
//   - Container-Env-RUNTIME_TYPE = openclaw
//   - Container-Env-OPENCLAW_GATEWAY_URL = http://127.0.0.1:18789
//   - Container-Env-OPENCLAW_GATEWAY_TOKEN = openclaw-test-token
//
// Parsing rule:
//   - For each tag with Name prefix "Container-Env-", ENV_KEY = strings.TrimPrefix(Name, prefix)
//   - Emit env entry as ENV_KEY + "=" + Value
const ContainerEnvTagPrefix = "Container-Env-"

// ContainerEnvFromTags extracts container environment variables from tags using ContainerEnvTagPrefix.
//
// It returns a deterministic (sorted) env list in the form "KEY=VALUE".
// If the same KEY appears multiple times, the last occurrence wins.
func ContainerEnvFromTags(tags []goarSchema.Tag) []string {
	envMap := make(map[string]string)
	for _, tag := range tags {
		if !strings.HasPrefix(tag.Name, ContainerEnvTagPrefix) {
			continue
		}
		key := strings.TrimPrefix(tag.Name, ContainerEnvTagPrefix)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		envMap[key] = tag.Value
	}

	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+envMap[k])
	}
	return env
}
