// Command vmme2e is a thin end-to-end driver for the host-side capability
// operations (seed / export) that are handled inside vmdocker.Apply and
// therefore cannot be reached by curling the container's /vmm. It wraps the real
// production code (no new logic) so scripts/e2e_capability.sh can drive a
// real-container round-trip and the hardening negative cases. See the plan:
// docs/vmdocker-profile-module.
package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cryptowizard0/vmdockerv2/vmdocker"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/capability"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/modulebuild"
	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "seed":
		err = cmdSeed(os.Args[2:])
	case "seed-clone":
		err = cmdSeedClone(os.Args[2:])
	case "export":
		err = cmdExport(os.Args[2:])
	case "pack-synthetic":
		err = cmdPackSynthetic(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		// Surface a stable error code (e.g. UNAUTHORIZED_PATH, TOO_LARGE) so the
		// shell can assert on it; exit 3 for coded capability errors, 1 otherwise.
		var coded *capability.CodedError
		if errors.As(err, &coded) {
			fmt.Fprintln(os.Stderr, coded.Code+": "+coded.Err.Error())
			os.Exit(3)
		}
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: vmme2e <seed|seed-clone|export|pack-synthetic> [flags]")
	os.Exit(2)
}

// flagset is a tiny flag parser: --key value / --key=value / --flag (bool "true").
func flagset(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) < 2 || a[0] != '-' {
			continue
		}
		a = a[1:]
		if len(a) > 0 && a[0] == '-' {
			a = a[1:]
		}
		if eq := indexByte(a, '='); eq >= 0 {
			out[a[:eq]] = a[eq+1:]
			continue
		}
		if i+1 < len(args) && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
			out[a] = args[i+1]
			i++
		} else {
			out[a] = "true"
		}
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func require(f map[string]string, key string) (string, error) {
	if v, ok := f[key]; ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("missing required flag --%s", key)
}

// seed exercises the real spawn-time seeding of profile.toml into the workspace.
// The module file must be resolvable relative to CWD (mod/mod-<id>.json or
// mod-<id>.json), matching resolveModuleFilePath in the runtime.
func cmdSeed(args []string) error {
	f := flagset(args)
	id, err := require(f, "module-id")
	if err != nil {
		return err
	}
	ws, err := require(f, "workspace")
	if err != nil {
		return err
	}
	format := f["archive-format"]
	if format == "" {
		format = runtimeSchema.ImageArchiveContainerTarGZ
	}
	if err := vmdocker.SeedWorkspaceProfileFromModule(id, ws, format); err != nil {
		return err
	}
	fmt.Printf("seeded %s\n", filepath.Join(ws, "profile.toml"))
	return nil
}

// seed-clone exercises the real spawn-time seeding of profile.toml AND public.zip
// into a fresh workspace. The module file must be resolvable relative to CWD
// (mod/mod-<id>.json), matching resolveModuleFilePath in the runtime.
func cmdSeedClone(args []string) error {
	f := flagset(args)
	id, err := require(f, "module-id")
	if err != nil {
		return err
	}
	ws, err := require(f, "workspace")
	if err != nil {
		return err
	}
	format := f["archive-format"]
	if format == "" {
		format = runtimeSchema.ImageArchiveContainerTarGZ
	}
	if err := vmdocker.SeedWorkspaceFromModule(id, ws, format); err != nil {
		return err
	}
	fmt.Printf("seeded clone workspace %s\n", ws)
	return nil
}

// export runs the real host-side Export (rebuilds the image + packs public.zip).
func cmdExport(args []string) error {
	f := flagset(args)
	ws, err := require(f, "workspace")
	if err != nil {
		return err
	}
	out, err := require(f, "out")
	if err != nil {
		return err
	}
	res, err := capability.Export(context.Background(), ws, capability.ExportOptions{
		AgentBinPath: f["agent-bin"],
		BuildTag:     f["build-tag"],
		SignerKey:    f["signer-key"],
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, res.ModuleBytes, 0o644); err != nil {
		return err
	}
	fmt.Printf("exported module %s (%d public files)\n", out, len(res.Collection.Entries))
	return nil
}

// pack-synthetic builds a V2 module directly (stub image + profile + a public.zip
// zipped verbatim from --public-dir) so the shell can craft negative cases
// (arbitrary/out-of-whitelist paths, compressible members) without docker.
func cmdPackSynthetic(args []string) error {
	f := flagset(args)
	profilePath, err := require(f, "profile")
	if err != nil {
		return err
	}
	publicDir, err := require(f, "public-dir")
	if err != nil {
		return err
	}
	out, err := require(f, "out")
	if err != nil {
		return err
	}
	profileTOML, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	profile, err := modulebuild.ParseProfile(profileTOML)
	if err != nil {
		return err
	}
	publicZip, err := zipDir(publicDir)
	if err != nil {
		return err
	}
	artifact, err := modulebuild.PackModule(modulebuild.PackInput{
		ImageArchive:    stubImageArchive(),
		ImageName:       valueOr(f["image-name"], "vmme2e-synth:latest"),
		ImageID:         valueOr(f["image-id"], "sha256:0000000000000000000000000000000000000000000000000000000000000000"),
		ProfileTOML:     profileTOML,
		PublicZip:       publicZip,
		Public:          profile.Vmdocker.Public,
		IncludeImageSHA: true,
	})
	if err != nil {
		return err
	}
	moduleBytes, err := capability.SignModuleArtifact(artifact, f["signer-key"])
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, moduleBytes, 0o644); err != nil {
		return err
	}
	fmt.Printf("packed synthetic module %s\n", out)
	return nil
}

func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// zipDir zips every regular file under dir with its forward-slash relative path
// (no whitelist filtering — intentional, so negative cases can stage arbitrary
// paths like secret/x).
func zipDir(dir string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// stubImageArchive returns a small gzip payload standing in for image.tar.gz;
// nothing in this driver reads the image member, so any non-empty bytes suffice.
func stubImageArchive() []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("vmme2e-stub-image"))
	_ = gz.Close()
	return buf.Bytes()
}
