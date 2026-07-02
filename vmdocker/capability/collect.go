// Package capability implements runtime Export/Import/Preview of an agent's
// public entries. All paths are rooted at HOME (/home/hymx after P3).
package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PublicEntry is one file selected for export.
type PublicEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Collection is the result of scanning the profile's public entries.
type Collection struct {
	Public     []string      `json:"public"`
	Entries    []PublicEntry `json:"entries"`
	TotalBytes int64         `json:"total_bytes"`
	Warnings   []string      `json:"warnings"`
}

func resolveWithinHome(home, abs string) (string, error) {
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	homeReal, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", err
	}
	if real != homeReal && !strings.HasPrefix(real, homeReal+string(os.PathSeparator)) {
		return "", fmt.Errorf("PATH_ESCAPE: %s resolves outside HOME", abs)
	}
	return real, nil
}

// CollectPublic collects each public entry under home: a directory when the
// entry ends with "/", else a single file. Type mismatches warn and skip.
func CollectPublic(home string, public []string) (Collection, error) {
	col := Collection{Public: append([]string(nil), public...)}
	for _, entry := range public {
		wantDir := strings.HasSuffix(entry, "/")
		root := filepath.Join(home, strings.TrimRight(entry, "/"))
		info, err := os.Stat(root)
		if err != nil {
			col.Warnings = append(col.Warnings, fmt.Sprintf("public entry %q missing", entry))
			continue
		}
		if info.IsDir() != wantDir {
			col.Warnings = append(col.Warnings, fmt.Sprintf("public entry %q does not match its trailing-slash marker", entry))
			continue
		}
		if !wantDir {
			if err := collectOne(home, root, &col); err != nil {
				return Collection{}, err
			}
			continue
		}
		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			return collectOne(home, path, &col)
		})
		if err != nil {
			return Collection{}, err
		}
	}
	sort.Slice(col.Entries, func(i, j int) bool { return col.Entries[i].Path < col.Entries[j].Path })
	return col, nil
}

func collectOne(home, path string, col *Collection) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if _, err := resolveWithinHome(home, path); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	col.Entries = append(col.Entries, PublicEntry{
		Path:   filepath.ToSlash(rel),
		Size:   int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]),
	})
	col.TotalBytes += int64(len(data))
	return nil
}
