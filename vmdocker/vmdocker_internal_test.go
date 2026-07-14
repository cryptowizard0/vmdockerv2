package vmdocker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
)

func TestBuildSandboxCurlCommandFromFile(t *testing.T) {
	command := buildSandboxCurlCommandFromFile("/vmm/spawn", "/tmp/request.json")
	if !strings.Contains(command, "--data-binary @'/tmp/request.json'") {
		t.Fatalf("expected curl command to read payload from file, got: %s", command)
	}
	if strings.Contains(command, "--data-raw") {
		t.Fatalf("expected file-based curl command to avoid --data-raw, got: %s", command)
	}
}

func TestWorkspaceCheckpointJSONRoundTrip(t *testing.T) {
	raw, err := json.Marshal(workspaceCheckpoint{
		Format:                  workspaceCheckpointFormatV1,
		WorkspaceArchive:        "archive",
		WorkspaceCheckpointName: "workspace",
		RuntimeState:            `{"format":"openclaw.runtime.v1","sessionId":"sess-1"}`,
		Backend:                 "docker",
		RuntimeSpec: runtimeSchema.RuntimeSpec{
			Backend: "docker",
			Image: runtimeSchema.ImageInfo{
				Name: "image:test",
				SHA:  "sha256:test",
			},
		},
		RuntimeEnv: []string{"RUNTIME_TYPE=claude"},
	})
	if err != nil {
		t.Fatalf("marshal checkpoint failed: %v", err)
	}

	var decoded workspaceCheckpoint
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal checkpoint failed: %v", err)
	}
	if decoded.Format != workspaceCheckpointFormatV1 {
		t.Fatalf("format = %q, want %q", decoded.Format, workspaceCheckpointFormatV1)
	}
	if decoded.Backend != "docker" {
		t.Fatalf("backend = %q, want %q", decoded.Backend, "docker")
	}
	if decoded.WorkspaceCheckpointName != "workspace" {
		t.Fatalf("workspace checkpoint name = %q, want %q", decoded.WorkspaceCheckpointName, "workspace")
	}
	if decoded.RuntimeSpec.Image.Name != "image:test" {
		t.Fatalf("runtime spec image = %q, want %q", decoded.RuntimeSpec.Image.Name, "image:test")
	}
	if len(decoded.RuntimeEnv) != 1 || decoded.RuntimeEnv[0] != "RUNTIME_TYPE=claude" {
		t.Fatalf("runtime env = %v, want %v", decoded.RuntimeEnv, []string{"RUNTIME_TYPE=claude"})
	}
	if !strings.Contains(decoded.RuntimeState, `"sessionId":"sess-1"`) {
		t.Fatalf("runtime state missing session id: %s", decoded.RuntimeState)
	}
}

func TestHandleRestoreFailureRestoresPreviousRuntimeWithoutWorkspaceRollback(t *testing.T) {
	restored := false
	handleRestoreFailure(nil, func() error {
		restored = true
		return nil
	}, true, &restored)

	if !restored {
		t.Fatalf("expected previous runtime restore to run")
	}
}

func TestHandleRestoreFailureDoesNotMarkRestoredOnError(t *testing.T) {
	restored := false
	handleRestoreFailure(nil, func() error {
		return errors.New("boom")
	}, true, &restored)

	if restored {
		t.Fatalf("expected previous runtime restore flag to remain false on error")
	}
}
