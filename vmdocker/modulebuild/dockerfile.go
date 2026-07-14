package modulebuild

import (
	"bytes"
	"encoding/json"
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
	CMDLine     string
	Tools       []string
	Run         []string
}

var dockerfileTmpl = template.Must(template.New("dockerfile").Parse(
	`FROM {{.BaseImage}}
USER root
WORKDIR /app

COPY {{.AgentBinSrc}} /usr/local/bin/vmdocker-agent

COPY {{.Bin}}/ /usr/local/bin/
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
    chmod +x /usr/local/bin/*; \
    chown -R hymx:hymx /home/hymx /app
ENV HOME=/home/hymx
USER hymx
WORKDIR /home/hymx
ENTRYPOINT ["/usr/local/bin/vmdocker-agent"]
{{if .CMDLine}}{{.CMDLine}}
{{end}}`))

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
	if strings.TrimSpace(in.AgentBinSrc) == "" {
		return "", fmt.Errorf("platform AgentBinSrc is required")
	}
	cmdLine, err := renderCMD(d.CMD)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := dockerfileTmpl.Execute(&buf, dockerfileView{
		BaseImage:   d.From,
		AgentBinSrc: in.AgentBinSrc,
		Bin:         d.Bin,
		CMDLine:     cmdLine,
		Tools:       d.Tools,
		Run:         d.Run,
	}); err != nil {
		return "", fmt.Errorf("render Dockerfile: %w", err)
	}
	return buf.String(), nil
}

// renderCMD returns the Dockerfile CMD instruction for a [dockerfile].CMD value,
// or "" when no CMD is set. A string is shell form (CMD <string>); a string
// array is exec form (CMD ["a","b"]). Any other shape is an error.
func renderCMD(raw any) (string, error) {
	switch v := raw.(type) {
	case nil:
		return "", nil
	case string:
		if strings.TrimSpace(v) == "" {
			return "", nil
		}
		return "CMD " + v, nil
	case []string:
		return execCMDLine(v)
	case []any:
		args := make([]string, len(v))
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return "", fmt.Errorf("[dockerfile].CMD exec form: element %d is %T, want string", i, e)
			}
			args[i] = s
		}
		return execCMDLine(args)
	default:
		return "", fmt.Errorf("[dockerfile].CMD must be a string (shell form) or an array of strings (exec form), got %T", raw)
	}
}

func execCMDLine(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("render CMD exec form: %w", err)
	}
	return "CMD " + string(b), nil
}
