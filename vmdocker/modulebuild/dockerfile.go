package modulebuild

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// DockerfileInput carries everything needed to render the standardized
// Dockerfile. AgentBinSrc is the build-context path to the platform adapter
// binary, which is launched directly as ENTRYPOINT.
type DockerfileInput struct {
	Profile     Profile
	AgentBinSrc string
}

type dockerfileView struct {
	BaseImage   string
	AgentBinSrc string
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
    chmod +x /usr/local/bin/* /usr/local/lib/vmdocker-agent/user-startup.sh; \
    chown -R hymx:hymx /home/hymx /app
ENV HOME=/home/hymx
USER hymx
WORKDIR /home/hymx
ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]
`))

// GenerateDockerfile renders the standardized Dockerfile from a profile.
// FROM is used verbatim as the base image name; RUNTIME_TYPE is not a build-time
// concern (it is supplied at spawn via the Container-Env-RUNTIME_TYPE tag).
func GenerateDockerfile(in DockerfileInput) (string, error) {
	d := in.Profile.Dockerfile
	if strings.TrimSpace(d.From) == "" {
		return "", fmt.Errorf("profile [dockerfile].FROM is required")
	}
	if strings.TrimSpace(d.Bin) == "" {
		return "", fmt.Errorf("profile [dockerfile].bin is required")
	}
	if strings.TrimSpace(d.Startup) == "" {
		return "", fmt.Errorf("profile [dockerfile].startup is required")
	}
	if strings.TrimSpace(in.AgentBinSrc) == "" {
		return "", fmt.Errorf("platform AgentBinSrc is required")
	}

	var buf bytes.Buffer
	if err := dockerfileTmpl.Execute(&buf, dockerfileView{
		BaseImage:   d.From,
		AgentBinSrc: in.AgentBinSrc,
		Bin:         d.Bin,
		Startup:     d.Startup,
		Tools:       d.Tools,
		Run:         d.Run,
	}); err != nil {
		return "", fmt.Errorf("render Dockerfile: %w", err)
	}
	return buf.String(), nil
}
