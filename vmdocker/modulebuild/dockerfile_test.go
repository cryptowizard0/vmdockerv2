package modulebuild

import (
	"strings"
	"testing"
)

func TestGenerateDockerfile(t *testing.T) {
	p := Profile{
		Dockerfile: DockerfileSection{
			From:    "docker/sandbox-templates:shell",
			Bin:     "bin",
			Tools:   []string{"curl", "jq"},
			Run:     []string{"pip install --no-cache-dir foo"},
			Startup: "startup.sh",
		},
	}
	out, err := GenerateDockerfile(DockerfileInput{
		Profile:     p,
		AgentBinSrc: "platform/vmdocker-agent",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile error: %v", err)
	}
	mustContain := []string{
		"FROM docker/sandbox-templates:shell",
		"COPY platform/vmdocker-agent /usr/local/bin/vmdocker-agent",
		"COPY bin/ /usr/local/bin/",
		"COPY startup.sh /usr/local/lib/vmdocker-agent/user-startup.sh",
		"COPY profile.toml /home/hymx/profile.toml",
		"RUN pip install --no-cache-dir foo",
		"useradd",
		"ENV HOME=/home/hymx",
		"USER hymx",
		"WORKDIR /home/hymx",
		`ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`,
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("Dockerfile missing %q\n---\n%s", want, out)
		}
	}
	// RUNTIME_TYPE is no longer a build-time concern; it must not be baked.
	if strings.Contains(out, "RUNTIME_TYPE") {
		t.Errorf("Dockerfile must not bake RUNTIME_TYPE:\n%s", out)
	}
	if strings.Contains(out, "start-vmdocker-agent.sh") {
		t.Error("wrapper must no longer be referenced")
	}
	if strings.Contains(out, `ENTRYPOINT ["/usr/local/bin/startup.sh"]`) {
		t.Error("user startup.sh must not become the container ENTRYPOINT")
	}
}

func TestGenerateDockerfileUsesFromVerbatim(t *testing.T) {
	profile := Profile{Dockerfile: DockerfileSection{
		From: "ghcr.io/acme/custom-agent:v1.2.3", Bin: "bin", Startup: "start.sh",
	}}
	out, err := GenerateDockerfile(DockerfileInput{
		Profile:     profile,
		AgentBinSrc: "platform/vmdocker-agent",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(out, "FROM ghcr.io/acme/custom-agent:v1.2.3") {
		t.Fatalf("FROM not used verbatim:\n%s", out)
	}
	if !strings.Contains(out, `ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`) {
		t.Fatalf("ENTRYPOINT not adapter:\n%s", out)
	}
	if strings.Contains(out, "RUNTIME_TYPE") {
		t.Fatalf("RUNTIME_TYPE must not be baked:\n%s", out)
	}
	if !strings.Contains(out, "user-startup.sh") {
		t.Fatalf("startup COPY dropped:\n%s", out)
	}
}

func TestGenerateDockerfileRequiresFrom(t *testing.T) {
	profile := Profile{Dockerfile: DockerfileSection{Bin: "bin", Startup: "start.sh"}}
	if _, err := GenerateDockerfile(DockerfileInput{Profile: profile, AgentBinSrc: "platform/vmdocker-agent"}); err == nil {
		t.Fatal("expected error when FROM is empty")
	}
}
