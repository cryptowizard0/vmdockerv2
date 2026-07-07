package capability

import "testing"

func TestCompilePublicPatterns_RequiresTilde(t *testing.T) {
	pats, warns := compilePublicPatterns([]string{
		"~/skills/*",
		"skills/*",      // no ~/ -> rejected
		"investment.md", // no ~/ -> rejected
		"~/investment.md",
	})
	if len(pats) != 2 {
		t.Fatalf("want 2 valid patterns, got %d (%+v)", len(pats), pats)
	}
	if len(warns) != 2 {
		t.Fatalf("want 2 warnings for the bare entries, got %d (%v)", len(warns), warns)
	}
}

func TestCompilePublicPatterns_RejectsDotDotAndEmpty(t *testing.T) {
	pats, warns := compilePublicPatterns([]string{"~/../etc", "~/", "~/ok.md"})
	if len(pats) != 1 || pats[0].Rel != "ok.md" {
		t.Fatalf("only ~/ok.md should survive, got %+v", pats)
	}
	if len(warns) != 2 {
		t.Fatalf("want 2 warnings (dotdot + empty), got %v", warns)
	}
}

func TestCompilePublicPatterns_RejectsHomeRootGlob(t *testing.T) {
	// Root-level globs are rejected outright (fail-closed), not honored.
	pats, warns := compilePublicPatterns([]string{"~/*", "~/*.md", "~/skills/*"})
	if len(pats) != 1 || pats[0].Rel != "skills/*" {
		t.Fatalf("only the scoped glob should survive, got %+v", pats)
	}
	if len(warns) != 2 {
		t.Fatalf("want a rejection warning per root-level glob, got %d (%v)", len(warns), warns)
	}
	// Root-level exact files (no glob) are still fine.
	pats2, warns2 := compilePublicPatterns([]string{"~/investment.md"})
	if len(pats2) != 1 || len(warns2) != 0 {
		t.Fatalf("root-level exact file must be honored without warning, pats=%+v warns=%v", pats2, warns2)
	}
}

func TestMatchesAnyPublic_RejectedRootGlobAllowsNothing(t *testing.T) {
	// Fail-closed: a profile whose only entry is a rejected root glob must
	// allow no paths at all (critical for the import allowlist).
	pats, _ := compilePublicPatterns([]string{"~/*"})
	if matchesAnyPublic("skills/a.md", pats) || matchesAnyPublic(".ssh/id_rsa", pats) {
		t.Fatal("rejected root glob must allow nothing")
	}
}

func TestPublicPattern_Matching(t *testing.T) {
	pats, _ := compilePublicPatterns([]string{
		"~/skills/*",
		"~/persona/*.md",
		"~/investment.md",
	})
	cases := []struct {
		rel  string
		want bool
	}{
		{"skills/a.txt", true},                // * matches a file directly under skills
		{"skills/code-review/SKILL.md", true}, // * is recursive (crosses /)
		{"persona/bio.md", true},
		{"persona/sub/deep.md", true}, // *.md recursive under persona
		{"persona/bio.txt", false},    // not .md
		{"investment.md", true},       // exact file
		{"investment.md.bak", false},  // anchored: no trailing chars
		{"other/x", false},            // not covered
		{"skills", false},             // the dir itself is not a file entry
	}
	for _, c := range cases {
		if got := matchesAnyPublic(c.rel, pats); got != c.want {
			t.Errorf("match(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

func TestLiteralBaseDir(t *testing.T) {
	cases := map[string]string{
		"skills/*.md":    "skills",
		"skills/*":       "skills",
		"a/b/c/*":        "a/b/c",
		"*.md":           "",
		"persona/x/*.md": "persona/x",
	}
	for in, want := range cases {
		if got := literalBaseDir(in); got != want {
			t.Errorf("literalBaseDir(%q) = %q, want %q", in, got, want)
		}
	}
}
