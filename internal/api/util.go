package api

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// newID returns a short random hex id (no external uuid dependency needed
// for this local tool's scale).
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var slugInvalidRe = regexp.MustCompile(`[^a-z0-9-]+`)
var slugDashesRe = regexp.MustCompile(`-+`)

// slugify lowercases and replaces anything that isn't [a-z0-9-] with a
// dash, collapsing repeats, for use as both a branch name and a directory
// name derived from a user-supplied worktree name.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugInvalidRe.ReplaceAllString(s, "-")
	s = slugDashesRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "worktree"
	}
	return s
}
