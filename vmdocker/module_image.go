package vmdocker

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/capability"
	runtimeSchema "github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	goarSchema "github.com/permadao/goar/schema"
)

var dockerLookPath = exec.LookPath

const maxModuleMemberBytes int64 = 1 << 30

func ensureModuleImageAvailable(ctx context.Context, moduleID string, image runtimeSchema.ImageInfo) error {
	if image.Source == "" && image.ArchiveFormat == "" {
		return nil
	}
	if image.Source != runtimeSchema.ImageSourceModuleData {
		return fmt.Errorf("unsupported image source %q", image.Source)
	}
	if image.ArchiveFormat != runtimeSchema.ImageArchiveDockerSaveGZ && image.ArchiveFormat != runtimeSchema.ImageArchiveContainerTarGZ {
		return fmt.Errorf("unsupported image archive format %q", image.ArchiveFormat)
	}

	cliBin, err := dockerBinary()
	if err != nil {
		return err
	}

	matched, err := imageMatchesRef(ctx, cliBin, image.Name, image.SHA)
	if err == nil && matched {
		return nil
	}
	if err == nil && !matched {
		log.Info("local image tag exists but sha mismatched, reloading from module", "module", moduleID, "image", image.Name, "expected_sha", image.SHA)
	}

	if err := ensureImageTaggedByID(ctx, cliBin, image.SHA, image.Name); err == nil {
		return nil
	}

	if err := dockerLoadArchive(ctx, cliBin, moduleID, image.ArchiveFormat); err != nil {
		return err
	}
	if err := ensureImageTaggedByID(ctx, cliBin, image.SHA, image.Name); err != nil {
		return fmt.Errorf("loaded image from module %s but failed to tag/verify %s: %w", moduleID, image.Name, err)
	}
	return nil
}

func dockerBinary() (string, error) {
	cliBin, err := dockerLookPath("docker")
	if err != nil {
		return "", fmt.Errorf("docker CLI is not available: %w", err)
	}
	return cliBin, nil
}

func imageMatchesRef(ctx context.Context, cliBin, ref, expectedID string) (bool, error) {
	actualID, err := inspectLocalImageID(ctx, cliBin, ref)
	if err != nil {
		return false, err
	}
	return actualID == expectedID, nil
}

func inspectLocalImageID(ctx context.Context, cliBin, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, cliBin, "image", "inspect", "--format", "{{.Id}}", ref)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect image %s failed: %w: %s", ref, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func ensureImageTaggedByID(ctx context.Context, cliBin, imageID, imageName string) error {
	if _, err := inspectLocalImageID(ctx, cliBin, imageID); err != nil {
		return err
	}
	if matched, err := imageMatchesRef(ctx, cliBin, imageName, imageID); err == nil && matched {
		return nil
	}
	cmd := exec.CommandContext(ctx, cliBin, "image", "tag", imageID, imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tag image %s as %s failed: %w: %s", imageID, imageName, err, strings.TrimSpace(string(output)))
	}
	matched, err := imageMatchesRef(ctx, cliBin, imageName, imageID)
	if err != nil {
		return err
	}
	if !matched {
		return fmt.Errorf("image %s does not match expected id %s after tagging", imageName, imageID)
	}
	return nil
}

func dockerLoadArchive(ctx context.Context, cliBin, moduleID, archiveFormat string) error {
	payload, err := openModulePayloadGzip(moduleID)
	if err != nil {
		return fmt.Errorf("read module %s payload stream failed: %w", moduleID, err)
	}
	defer payload.Close()
	loadStream := io.Reader(payload)
	if archiveFormat == runtimeSchema.ImageArchiveContainerTarGZ {
		img, err := extractImageFromContainerTar(payload)
		if err != nil {
			return err
		}
		defer img.Close()
		loadStream = img
	}

	cmd := exec.CommandContext(ctx, cliBin, "image", "load")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open docker load stdin failed: %w", err)
	}
	copyErrCh := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stdin, loadStream)
		closeErr := stdin.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		copyErrCh <- copyErr
	}()
	output, err := cmd.CombinedOutput()
	copyErr := <-copyErrCh
	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		return fmt.Errorf("stream docker image load payload failed: %w", copyErr)
	}
	if err != nil {
		return fmt.Errorf("docker image load failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// SeedWorkspaceProfileFromModule exposes seedWorkspaceProfileFromModule for
// out-of-package tooling (the e2e driver). It resolves the module file relative
// to the current working directory, like the runtime spawn path.
func SeedWorkspaceProfileFromModule(moduleID, workspace, archiveFormat string) error {
	return seedWorkspaceProfileFromModule(moduleID, workspace, archiveFormat)
}

func seedWorkspaceProfileFromModule(moduleID, workspace, archiveFormat string) error {
	if archiveFormat != runtimeSchema.ImageArchiveContainerTarGZ {
		return nil
	}
	payload, err := openModulePayloadGzip(moduleID)
	if err != nil {
		return fmt.Errorf("read module %s payload stream failed: %w", moduleID, err)
	}
	defer payload.Close()

	profile, err := extractMemberFromContainerTar(payload, "profile.toml", 1<<20)
	if err != nil {
		return err
	}
	defer profile.Close()
	data, err := io.ReadAll(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workspace, "profile.toml"), data, 0o644)
}

func openModulePayloadGzip(moduleID string) (io.ReadCloser, error) {
	modulePath, err := resolveModuleFilePath(moduleID)
	if err != nil {
		return nil, fmt.Errorf("read module file for %s failed: %w", moduleID, err)
	}

	file, err := os.Open(modulePath)
	if err != nil {
		return nil, fmt.Errorf("open module file %s failed: %w", modulePath, err)
	}
	dataReader, err := newModuleDataReader(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	base64Reader := base64.NewDecoder(base64.RawURLEncoding, dataReader)
	reader, err := gzip.NewReader(base64Reader)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("open gzip payload failed: %w", err)
	}
	return &modulePayloadReader{Reader: reader, file: file}, nil
}

type modulePayloadReader struct {
	*gzip.Reader
	file *os.File
}

func (r *modulePayloadReader) Close() error {
	gzipErr := r.Reader.Close()
	fileErr := r.file.Close()
	if gzipErr != nil {
		return gzipErr
	}
	return fileErr
}

func extractImageFromContainerTar(r io.Reader) (io.ReadCloser, error) {
	return extractMemberFromContainerTar(r, "image.tar.gz", maxModuleMemberBytes)
}

// readModuleImageArchive reads the image.tar.gz member out of a module's
// container-tar payload and returns it fully in memory. Export (Option A) uses
// it to reuse the running agent's existing image instead of rebuilding: the
// image-build inputs (bin/, start.sh) live baked in the image, not in the
// runtime workspace, so a rebuild is impossible — but the exact image bytes are
// already in the module the process was spawned from.
func readModuleImageArchive(moduleID, archiveFormat string) ([]byte, error) {
	if archiveFormat != runtimeSchema.ImageArchiveContainerTarGZ {
		return nil, fmt.Errorf("module %s image archive format %q is not reusable for export", moduleID, archiveFormat)
	}
	payload, err := openModulePayloadGzip(moduleID)
	if err != nil {
		return nil, fmt.Errorf("read module %s payload stream failed: %w", moduleID, err)
	}
	defer payload.Close()

	rc, err := extractImageFromContainerTar(payload)
	if err != nil {
		return nil, fmt.Errorf("extract image.tar.gz from module %s failed: %w", moduleID, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// persistModuleToStore parses the exported module's BundleItem id and writes the
// full module JSON into the node's module store (mod/mod-<id>.json — the path
// resolveModuleFilePath loads from), returning the id. Export uses this instead
// of returning the module bytes through the result channel: the module embeds
// the full container image (hundreds of MB) and would blow past redis's
// proto-max-bulk-len if routed as a message result.
func persistModuleToStore(moduleBytes []byte) (string, error) {
	var item goarSchema.BundleItem
	if err := json.Unmarshal(moduleBytes, &item); err != nil {
		return "", fmt.Errorf("parse exported module: %w", err)
	}
	if item.Id == "" {
		return "", fmt.Errorf("exported module has empty id")
	}
	path := moduleFilePath(item.Id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, moduleBytes, 0o644); err != nil {
		return "", fmt.Errorf("write module %s: %w", item.Id, err)
	}
	return item.Id, nil
}

func extractMemberFromContainerTar(r io.Reader, memberName string, maxBytes int64) (io.ReadCloser, error) {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s member not found in module container tar", memberName)
		}
		if err != nil {
			return nil, fmt.Errorf("read module container tar: %w", err)
		}
		if hdr.Name == memberName {
			if hdr.Size > maxBytes {
				return nil, fmt.Errorf("%s member %d bytes exceeds %d", memberName, hdr.Size, maxBytes)
			}
			tmp, err := os.CreateTemp("", "vmdocker-module-member-*")
			if err != nil {
				return nil, err
			}
			n, err := io.Copy(tmp, io.LimitReader(tr, maxBytes+1))
			if err != nil {
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
				return nil, err
			}
			if n > maxBytes {
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
				return nil, fmt.Errorf("%s member exceeds %d bytes", memberName, maxBytes)
			}
			if _, err := tmp.Seek(0, io.SeekStart); err != nil {
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
				return nil, err
			}
			return &tempFileReadCloser{File: tmp, path: tmp.Name()}, nil
		}
	}
}

type tempFileReadCloser struct {
	*os.File
	path string
}

func (r *tempFileReadCloser) Close() error {
	closeErr := r.File.Close()
	removeErr := os.Remove(r.path)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func moduleFilePath(moduleID string) string {
	return filepath.Join("mod", fmt.Sprintf("mod-%s.json", moduleID))
}

func legacyModuleFilePath(moduleID string) string {
	return fmt.Sprintf("mod-%s.json", moduleID)
}

func resolveModuleFilePath(moduleID string) (string, error) {
	candidates := []string{
		moduleFilePath(moduleID),
		legacyModuleFilePath(moduleID),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", os.ErrNotExist
}

func newModuleDataReader(file *os.File) (io.Reader, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(file)
	inObject := 0
	expectingKey := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("module data field not found")
			}
			return nil, err
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				inObject++
				expectingKey = true
			case '}':
				if inObject > 0 {
					inObject--
				}
				expectingKey = inObject > 0
			case '[':
				expectingKey = false
			case ']':
				expectingKey = inObject > 0
			}
		case string:
			if inObject > 0 && expectingKey {
				if v == "data" {
					return newJSONStringValueReader(file, decoder.InputOffset())
				}
				expectingKey = false
				continue
			}
			if inObject > 0 {
				expectingKey = true
			}
		default:
			if inObject > 0 && !expectingKey {
				expectingKey = true
			}
		}
	}
}

type jsonStringValueReader struct {
	reader *bufio.Reader
	buf    []byte
	done   bool
	escape bool
}

func (r *jsonStringValueReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) {
		if len(r.buf) > 0 {
			copied := copy(p[n:], r.buf)
			r.buf = r.buf[copied:]
			n += copied
			if n == len(p) {
				return n, nil
			}
			continue
		}

		b, err := r.reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if n > 0 {
					return n, fmt.Errorf("unexpected EOF while reading module data field")
				}
				return 0, fmt.Errorf("unexpected EOF while reading module data field")
			}
			if n > 0 {
				return n, err
			}
			return 0, err
		}

		if r.escape {
			decoded, err := decodeJSONStringEscape(r.reader, b)
			if err != nil {
				if n > 0 {
					return n, err
				}
				return 0, err
			}
			r.escape = false
			if len(decoded) == 0 {
				continue
			}
			r.buf = decoded
			continue
		}

		switch b {
		case '\\':
			r.escape = true
		case '"':
			r.done = true
			if n == 0 {
				return 0, io.EOF
			}
			return n, io.EOF
		default:
			p[n] = b
			n++
		}
	}
	return n, nil
}

func newJSONStringValueReader(file *os.File, offset int64) (io.Reader, error) {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	if err := consumeJSONStringValueStart(reader); err != nil {
		return nil, err
	}
	return &jsonStringValueReader{reader: reader}, nil
}

func consumeJSONStringValueStart(reader *bufio.Reader) error {
	b, err := readJSONNonSpaceByte(reader)
	if err != nil {
		return err
	}
	if b != ':' {
		return fmt.Errorf("expected ':' before module data value, got %q", b)
	}
	b, err = readJSONNonSpaceByte(reader)
	if err != nil {
		return err
	}
	if b != '"' {
		return fmt.Errorf("expected '\"' to start module data value, got %q", b)
	}
	return nil
}

func readJSONNonSpaceByte(reader *bufio.Reader) (byte, error) {
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		switch b {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return b, nil
		}
	}
}

func decodeJSONStringEscape(reader *bufio.Reader, esc byte) ([]byte, error) {
	switch esc {
	case '"', '\\', '/':
		return []byte{esc}, nil
	case 'b':
		return []byte{'\b'}, nil
	case 'f':
		return []byte{'\f'}, nil
	case 'n':
		return []byte{'\n'}, nil
	case 'r':
		return []byte{'\r'}, nil
	case 't':
		return []byte{'\t'}, nil
	case 'u':
		r, err := readUnicodeEscape(reader)
		if err != nil {
			return nil, err
		}
		return []byte(string(r)), nil
	default:
		return nil, fmt.Errorf("unsupported JSON escape sequence \\%c in module data field", esc)
	}
}

func readUnicodeEscape(reader *bufio.Reader) (rune, error) {
	hex, err := readExactBytes(reader, 4)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(string(hex), 16, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid unicode escape in module data field: %w", err)
	}
	first := rune(value)
	if !utf16.IsSurrogate(first) {
		return first, nil
	}

	slash, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	u, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	if slash != '\\' || u != 'u' {
		return 0, fmt.Errorf("invalid surrogate pair in module data field")
	}
	secondHex, err := readExactBytes(reader, 4)
	if err != nil {
		return 0, err
	}
	secondValue, err := strconv.ParseUint(string(secondHex), 16, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid unicode escape in module data field: %w", err)
	}
	second := rune(secondValue)
	decoded := utf16.DecodeRune(first, second)
	if decoded == utf8.RuneError {
		return 0, fmt.Errorf("invalid surrogate pair in module data field")
	}
	return decoded, nil
}

func readExactBytes(reader *bufio.Reader, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// SeedWorkspaceFromModule exposes seedWorkspaceFromModule for out-of-package
// tooling (the e2e driver). It resolves the module file relative to the current
// working directory, like the runtime spawn path.
func SeedWorkspaceFromModule(moduleID, workspace, archiveFormat string) error {
	return seedWorkspaceFromModule(moduleID, workspace, archiveFormat)
}

// seedWorkspaceFromModule writes the module's profile.toml into workspace and,
// if the module carries a public.zip member, unpacks it into workspace — both in
// a single pass over the container tar. It is the spawn-time seed for a freshly
// created (empty) workspace. A missing public.zip member is a no-op (build-flow
// modules have none); a non-container-tar archive format is a no-op.
func seedWorkspaceFromModule(moduleID, workspace, archiveFormat string) error {
	if archiveFormat != runtimeSchema.ImageArchiveContainerTarGZ {
		return nil
	}
	payload, err := openModulePayloadGzip(moduleID)
	if err != nil {
		return fmt.Errorf("read module %s payload stream failed: %w", moduleID, err)
	}
	defer payload.Close()

	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}

	tr := tar.NewReader(payload)
	seededProfile := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read module container tar: %w", err)
		}
		switch hdr.Name {
		case "profile.toml":
			if hdr.Size > 1<<20 {
				return fmt.Errorf("profile.toml member %d bytes exceeds %d", hdr.Size, 1<<20)
			}
			data, err := io.ReadAll(io.LimitReader(tr, 1<<20))
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(workspace, "profile.toml"), data, 0o644); err != nil {
				return err
			}
			seededProfile = true
		case "public.zip":
			// public.zip is fully buffered into memory here, so bound it by the
			// same limit UnpackPublicZip enforces (DefaultMaxBytes, 64 MiB) rather
			// than the 1 GiB image-member limit. Otherwise a hostile module could
			// force the host to buffer up to 1 GiB before unpack rejects it — a
			// memory-pressure vector. The LimitReader caps the read even when a
			// crafted tar header understates hdr.Size.
			if hdr.Size > capability.DefaultMaxBytes {
				return fmt.Errorf("public.zip member %d bytes exceeds %d", hdr.Size, capability.DefaultMaxBytes)
			}
			data, err := io.ReadAll(io.LimitReader(tr, capability.DefaultMaxBytes+1))
			if err != nil {
				return err
			}
			if int64(len(data)) > capability.DefaultMaxBytes {
				return fmt.Errorf("public.zip member exceeds %d bytes", capability.DefaultMaxBytes)
			}
			if err := capability.UnpackPublicZip(workspace, data); err != nil {
				return err
			}
		}
	}
	if !seededProfile {
		return fmt.Errorf("profile.toml member not found in module container tar")
	}
	return nil
}
