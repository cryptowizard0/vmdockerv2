package modulebuild

import "testing"

func TestParseProfile_AllFields(t *testing.T) {
	src := []byte(`
[dockerfile]
FROM = "docker/sandbox-templates:shell"
bin = "bin"
tools = ["curl", "ripgrep"]
RUN = ["pip install --no-cache-dir foo"]
startup = "startup.sh"

[vmdocker]
public = ["skills", "persona"]
`)
	p, err := ParseProfile(src)
	if err != nil {
		t.Fatalf("ParseProfile returned error: %v", err)
	}
	if p.Dockerfile.From != "docker/sandbox-templates:shell" {
		t.Fatalf("From = %q, want docker/sandbox-templates:shell", p.Dockerfile.From)
	}
	if p.Dockerfile.Bin != "bin" {
		t.Fatalf("Bin = %q, want bin", p.Dockerfile.Bin)
	}
	if len(p.Dockerfile.Tools) != 2 || p.Dockerfile.Tools[0] != "curl" {
		t.Fatalf("Tools = %v, want [curl ripgrep]", p.Dockerfile.Tools)
	}
	if len(p.Dockerfile.Run) != 1 || p.Dockerfile.Run[0] != "pip install --no-cache-dir foo" {
		t.Fatalf("Run = %v", p.Dockerfile.Run)
	}
	if p.Dockerfile.Startup != "startup.sh" {
		t.Fatalf("Startup = %q, want startup.sh", p.Dockerfile.Startup)
	}
	if len(p.Vmdocker.Public) != 2 || p.Vmdocker.Public[1] != "persona" {
		t.Fatalf("Public = %v, want [skills persona]", p.Vmdocker.Public)
	}
}

func TestParseProfile_RequiresFROM(t *testing.T) {
	_, err := ParseProfile([]byte("[dockerfile]\nbin = \"bin\"\n"))
	if err == nil {
		t.Fatal("expected error when FROM is missing")
	}
}
