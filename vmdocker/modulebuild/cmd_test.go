package modulebuild

import (
	"strings"
	"testing"
)

func genWithCMD(t *testing.T, cmd any) string {
	t.Helper()
	out, err := GenerateDockerfile(DockerfileInput{
		Profile:     Profile{Dockerfile: DockerfileSection{From: "img", Bin: "bin", CMD: cmd}},
		AgentBinSrc: "platform/vmdocker-agent",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile error: %v", err)
	}
	return out
}

func TestGenerateDockerfile_CMDExecForm(t *testing.T) {
	out := genWithCMD(t, []string{"node", "init.js", "--seed"})
	if !strings.Contains(out, `CMD ["node","init.js","--seed"]`) {
		t.Fatalf("exec-form CMD missing:\n%s", out)
	}
}

func TestGenerateDockerfile_CMDShellForm(t *testing.T) {
	out := genWithCMD(t, "node init.js --seed")
	if !strings.Contains(out, "CMD node init.js --seed") {
		t.Fatalf("shell-form CMD missing:\n%s", out)
	}
}

func TestGenerateDockerfile_NoCMDOmitsLine(t *testing.T) {
	out := genWithCMD(t, nil)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "CMD ") {
			t.Fatalf("CMD line must be absent when unset, found %q:\n%s", line, out)
		}
	}
	if !strings.Contains(out, `ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]`) {
		t.Fatalf("ENTRYPOINT must remain the adapter:\n%s", out)
	}
}

func TestGenerateDockerfile_NoUserStartupCopy(t *testing.T) {
	out := genWithCMD(t, []string{"true"})
	if strings.Contains(out, "user-startup.sh") {
		t.Fatalf("user-startup.sh must no longer be referenced:\n%s", out)
	}
}

func TestParseProfile_CMDForms(t *testing.T) {
	execP, err := ParseProfile([]byte("[dockerfile]\nFROM=\"img\"\nbin=\"bin\"\nCMD=[\"node\",\"x\"]\n"))
	if err != nil {
		t.Fatalf("exec CMD parse: %v", err)
	}
	if arr, ok := execP.Dockerfile.CMD.([]any); !ok || len(arr) != 2 {
		t.Fatalf("exec CMD = %#v, want []any len 2", execP.Dockerfile.CMD)
	}
	shellP, err := ParseProfile([]byte("[dockerfile]\nFROM=\"img\"\nbin=\"bin\"\nCMD=\"node x\"\n"))
	if err != nil {
		t.Fatalf("shell CMD parse: %v", err)
	}
	if s, ok := shellP.Dockerfile.CMD.(string); !ok || s != "node x" {
		t.Fatalf("shell CMD = %#v, want \"node x\"", shellP.Dockerfile.CMD)
	}
}

func TestParseProfile_CMDInvalidShape(t *testing.T) {
	if _, err := ParseProfile([]byte("[dockerfile]\nFROM=\"img\"\nbin=\"bin\"\nCMD=42\n")); err == nil {
		t.Fatal("expected error for a non-string/array CMD")
	}
}
