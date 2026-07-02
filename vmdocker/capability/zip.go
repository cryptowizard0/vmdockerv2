package capability

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
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
