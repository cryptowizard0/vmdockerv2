package modulebuild

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// BaseSpec is the resolved result of a FROM alias (spec §5.3).
type BaseSpec struct {
	Image       string // base docker image that already bundles the engine
	RuntimeType string // value for the RUNTIME_TYPE env / adapter dispatch
}

// baseAliases maps profile [dockerfile].FROM aliases to a platform base image.
// hermes is intentionally omitted until its base image is provided.
var baseAliases = map[string]BaseSpec{
	"openclaw": {Image: "docker/sandbox-templates:shell", RuntimeType: "openclaw"},
	"claude":   {Image: "docker/sandbox-templates:claude-code", RuntimeType: "claude"},
}

// ResolveFROM resolves a FROM alias to its base image + runtime type.
func ResolveFROM(alias string) (BaseSpec, error) {
	spec, ok := baseAliases[strings.ToLower(strings.TrimSpace(alias))]
	if !ok {
		return BaseSpec{}, fmt.Errorf("unknown FROM alias %q (supported: openclaw, claude)", alias)
	}
	return spec, nil
}

// DockerfileInput carries everything needed to render the standardized
// Dockerfile. AgentBinSrc/WrapperSrc are build-context paths to the platform
// adapter binary and ENTRYPOINT wrapper.
type DockerfileInput struct {
	Profile     Profile
	AgentBinSrc string
	WrapperSrc  string
}

type dockerfileView struct {
	BaseImage   string
	RuntimeType string
	AgentBinSrc string
	WrapperSrc  string
	Bin         string
	Startup     string
	Tools       []string
	Run         []string
}

var dockerfileTmpl = template.Must(template.New("dockerfile").Parse(
	`FROM {{.BaseImage}}
USER root
WORKDIR /app

COPY {{.AgentBinSrc}} /usr/local/bin/vmdocker-agent
COPY {{.WrapperSrc}} /usr/local/bin/start-vmdocker-agent.sh

COPY {{.Bin}}/ /usr/local/bin/
COPY {{.Startup}} /usr/local/lib/vmdocker-agent/user-startup.sh
COPY profile.toml /home/hymx/profile.toml
{{if .Tools}}RUN set -eux; \
    if command -v apt-get >/dev/null 2>&1; then apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends {{range .Tools}}{{.}} {{end}}&& rm -rf /var/lib/apt/lists/*; \
    elif command -v apk >/dev/null 2>&1; then apk add --no-cache {{range .Tools}}{{.}} {{end}}; \
    elif command -v microdnf >/dev/null 2>&1; then microdnf install -y {{range .Tools}}{{.}} {{end}}&& microdnf clean all; fi
{{end}}RUN set -eux; \
    useradd --create-home --home-dir /home/hymx --shell /bin/bash hymx || true; \
    gpasswd -d hymx sudo || true; \
    gpasswd -d hymx docker || true; \
    rm -f /etc/sudoers.d/*
{{range .Run}}RUN {{.}}
{{end}}RUN set -eux; \
    chmod +x /usr/local/bin/* /usr/local/bin/start-vmdocker-agent.sh /usr/local/lib/vmdocker-agent/user-startup.sh; \
    chown -R hymx:hymx /home/hymx /app
ENV HOME=/home/hymx
ENV RUNTIME_TYPE={{.RuntimeType}}
USER hymx
WORKDIR /home/hymx
ENTRYPOINT ["/usr/local/bin/start-vmdocker-agent.sh"]
`))

// GenerateDockerfile renders the standardized Dockerfile from a profile.
func GenerateDockerfile(in DockerfileInput) (string, error) {
	base, err := ResolveFROM(in.Profile.Dockerfile.From)
	if err != nil {
		return "", err
	}
	d := in.Profile.Dockerfile
	if strings.TrimSpace(d.Bin) == "" {
		return "", fmt.Errorf("profile [dockerfile].bin is required")
	}
	if strings.TrimSpace(d.Startup) == "" {
		return "", fmt.Errorf("profile [dockerfile].startup is required")
	}
	if strings.TrimSpace(in.AgentBinSrc) == "" || strings.TrimSpace(in.WrapperSrc) == "" {
		return "", fmt.Errorf("platform AgentBinSrc and WrapperSrc are required")
	}

	var buf bytes.Buffer
	if err := dockerfileTmpl.Execute(&buf, dockerfileView{
		BaseImage:   base.Image,
		RuntimeType: base.RuntimeType,
		AgentBinSrc: in.AgentBinSrc,
		WrapperSrc:  in.WrapperSrc,
		Bin:         d.Bin,
		Startup:     d.Startup,
		Tools:       d.Tools,
		Run:         d.Run,
	}); err != nil {
		return "", fmt.Errorf("render Dockerfile: %w", err)
	}
	return buf.String(), nil
}
