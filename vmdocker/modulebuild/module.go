package modulebuild

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	arSchema "github.com/permadao/goar/schema"
)

const (
	// ModuleFormat is the capability/module format tag value; vmdockerv2 uses a
	// single format for both runtime spawn and module packaging.
	ModuleFormat = runtimeSchema.ModuleFormat

	memberImage   = "image.tar.gz"
	memberProfile = "profile.toml"
	memberPublic  = "public.zip"
)

// ModuleArtifact is the packed, unsigned module payload + extension tags.
// ModuleBytes = gzip(container tar).
type ModuleArtifact struct {
	ModuleBytes []byte
	Tags        []arSchema.Tag
}

// PackInput describes the members + metadata for one module.
// PublicZip is nil for the build flow; set for Export.
type PackInput struct {
	ImageArchive    []byte
	ImageName       string
	ImageID         string
	ProfileTOML     []byte
	PublicZip       []byte
	Public          []string
	IncludeImageSHA bool
}

// PackModule assembles the container tar (image + profile [+ public]) and tags.
func PackModule(in PackInput) (ModuleArtifact, error) {
	if len(in.ImageArchive) == 0 {
		return ModuleArtifact{}, fmt.Errorf("ImageArchive is empty")
	}
	if len(in.ProfileTOML) == 0 {
		return ModuleArtifact{}, fmt.Errorf("ProfileTOML is empty")
	}

	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)

	writeMember := func(name string, data []byte) error {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(data)),
		}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}

	if err := writeMember(memberImage, in.ImageArchive); err != nil {
		return ModuleArtifact{}, fmt.Errorf("write image member: %w", err)
	}
	if err := writeMember(memberProfile, in.ProfileTOML); err != nil {
		return ModuleArtifact{}, fmt.Errorf("write profile member: %w", err)
	}
	if len(in.PublicZip) > 0 {
		if err := writeMember(memberPublic, in.PublicZip); err != nil {
			return ModuleArtifact{}, fmt.Errorf("write public member: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return ModuleArtifact{}, err
	}
	if err := gz.Close(); err != nil {
		return ModuleArtifact{}, err
	}

	tags := []arSchema.Tag{
		{Name: "Image-Name", Value: in.ImageName},
		{Name: "Image-ID", Value: in.ImageID},
		{Name: "Image-Source", Value: "module-data"},
		{Name: "Image-Archive-Format", Value: "container-tar+image.tar.gz"},
		{Name: "Capability-Public", Value: strings.Join(in.Public, ",")},
		{Name: "Created-At", Value: time.Now().UTC().Format(time.RFC3339)},
	}
	if in.IncludeImageSHA {
		sum := sha256.Sum256(in.ImageArchive)
		tags = append(tags, arSchema.Tag{Name: "Member-Image-SHA256", Value: hex.EncodeToString(sum[:])})
	}

	return ModuleArtifact{ModuleBytes: archive.Bytes(), Tags: tags}, nil
}
