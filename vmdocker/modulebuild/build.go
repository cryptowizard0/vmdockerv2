package modulebuild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// BuildOptions is the input to BuildModuleArtifact.
type BuildOptions struct {
	ProfileTOML  []byte // raw profile.toml bytes, also copied into the image
	ProfileDir   string // dir containing the user's bin/ dir
	AgentBinPath string // host path to the platform adapter binary
	BuildTag     string // optional docker tag; defaults to a content hash
	PublicZip    []byte // optional public.zip member for runtime Export
}

// BuildModuleArtifact runs the full build flow: stage context -> docker build
// -> docker save|gzip -> PackModule(image+profile).
func BuildModuleArtifact(ctx context.Context, opts BuildOptions) (ModuleArtifact, error) {
	ctxDir, err := stageBuildContext(opts)
	if err != nil {
		return ModuleArtifact{}, err
	}
	defer os.RemoveAll(ctxDir)

	profile, err := ParseProfile(opts.ProfileTOML)
	if err != nil {
		return ModuleArtifact{}, err
	}
	dockerfile, err := os.ReadFile(filepath.Join(ctxDir, "Dockerfile"))
	if err != nil {
		return ModuleArtifact{}, err
	}
	tag := opts.BuildTag
	if tag == "" {
		sum := sha256.Sum256(dockerfile)
		tag = "vmdocker-module:" + hex.EncodeToString(sum[:])[:12]
	}

	if err := dockerBuild(ctx, string(dockerfile), ctxDir, tag, nil); err != nil {
		return ModuleArtifact{}, err
	}
	imageID, err := inspectImageID(ctx, tag)
	if err != nil {
		return ModuleArtifact{}, err
	}
	imageArchive, err := exportImageArchive(ctx, tag)
	if err != nil {
		return ModuleArtifact{}, err
	}
	return PackModule(PackInput{
		ImageArchive:    imageArchive,
		ImageName:       tag,
		ImageID:         imageID,
		ProfileTOML:     opts.ProfileTOML,
		PublicZip:       opts.PublicZip,
		Public:          profile.Vmdocker.Public,
		IncludeImageSHA: true,
	})
}

func stageBuildContext(opts BuildOptions) (string, error) {
	profile, err := ParseProfile(opts.ProfileTOML)
	if err != nil {
		return "", err
	}
	dockerfile, err := GenerateDockerfile(DockerfileInput{
		Profile:     profile,
		AgentBinSrc: "platform/vmdocker-agent",
	})
	if err != nil {
		return "", err
	}

	ctxDir, err := os.MkdirTemp("", "vmdocker-modulebuild-*")
	if err != nil {
		return "", err
	}
	cleanup := func() { _ = os.RemoveAll(ctxDir) }

	if err := os.WriteFile(filepath.Join(ctxDir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		cleanup()
		return "", err
	}
	if err := os.WriteFile(filepath.Join(ctxDir, "profile.toml"), opts.ProfileTOML, 0o600); err != nil {
		cleanup()
		return "", err
	}
	if err := copyTree(filepath.Join(opts.ProfileDir, profile.Dockerfile.Bin), filepath.Join(ctxDir, profile.Dockerfile.Bin)); err != nil {
		cleanup()
		return "", fmt.Errorf("stage bin: %w", err)
	}
	platformDir := filepath.Join(ctxDir, "platform")
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		cleanup()
		return "", err
	}
	if err := copyFile(opts.AgentBinPath, filepath.Join(platformDir, "vmdocker-agent")); err != nil {
		cleanup()
		return "", fmt.Errorf("stage agent binary: %w", err)
	}
	return ctxDir, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyTree(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}
