package modulebuild

import (
	"strings"
	"testing"
)

func TestResolveFROM(t *testing.T) {
	cases := []struct {
		alias       string
		wantImage   string
		wantRuntime string
	}{
		{"openclaw", "docker/sandbox-templates:shell", "openclaw"},
		{"claude", "docker/sandbox-templates:claude-code", "claude"},
	}
	for _, c := range cases {
		got, err := ResolveFROM(c.alias)
		if err != nil {
			t.Fatalf("ResolveFROM(%q) error: %v", c.alias, err)
		}
		if got.Image != c.wantImage || got.RuntimeType != c.wantRuntime {
			t.Fatalf("ResolveFROM(%q) = %+v, want image=%s runtime=%s", c.alias, got, c.wantImage, c.wantRuntime)
		}
	}
}

func TestResolveFROM_Unknown(t *testing.T) {
	if _, err := ResolveFROM("nope"); err == nil {
		t.Fatal("expected error for unknown FROM alias")
	}
}

func TestGenerateDockerfile(t *testing.T) {
	p := Profile{
		Dockerfile: DockerfileSection{
			From:    "openclaw",
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
		"ENV RUNTIME_TYPE=openclaw",
		"USER hymx",
		"WORKDIR /home/hymx",
		`ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`,
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("Dockerfile missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "start-vmdocker-agent.sh") {
		t.Error("wrapper must no longer be referenced")
	}
	if strings.Contains(out, `ENTRYPOINT ["/usr/local/bin/startup.sh"]`) {
		t.Error("user startup.sh must not become the container ENTRYPOINT")
	}
}

func TestGenerateDockerfileUsesAdapterEntrypoint(t *testing.T) {
	profile := Profile{Dockerfile: DockerfileSection{
		From: "openclaw", Bin: "bin", Startup: "start.sh",
	}}
	out, err := GenerateDockerfile(DockerfileInput{
		Profile:     profile,
		AgentBinSrc: "platform/vmdocker-agent",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(out, `ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`) {
		t.Fatalf("ENTRYPOINT not adapter:\n%s", out)
	}
	if strings.Contains(out, "start-vmdocker-agent.sh") {
		t.Fatalf("wrapper still referenced:\n%s", out)
	}
	if !strings.Contains(out, "ENV RUNTIME_TYPE=openclaw") {
		t.Fatalf("RUNTIME_TYPE dropped:\n%s", out)
	}
	if !strings.Contains(out, "user-startup.sh") {
		t.Fatalf("startup COPY dropped:\n%s", out)
	}
}
