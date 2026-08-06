package api

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Feature Branch", "feature-branch"},
		{"  spaced out  ", "spaced-out"},
		{"UPPER_CASE", "upper-case"},
		{"a---b", "a-b"},
		{"-leading-and-trailing-", "leading-and-trailing"},
		{"", "worktree"},
		{"   ", "worktree"},
		{"!!!", "worktree"},
		// Flag-injection attempt: leading dashes must not survive, since a
		// worktree/branch name starting with "-" could otherwise be
		// interpreted as a git command-line flag.
		{"--upload-pack=/bin/sh", "upload-pack-bin-sh"},
		{"already-valid-slug", "already-valid-slug"},
		{"emoji🎉name", "emoji-name"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSlugifyNeverStartsWithDash(t *testing.T) {
	inputs := []string{"-x", "--force", "---", "- -"}
	for _, in := range inputs {
		got := slugify(in)
		if len(got) > 0 && got[0] == '-' {
			t.Errorf("slugify(%q) = %q starts with a dash", in, got)
		}
	}
}

func TestNewIDIsUniqueAndHex(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newID()
		if len(id) != 16 { // 8 bytes hex-encoded
			t.Fatalf("newID() = %q, want length 16", id)
		}
		if seen[id] {
			t.Fatalf("newID() produced duplicate: %q", id)
		}
		seen[id] = true
	}
}
