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

// CollectPublic collects the files selected by the profile's `public` entries.
// Each entry is a "~/"-prefixed HOME-relative glob (see publicPattern); an entry
// with no glob metacharacter selects one exact file, while a glob (e.g.
// "~/skills/*") selects every matching file, recursively. Malformed entries and
// entries that match nothing produce warnings and are skipped.
func CollectPublic(home string, public []string) (Collection, error) {
	col := Collection{Public: append([]string(nil), public...)}
	pats, warnings := compilePublicPatterns(public)
	col.Warnings = append(col.Warnings, warnings...)

	seen := make(map[string]bool)
	for _, p := range pats {
		matched, err := collectPattern(home, p, seen, &col)
		if err != nil {
			return Collection{}, err
		}
		if !matched {
			col.Warnings = append(col.Warnings, fmt.Sprintf("public entry %q matched no files", p.Raw))
		}
	}
	sort.Slice(col.Entries, func(i, j int) bool { return col.Entries[i].Path < col.Entries[j].Path })
	return col, nil
}

// collectPattern collects the files matched by one pattern, deduping against
// seen (a file may be selected by multiple patterns). It reports whether the
// pattern matched at least one file.
func collectPattern(home string, p publicPattern, seen map[string]bool, col *Collection) (bool, error) {
	// Exact file (no glob): stat directly; a directory here is a usage error.
	if !p.IsGlob {
		abs := filepath.Join(home, p.Rel)
		info, err := os.Stat(abs)
		if err != nil {
			return false, nil
		}
		if info.IsDir() {
			col.Warnings = append(col.Warnings, fmt.Sprintf("public entry %q is a directory; use %q to include its contents", p.Raw, "~/"+p.Rel+"/*"))
			return false, nil
		}
		if seen[p.Rel] {
			return true, nil
		}
		seen[p.Rel] = true
		return true, collectOne(home, abs, col)
	}

	// Glob: walk from the literal base directory and match each file.
	root := filepath.Join(home, literalBaseDir(p.Rel))
	matched := false
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(home, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if !p.re.MatchString(relSlash) {
			return nil
		}
		matched = true
		if seen[relSlash] {
			return nil
		}
		seen[relSlash] = true
		return collectOne(home, path, col)
	})
	if err != nil {
		return matched, err
	}
	return matched, nil
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
