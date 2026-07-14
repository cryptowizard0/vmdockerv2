// Package modulebuild builds a signed vmdocker module from a declarative profile:
// profile.toml -> standardized Dockerfile -> docker build/save -> container-tar module.
package modulebuild

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Profile is the declarative build recipe (profile.toml). See spec §5.
type Profile struct {
	Dockerfile DockerfileSection `toml:"dockerfile"`
	Vmdocker   VmdockerSection   `toml:"vmdocker"`
}

// DockerfileSection maps to [dockerfile]; keys mirror Dockerfile instructions
// where one exists (FROM/RUN), plus convenience keys (bin/tools/startup).
type DockerfileSection struct {
	From    string   `toml:"FROM"`    // full base image name, used verbatim as Dockerfile FROM
	Bin     string   `toml:"bin"`     // user executables dir, COPY'd to /usr/local/bin
	Tools   []string `toml:"tools"`   // packages to install
	Run     []string `toml:"RUN"`     // custom RUN args (without the RUN prefix)
	// CMD is the module's startup command, mirroring Dockerfile CMD syntax: a
	// string is shell form, an array of strings is exec form. It is baked as the
	// image CMD and run by the adapter (which stays the ENTRYPOINT). Optional.
	CMD any `toml:"CMD"`
}

// VmdockerSection maps to [vmdocker]; consumed by runtime Export/Import (P4).
type VmdockerSection struct {
	// Public is the exportable path allowlist. Each entry MUST start with "~/"
	// and is a HOME-relative glob: '*' matches any characters including '/'
	// (recursive), '?' matches one character. Examples:
	//   public = ["~/skills/*", "~/persona/*.md", "~/investment.md"]
	Public []string `toml:"public"`
}

// ParseProfile parses profile.toml bytes and validates required fields.
func ParseProfile(data []byte) (Profile, error) {
	var p Profile
	if err := toml.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("parse profile.toml: %w", err)
	}
	if strings.TrimSpace(p.Dockerfile.From) == "" {
		return Profile{}, fmt.Errorf("profile [dockerfile].FROM is required")
	}
	if _, err := renderCMD(p.Dockerfile.CMD); err != nil {
		return Profile{}, err
	}
	return p, nil
}
