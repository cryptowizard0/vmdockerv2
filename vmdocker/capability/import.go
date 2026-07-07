package capability

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/modulebuild"
	"github.com/pelletier/go-toml/v2"
	arSchema "github.com/permadao/goar/schema"
	arutils "github.com/permadao/goar/utils"
)

const (
	moduleFormat          = modulebuild.ModuleFormat
	DefaultMaxModuleBytes = 1 << 30
)

// ImportOptions controls conflict handling and size limits.
type ImportOptions struct {
	OnConflict     string // "skip"(default) | "overwrite" | "fail"
	MaxBytes       int64  // 0 -> DefaultMaxBytes
	MaxModuleBytes int64  // 0 -> DefaultMaxModuleBytes
}

// ImportResult is returned to the caller.
type ImportResult struct {
	Imported       []string `json:"imported"`
	Skipped        []string `json:"skipped"`
	Public         []string `json:"public"`
	ProfileUpdated bool     `json:"profileUpdated"`
}

// Import decodes a V2 module and overlays its public.zip into home.
func Import(home string, moduleBytes []byte, opts ImportOptions) (ImportResult, error) {
	max := opts.MaxBytes
	if max == 0 {
		max = DefaultMaxBytes
	}
	maxModule := opts.MaxModuleBytes
	if maxModule == 0 {
		maxModule = DefaultMaxModuleBytes
	}
	format, members, err := readModule(moduleBytes, max, maxModule)
	if err != nil {
		return ImportResult{}, err
	}
	if format != moduleFormat {
		return ImportResult{}, coded("FORMAT_MISMATCH", "module format %q is not %q", format, moduleFormat)
	}
	publicZip, ok := members["public.zip"]
	if !ok || len(publicZip) == 0 {
		return ImportResult{}, coded("NO_PUBLIC", "public.zip member missing")
	}
	public, err := readTargetPublic(home)
	if err != nil {
		return ImportResult{}, err
	}
	res, err := applyPublicZip(home, publicZip, public, opts)
	if err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

func readTargetPublic(home string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(home, "profile.toml"))
	if err != nil {
		return nil, err
	}
	var profile struct {
		Vmdocker struct {
			Public []string `toml:"public"`
		} `toml:"vmdocker"`
	}
	if err := toml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse target profile.toml: %w", err)
	}
	return profile.Vmdocker.Public, nil
}

func readModule(moduleBytes []byte, maxPublicBytes, maxModuleBytes int64) (string, map[string][]byte, error) {
	var item arSchema.BundleItem
	if err := json.Unmarshal(moduleBytes, &item); err != nil {
		return "", nil, coded("CORRUPT", "unmarshal module: %v", err)
	}
	format := tagValue(item.Tags, "Module-Format")
	data, err := arutils.Base64Decode(item.Data)
	if err != nil {
		return "", nil, coded("CORRUPT", "decode data: %v", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", nil, coded("CORRUPT", "open module gzip: %v", err)
	}
	defer gz.Close()

	members := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, coded("CORRUPT", "read module tar: %v", err)
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		if hdr.Size > maxModuleBytes {
			return "", nil, coded("TOO_LARGE", "module member %q %d bytes exceeds %d", hdr.Name, hdr.Size, maxModuleBytes)
		}
		if hdr.Name != "public.zip" {
			if err := discardLimited(tr, hdr.Size, maxModuleBytes); err != nil {
				return "", nil, err
			}
			continue
		}
		if hdr.Size > maxPublicBytes {
			return "", nil, coded("TOO_LARGE", "public.zip %d bytes exceeds %d", hdr.Size, maxPublicBytes)
		}
		data, err := readLimited(tr, maxPublicBytes)
		if err != nil {
			return "", nil, err
		}
		members[hdr.Name] = data
	}
	return format, members, nil
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(r, max+1))
	if err != nil {
		return nil, coded("CORRUPT", "read limited payload: %v", err)
	}
	if n > max {
		return nil, coded("TOO_LARGE", "payload %d bytes exceeds %d", n, max)
	}
	return buf.Bytes(), nil
}

func discardLimited(r io.Reader, declaredSize, max int64) error {
	if declaredSize > max {
		return coded("TOO_LARGE", "payload %d bytes exceeds %d", declaredSize, max)
	}
	n, err := io.Copy(io.Discard, io.LimitReader(r, max+1))
	if err != nil {
		return err
	}
	if n > max {
		return coded("TOO_LARGE", "payload %d bytes exceeds %d", n, max)
	}
	return nil
}

func applyPublicZip(home string, publicZip []byte, allowedRoots []string, opts ImportOptions) (ImportResult, error) {
	max := opts.MaxBytes
	if max == 0 {
		max = DefaultMaxBytes
	}
	if int64(len(publicZip)) > max {
		return ImportResult{}, coded("TOO_LARGE", "public.zip %d bytes exceeds %d", len(publicZip), max)
	}
	zr, err := zip.NewReader(bytes.NewReader(publicZip), int64(len(publicZip)))
	if err != nil {
		return ImportResult{}, coded("CORRUPT", "open public.zip: %v", err)
	}

	// The target's own profile `public` entries are the import allowlist.
	// Malformed entries are skipped (fail-closed: they allow nothing).
	allowPatterns, _ := compilePublicPatterns(allowedRoots)

	res := ImportResult{Public: append([]string(nil), allowedRoots...)}
	type rollbackEntry struct {
		path    string
		existed bool
		data    []byte
		mode    os.FileMode
	}
	placed := make([]rollbackEntry, 0)
	rollback := func() {
		for i := len(placed) - 1; i >= 0; i-- {
			entry := placed[i]
			if !entry.existed {
				_ = os.Remove(entry.path)
				continue
			}
			_ = os.WriteFile(entry.path, entry.data, entry.mode)
		}
	}
	remaining := max
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if zf.FileInfo().Mode()&os.ModeSymlink != 0 {
			rollback()
			return ImportResult{}, coded("PATH_ESCAPE", "zip entry %q is a symlink", zf.Name)
		}
		rel := path.Clean(zf.Name)
		if strings.HasPrefix(zf.Name, "/") || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			rollback()
			return ImportResult{}, coded("PATH_ESCAPE", "zip entry %q escapes", zf.Name)
		}
		if !matchesAnyPublic(rel, allowPatterns) {
			rollback()
			return ImportResult{}, coded("UNAUTHORIZED_PATH", "%q not under target public whitelist", rel)
		}
		dst := filepath.Join(home, filepath.FromSlash(rel))
		if dst != home && !strings.HasPrefix(dst, home+string(os.PathSeparator)) {
			rollback()
			return ImportResult{}, coded("PATH_ESCAPE", "%q resolves outside HOME", rel)
		}

		var rb rollbackEntry
		if _, err := os.Stat(dst); err == nil {
			switch opts.OnConflict {
			case "", "skip":
				res.Skipped = append(res.Skipped, rel)
				continue
			case "overwrite":
				info, err := os.Stat(dst)
				if err != nil {
					rollback()
					return ImportResult{}, err
				}
				original, err := os.ReadFile(dst)
				if err != nil {
					rollback()
					return ImportResult{}, err
				}
				rb = rollbackEntry{path: dst, existed: true, data: original, mode: info.Mode().Perm()}
			case "fail":
				rollback()
				return ImportResult{}, coded("CONFLICT", "%q already exists", rel)
			default:
				rollback()
				return ImportResult{}, coded("CONFLICT", "unknown on_conflict %q", opts.OnConflict)
			}
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			rollback()
			return ImportResult{}, err
		}
		if rb.path == "" {
			rb = rollbackEntry{path: dst}
		}
		rc, err := zf.Open()
		if err != nil {
			rollback()
			return ImportResult{}, coded("CORRUPT", "open zip member %q: %v", rel, err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(dst), ".import-*")
		if err != nil {
			rc.Close()
			rollback()
			return ImportResult{}, err
		}
		written, copyErr := copyLimited(tmp, rc, remaining)
		closeInErr := rc.Close()
		closeOutErr := tmp.Close()
		if copyErr != nil {
			_ = os.Remove(tmp.Name())
			rollback()
			return ImportResult{}, copyErr
		}
		if closeInErr != nil {
			_ = os.Remove(tmp.Name())
			rollback()
			return ImportResult{}, closeInErr
		}
		if closeOutErr != nil {
			_ = os.Remove(tmp.Name())
			rollback()
			return ImportResult{}, closeOutErr
		}
		remaining -= written
		if err := os.Rename(tmp.Name(), dst); err != nil {
			_ = os.Remove(tmp.Name())
			rollback()
			return ImportResult{}, err
		}
		placed = append(placed, rb)
		res.Imported = append(res.Imported, rel)
	}
	return res, nil
}

func copyLimited(dst io.Writer, src io.Reader, max int64) (int64, error) {
	if max < 0 {
		max = 0
	}
	n, err := io.Copy(dst, io.LimitReader(src, max+1))
	if err != nil {
		return n, err
	}
	if n > max {
		return n, coded("TOO_LARGE", "expanded public.zip content exceeds %d bytes", max)
	}
	return n, nil
}
