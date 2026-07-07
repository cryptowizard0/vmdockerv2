package capability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectPublic_FilesAndSha(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills", "code-review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "code-review", "SKILL.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := CollectPublic(home, []string{"~/skills/*"})
	if err != nil {
		t.Fatalf("CollectPublic: %v", err)
	}
	if len(col.Entries) != 1 || col.Entries[0].Path != "skills/code-review/SKILL.md" {
		t.Fatalf("entries = %+v", col.Entries)
	}
	if col.TotalBytes != 5 {
		t.Fatalf("TotalBytes = %d, want 5", col.TotalBytes)
	}
	if col.Entries[0].SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("sha = %s", col.Entries[0].SHA256)
	}
}

func TestCollectPublic_RejectsSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "skills", "leak")); err != nil {
		t.Skip("symlinks unsupported")
	}
	_, err := CollectPublic(home, []string{"~/skills/*"})
	if err == nil {
		t.Fatal("expected PATH_ESCAPE error for symlink pointing outside HOME")
	}
}

func TestCollectPublic_SingleFile(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "s.md"), []byte("S"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "note.md"), []byte("NN"), 0o644); err != nil {
		t.Fatal(err)
	}

	col, err := CollectPublic(home, []string{"~/skills/*", "~/note.md"})
	if err != nil {
		t.Fatalf("CollectPublic: %v", err)
	}
	paths := map[string]bool{}
	for _, e := range col.Entries {
		paths[e.Path] = true
	}
	if !paths["note.md"] || !paths["skills/s.md"] {
		t.Fatalf("entries = %+v, want note.md + skills/s.md", col.Entries)
	}
}

func TestCollectPublic_DirAsExactFileWarns(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	// "~/skills" has no glob, so it is treated as an exact file; a directory
	// there is a usage error and must warn (not silently export the tree).
	col, err := CollectPublic(home, []string{"~/skills"})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Entries) != 0 || len(col.Warnings) == 0 {
		t.Fatalf("expected directory warning + no entries, got entries=%v warnings=%v", col.Entries, col.Warnings)
	}
}

func TestCollectPublic_GlobFilterAndRecursion(t *testing.T) {
	home := t.TempDir()
	mustWrite := func(rel, data string) {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("persona/bio.md", "a")
	mustWrite("persona/sub/deep.md", "b") // recursive *.md
	mustWrite("persona/notes.txt", "c")   // excluded by *.md
	mustWrite("investment.md", "d")

	col, err := CollectPublic(home, []string{"~/persona/*.md", "~/investment.md"})
	if err != nil {
		t.Fatalf("CollectPublic: %v", err)
	}
	got := map[string]bool{}
	for _, e := range col.Entries {
		got[e.Path] = true
	}
	if !got["persona/bio.md"] || !got["persona/sub/deep.md"] || !got["investment.md"] {
		t.Fatalf("missing expected entries: %+v", col.Entries)
	}
	if got["persona/notes.txt"] {
		t.Fatalf("notes.txt must be excluded by *.md: %+v", col.Entries)
	}
}
