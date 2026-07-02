package vmdocker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	goarSchema "github.com/permadao/goar/schema"
	goarUtils "github.com/permadao/goar/utils"
)

func TestExtractImageFromContainerTar(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeMember := func(name string, data []byte) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	writeMember("image.tar.gz", []byte("IMAGE-BYTES"))
	writeMember("profile.toml", []byte("PROFILE"))
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := extractImageFromContainerTar(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "IMAGE-BYTES" {
		t.Fatalf("extracted = %q, want IMAGE-BYTES", got)
	}
}

func TestExtractImageFromContainerTarRejectsHugeMember(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "image.tar.gz", Mode: 0o644, Size: maxModuleMemberBytes + 1}); err != nil {
		t.Fatal(err)
	}

	if _, err := extractImageFromContainerTar(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected error for oversized image.tar.gz member")
	}
}

func TestExtractImageFromContainerTar_Missing(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "profile.toml", Mode: 0o644, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := extractImageFromContainerTar(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected error when image.tar.gz member is missing")
	}
}

func TestSeedWorkspaceProfileFromContainerTarModule(t *testing.T) {
	const moduleID = "module-profile"

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	if err := os.MkdirAll("mod", 0o755); err != nil {
		t.Fatalf("mkdir mod failed: %v", err)
	}
	if err := writeContainerModulePayload(moduleID, []byte("image-archive")); err != nil {
		t.Fatalf("write container module payload failed: %v", err)
	}

	workspace := t.TempDir()
	if err := seedWorkspaceProfileFromModule(moduleID, workspace, runtimeSchema.ImageArchiveContainerTarGZ); err != nil {
		t.Fatalf("seedWorkspaceProfileFromModule: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(workspace, "profile.toml"))
	if err != nil {
		t.Fatalf("read seeded profile: %v", err)
	}
	if !strings.Contains(string(got), `FROM="openclaw"`) {
		t.Fatalf("seeded profile = %q", got)
	}
}

func TestEnsureModuleImageAvailableUsesLocalMatch(t *testing.T) {
	const (
		imageName = "example/image:test"
		imageID   = "sha256:expected"
	)

	fakeDocker, logPath, nameState, _, cleanup := installFakeDocker(t, imageName, imageID)
	defer cleanup()
	if err := os.WriteFile(nameState, []byte(""), 0o644); err != nil {
		t.Fatalf("write name state failed: %v", err)
	}

	originalLookPath := dockerLookPath
	dockerLookPath = func(file string) (string, error) {
		if file == "docker" {
			return fakeDocker, nil
		}
		return originalLookPath(file)
	}
	defer func() {
		dockerLookPath = originalLookPath
	}()

	if err := ensureModuleImageAvailable(context.Background(), "unused", runtimeSchema.ImageInfo{
		Name:          imageName,
		SHA:           imageID,
		Source:        runtimeSchema.ImageSourceModuleData,
		ArchiveFormat: runtimeSchema.ImageArchiveDockerSaveGZ,
	}); err != nil {
		t.Fatalf("ensureModuleImageAvailable failed: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	if strings.Contains(string(raw), "image load") {
		t.Fatalf("expected local hit to skip docker image load, got log:\n%s", string(raw))
	}
}

func TestEnsureModuleImageAvailableLoadsFromModuleFileOnMiss(t *testing.T) {
	const (
		moduleID  = "module-1"
		imageName = "example/image:test"
		imageID   = "sha256:expected"
	)

	fakeDocker, logPath, nameState, _, cleanup := installFakeDocker(t, imageName, imageID)
	defer cleanup()

	originalLookPath := dockerLookPath
	dockerLookPath = func(file string) (string, error) {
		if file == "docker" {
			return fakeDocker, nil
		}
		return originalLookPath(file)
	}
	defer func() {
		dockerLookPath = originalLookPath
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	if err := os.MkdirAll("mod", 0o755); err != nil {
		t.Fatalf("mkdir mod failed: %v", err)
	}
	if err := writeModulePayload(moduleID, []byte("tar-contents"), false, false); err != nil {
		t.Fatalf("write module payload failed: %v", err)
	}

	if err := ensureModuleImageAvailable(context.Background(), moduleID, runtimeSchema.ImageInfo{
		Name:          imageName,
		SHA:           imageID,
		Source:        runtimeSchema.ImageSourceModuleData,
		ArchiveFormat: runtimeSchema.ImageArchiveDockerSaveGZ,
	}); err != nil {
		t.Fatalf("ensureModuleImageAvailable failed: %v", err)
	}

	if _, err := os.Stat(nameState); err != nil {
		t.Fatalf("expected image tag state to exist after load: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	log := string(raw)
	if !strings.Contains(log, "image load") {
		t.Fatalf("expected docker image load on cache miss, got log:\n%s", log)
	}
	if !strings.Contains(log, "image tag "+imageID+" "+imageName) {
		t.Fatalf("expected docker image tag after load, got log:\n%s", log)
	}
}

func TestEnsureModuleImageAvailableLoadsFromPrettyPrintedModuleFileOnMiss(t *testing.T) {
	const (
		moduleID  = "module-pretty"
		imageName = "example/image:test"
		imageID   = "sha256:expected"
	)

	fakeDocker, logPath, nameState, _, cleanup := installFakeDocker(t, imageName, imageID)
	defer cleanup()

	originalLookPath := dockerLookPath
	dockerLookPath = func(file string) (string, error) {
		if file == "docker" {
			return fakeDocker, nil
		}
		return originalLookPath(file)
	}
	defer func() {
		dockerLookPath = originalLookPath
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	if err := os.MkdirAll("mod", 0o755); err != nil {
		t.Fatalf("mkdir mod failed: %v", err)
	}
	if err := writeModulePayload(moduleID, []byte("tar-contents"), true, false); err != nil {
		t.Fatalf("write pretty module payload failed: %v", err)
	}

	if err := ensureModuleImageAvailable(context.Background(), moduleID, runtimeSchema.ImageInfo{
		Name:          imageName,
		SHA:           imageID,
		Source:        runtimeSchema.ImageSourceModuleData,
		ArchiveFormat: runtimeSchema.ImageArchiveDockerSaveGZ,
	}); err != nil {
		t.Fatalf("ensureModuleImageAvailable failed: %v", err)
	}

	if _, err := os.Stat(nameState); err != nil {
		t.Fatalf("expected image tag state to exist after load: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	if !strings.Contains(string(raw), "image load") {
		t.Fatalf("expected docker image load on pretty-printed module file, got log:\n%s", string(raw))
	}
}

func TestEnsureModuleImageAvailableLoadsFromContainerTarModuleFileOnMiss(t *testing.T) {
	const (
		moduleID  = "module-v2"
		imageName = "example/image:test"
		imageID   = "sha256:expected"
	)

	fakeDocker, logPath, nameState, _, cleanup := installFakeDocker(t, imageName, imageID)
	defer cleanup()

	originalLookPath := dockerLookPath
	dockerLookPath = func(file string) (string, error) {
		if file == "docker" {
			return fakeDocker, nil
		}
		return originalLookPath(file)
	}
	defer func() {
		dockerLookPath = originalLookPath
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	if err := os.MkdirAll("mod", 0o755); err != nil {
		t.Fatalf("mkdir mod failed: %v", err)
	}
	if err := writeContainerModulePayload(moduleID, []byte("image-archive")); err != nil {
		t.Fatalf("write container module payload failed: %v", err)
	}

	if err := ensureModuleImageAvailable(context.Background(), moduleID, runtimeSchema.ImageInfo{
		Name:          imageName,
		SHA:           imageID,
		Source:        runtimeSchema.ImageSourceModuleData,
		ArchiveFormat: runtimeSchema.ImageArchiveContainerTarGZ,
	}); err != nil {
		t.Fatalf("ensureModuleImageAvailable failed: %v", err)
	}

	if _, err := os.Stat(nameState); err != nil {
		t.Fatalf("expected image tag state to exist after load: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	if !strings.Contains(string(raw), "image load") {
		t.Fatalf("expected docker image load on V2 module file, got log:\n%s", string(raw))
	}
}

func TestEnsureModuleImageAvailableLoadsFromLegacyModuleFileOnMiss(t *testing.T) {
	const (
		moduleID  = "module-legacy"
		imageName = "example/image:test"
		imageID   = "sha256:expected"
	)

	fakeDocker, logPath, nameState, _, cleanup := installFakeDocker(t, imageName, imageID)
	defer cleanup()

	originalLookPath := dockerLookPath
	dockerLookPath = func(file string) (string, error) {
		if file == "docker" {
			return fakeDocker, nil
		}
		return originalLookPath(file)
	}
	defer func() {
		dockerLookPath = originalLookPath
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	if err := writeModulePayload(moduleID, []byte("tar-contents"), false, true); err != nil {
		t.Fatalf("write legacy module payload failed: %v", err)
	}

	if err := ensureModuleImageAvailable(context.Background(), moduleID, runtimeSchema.ImageInfo{
		Name:          imageName,
		SHA:           imageID,
		Source:        runtimeSchema.ImageSourceModuleData,
		ArchiveFormat: runtimeSchema.ImageArchiveDockerSaveGZ,
	}); err != nil {
		t.Fatalf("ensureModuleImageAvailable failed: %v", err)
	}

	if _, err := os.Stat(nameState); err != nil {
		t.Fatalf("expected image tag state to exist after legacy-path load: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log failed: %v", err)
	}
	if !strings.Contains(string(raw), "image load") {
		t.Fatalf("expected docker image load on legacy module path, got log:\n%s", string(raw))
	}
}

func writeContainerModulePayload(moduleID string, imageArchive []byte) error {
	var container bytes.Buffer
	tw := tar.NewWriter(&container)
	writeMember := func(name string, payload []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(payload))}); err != nil {
			return err
		}
		_, err := tw.Write(payload)
		return err
	}
	if err := writeMember("image.tar.gz", imageArchive); err != nil {
		return err
	}
	if err := writeMember("profile.toml", []byte("[dockerfile]\nFROM=\"openclaw\"\n")); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return writeModulePayload(moduleID, container.Bytes(), false, false)
}

func installFakeDocker(t *testing.T, imageName, imageID string) (string, string, string, string, func()) {
	t.Helper()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "docker.log")
	nameState := filepath.Join(tempDir, "name.state")
	idState := filepath.Join(tempDir, "id.state")
	fakeDocker := filepath.Join(tempDir, "docker")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >>%s
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  ref="$5"
  if [ "$ref" = %s ] && [ -f %s ]; then
    echo %s
    exit 0
  fi
  if [ "$ref" = %s ] && [ -f %s ]; then
    echo %s
    exit 0
  fi
  echo "missing image" >&2
  exit 1
fi
if [ "$1" = "image" ] && [ "$2" = "tag" ]; then
  if [ "$3" = %s ] && [ "$4" = %s ] && [ -f %s ]; then
    : > %s
    exit 0
  fi
  echo "cannot tag" >&2
  exit 1
fi
if [ "$1" = "image" ] && [ "$2" = "load" ]; then
  cat >/dev/null
  : > %s
  exit 0
fi
exit 0
`, shellEscapeForModuleTest(logPath), shellEscapeForModuleTest(imageName), shellEscapeForModuleTest(nameState), shellEscapeForModuleTest(imageID), shellEscapeForModuleTest(imageID), shellEscapeForModuleTest(idState), shellEscapeForModuleTest(imageID), shellEscapeForModuleTest(imageID), shellEscapeForModuleTest(imageName), shellEscapeForModuleTest(idState), shellEscapeForModuleTest(nameState), shellEscapeForModuleTest(idState))
	if err := os.WriteFile(fakeDocker, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker failed: %v", err)
	}

	return fakeDocker, logPath, nameState, idState, func() {}
}

func writeModulePayload(moduleID string, payload []byte, pretty, legacy bool) error {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	if _, err := gz.Write(payload); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	item := goarSchema.BundleItem{
		Data: goarUtils.Base64Encode(archive.Bytes()),
	}
	var itemBin []byte
	var err error
	if pretty {
		itemBin, err = json.MarshalIndent(item, "", "  ")
	} else {
		itemBin, err = json.Marshal(item)
	}
	if err != nil {
		return err
	}
	path := moduleFilePath(moduleID)
	if legacy {
		path = legacyModuleFilePath(moduleID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, itemBin, 0o644)
}

func shellEscapeForModuleTest(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
