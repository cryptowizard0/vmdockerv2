package capability

import (
	"fmt"
	"regexp"
	"strings"
)

// publicPattern is one compiled profile `public` entry.
//
// Format (hard requirement): every entry must start with "~/" and denotes a
// HOME-relative path glob. Glob metacharacters: '*' matches any run of
// characters INCLUDING '/', so it is recursive/cross-directory; '?' matches a
// single character. Everything else is literal.
//
//	~/investment.md   exact single file
//	~/skills/*        every file under skills/ (recursively)
//	~/skills/*.md     every *.md under skills/ (recursively)
type publicPattern struct {
	Raw    string         // original entry, e.g. "~/skills/*.md"
	Rel    string         // HOME-relative glob, e.g. "skills/*.md"
	IsGlob bool           // whether Rel contains a glob metacharacter
	re     *regexp.Regexp // anchored regexp over slash-separated rel paths
}

// compilePublicPatterns parses profile `public` entries. Each entry MUST start
// with "~/" (hard switch: bare HOME-relative entries are rejected). Malformed
// entries are collected as warnings and skipped rather than failing the whole
// operation — matching stays fail-closed (a skipped entry allows nothing).
func compilePublicPatterns(public []string) ([]publicPattern, []string) {
	var pats []publicPattern
	var warnings []string
	for _, entry := range public {
		e := strings.TrimSpace(entry)
		if !strings.HasPrefix(e, "~/") {
			warnings = append(warnings, fmt.Sprintf("public entry %q must start with \"~/\"", entry))
			continue
		}
		rel := strings.TrimLeft(strings.TrimPrefix(e, "~/"), "/")
		if rel == "" {
			warnings = append(warnings, fmt.Sprintf("public entry %q has no path after \"~/\"", entry))
			continue
		}
		if hasDotDotSegment(rel) {
			warnings = append(warnings, fmt.Sprintf("public entry %q must not contain \"..\"", entry))
			continue
		}
		isGlob := strings.ContainsAny(rel, "*?")
		if isGlob && literalBaseDir(rel) == "" {
			// A glob in the first segment (e.g. "~/*", "~/*.md") matches from
			// HOME root recursively, sweeping in hidden/dot files (.ssh,
			// .bashrc, ...). Reject it — too broad to be intentional. The entry
			// is skipped (fail-closed: it allows nothing); scope it to a
			// subdirectory instead.
			warnings = append(warnings, fmt.Sprintf("public entry %q rejected: a HOME-root-level glob matches all of HOME including hidden files; scope it to a subdirectory (e.g. \"~/skills/*\")", entry))
			continue
		}
		re, err := globToRegexp(rel)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("public entry %q is not a valid pattern: %v", entry, err))
			continue
		}
		pats = append(pats, publicPattern{
			Raw:    entry,
			Rel:    rel,
			IsGlob: isGlob,
			re:     re,
		})
	}
	return pats, warnings
}

func hasDotDotSegment(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// globToRegexp compiles a HOME-relative glob into an anchored regexp. '*' -> ".*"
// (greedy, crosses '/'), '?' -> "." ; every other character is matched
// literally.
func globToRegexp(glob string) (*regexp.Regexp, error) {
	quoted := regexp.QuoteMeta(glob)
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `.`)
	return regexp.Compile("^" + quoted + "$")
}

// literalBaseDir returns the leading literal directory of a glob's Rel, up to
// (but excluding) the first segment containing a glob metacharacter. It bounds
// the filesystem walk during collection. "skills/*.md" -> "skills";
// "*.md" -> "" (walk from HOME).
func literalBaseDir(rel string) string {
	var lit []string
	for _, seg := range strings.Split(rel, "/") {
		if strings.ContainsAny(seg, "*?") {
			break
		}
		lit = append(lit, seg)
	}
	return strings.Join(lit, "/")
}

// matchesAnyPublic reports whether the HOME-relative, slash-separated path rel
// matches any compiled pattern.
func matchesAnyPublic(rel string, pats []publicPattern) bool {
	for _, p := range pats {
		if p.re.MatchString(rel) {
			return true
		}
	}
	return false
}
