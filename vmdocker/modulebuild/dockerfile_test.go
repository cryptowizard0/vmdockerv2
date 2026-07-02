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
		WrapperSrc:  "platform/start-vmdocker-agent.sh",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile error: %v", err)
	}
	mustContain := []string{
		"FROM docker/sandbox-templates:shell",
		"COPY platform/vmdocker-agent /usr/local/bin/vmdocker-agent",
		"COPY platform/start-vmdocker-agent.sh /usr/local/bin/start-vmdocker-agent.sh",
		"COPY bin/ /usr/local/bin/",
		"COPY startup.sh /usr/local/lib/vmdocker-agent/user-startup.sh",
		"COPY profile.toml /home/hymx/profile.toml",
		"RUN pip install --no-cache-dir foo",
		"useradd",
		"ENV HOME=/home/hymx",
		"ENV RUNTIME_TYPE=openclaw",
		"USER hymx",
		"WORKDIR /home/hymx",
		`ENTRYPOINT ["/usr/local/bin/start-vmdocker-agent.sh"]`,
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("Dockerfile missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, `ENTRYPOINT ["/usr/local/bin/startup.sh"]`) ||
		strings.Contains(out, "COPY startup.sh /usr/local/bin/start-vmdocker-agent.sh") {
		t.Error("user startup.sh must not become the container ENTRYPOINT")
	}
}
