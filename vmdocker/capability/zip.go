package capability

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Preview runs only the collection phase; it never builds an image.
func Preview(home string, public []string) (Collection, error) {
	return CollectPublic(home, public)
}

// BuildPublicZip zips the collected public files, preserving HOME-relative paths.
func BuildPublicZip(home string, public []string) ([]byte, error) {
	col, err := CollectPublic(home, public)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range col.Entries {
		data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(e.Path)))
		if err != nil {
			return nil, err
		}
		w, err := zw.Create(e.Path)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnpackPublicZip extracts a module's public.zip into dst (a freshly created,
// empty spawn workspace) with path-safety only: absolute paths, ".." escapes,
// entries resolving outside dst, and symlink entries are rejected, and the total
// expanded size is bounded by DefaultMaxBytes. It has no allowlist, conflict
// policy, or rollback — those existed only to protect a live agent's private
// HOME, which does not apply to a clone's own fresh workspace. Rejections
// surface as *CodedError.
func UnpackPublicZip(dst string, zipBytes []byte) error {
	return unpackPublicZip(dst, zipBytes, DefaultMaxBytes)
}

func unpackPublicZip(dst string, zipBytes []byte, max int64) error {
	if int64(len(zipBytes)) > max {
		return coded("TOO_LARGE", "public.zip %d bytes exceeds %d", len(zipBytes), max)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return coded("CORRUPT", "open public.zip: %v", err)
	}
	remaining := max
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if zf.FileInfo().Mode()&os.ModeSymlink != 0 {
			return coded("PATH_ESCAPE", "zip entry %q is a symlink", zf.Name)
		}
		rel := path.Clean(zf.Name)
		if strings.HasPrefix(zf.Name, "/") || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			return coded("PATH_ESCAPE", "zip entry %q escapes", zf.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if target != dst && !strings.HasPrefix(target, dst+string(os.PathSeparator)) {
			return coded("PATH_ESCAPE", "%q resolves outside workspace", rel)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return coded("CORRUPT", "open zip member %q: %v", rel, err)
		}
		written, err := writePublicFile(target, rc, remaining)
		_ = rc.Close()
		if err != nil {
			return err
		}
		remaining -= written
	}
	return nil
}

func writePublicFile(target string, src io.Reader, max int64) (int64, error) {
	out, err := os.Create(target)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(out, io.LimitReader(src, max+1))
	closeErr := out.Close()
	if copyErr != nil {
		return n, copyErr
	}
	if closeErr != nil {
		return n, closeErr
	}
	if n > max {
		return n, coded("TOO_LARGE", "expanded public.zip content exceeds %d bytes", max)
	}
	return n, nil
}
