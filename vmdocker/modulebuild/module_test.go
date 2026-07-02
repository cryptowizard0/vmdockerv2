package modulebuild

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

func readContainerMembers(t *testing.T, moduleBytes []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(moduleBytes))
	if err != nil {
		t.Fatalf("gzip open: %v", err)
	}
	tr := tar.NewReader(gz)
	members := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read member %s: %v", hdr.Name, err)
		}
		members[hdr.Name] = data
	}
	return members
}

func TestPackModule_BuildFlow(t *testing.T) {
	image := []byte("fake-image-archive")
	profile := []byte("[dockerfile]\nFROM=\"openclaw\"\n")
	art, err := PackModule(PackInput{
		ImageArchive:    image,
		ImageName:       "vmdocker-openclaw:abc123",
		ImageID:         "sha256:deadbeef",
		ProfileTOML:     profile,
		Public:          []string{"skills", "persona"},
		IncludeImageSHA: true,
	})
	if err != nil {
		t.Fatalf("PackModule error: %v", err)
	}

	members := readContainerMembers(t, art.ModuleBytes)
	if !bytes.Equal(members["image.tar.gz"], image) {
		t.Error("image.tar.gz member mismatch")
	}
	if !bytes.Equal(members["profile.toml"], profile) {
		t.Error("profile.toml member mismatch")
	}
	if _, ok := members["public.zip"]; ok {
		t.Error("build flow must not include public.zip")
	}

	get := func(name string) string {
		for _, tg := range art.Tags {
			if tg.Name == name {
				return tg.Value
			}
		}
		return ""
	}
	if get("Image-Name") != "vmdocker-openclaw:abc123" {
		t.Errorf("Image-Name tag = %q", get("Image-Name"))
	}
	if get("Image-ID") != "sha256:deadbeef" {
		t.Errorf("Image-ID tag = %q", get("Image-ID"))
	}
	if get("Capability-Public") != "skills,persona" {
		t.Errorf("Capability-Public tag = %q", get("Capability-Public"))
	}
	if get("Created-At") == "" {
		t.Error("Created-At tag missing")
	}
	sum := sha256.Sum256(image)
	if get("Member-Image-SHA256") != hex.EncodeToString(sum[:]) {
		t.Errorf("Member-Image-SHA256 tag = %q", get("Member-Image-SHA256"))
	}
	for _, gone := range []string{"Module-Members", "Member-Profile-SHA256", "Member-Public-SHA256"} {
		if get(gone) != "" {
			t.Errorf("tag %s should have been removed", gone)
		}
	}
}

func TestPackModule_EmitsImageSourceAndArchiveTags(t *testing.T) {
	art, err := PackModule(PackInput{
		ImageArchive: []byte("img"),
		ImageName:    "n",
		ImageID:      "sha256:x",
		ProfileTOML:  []byte("t"),
	})
	if err != nil {
		t.Fatal(err)
	}
	get := func(name string) string {
		for _, tg := range art.Tags {
			if tg.Name == name {
				return tg.Value
			}
		}
		return ""
	}
	if get("Image-Source") != "module-data" {
		t.Errorf("Image-Source = %q, want module-data", get("Image-Source"))
	}
	if get("Image-Archive-Format") != "container-tar+image.tar.gz" {
		t.Errorf("Image-Archive-Format = %q", get("Image-Archive-Format"))
	}
}
