// Package skillasset embeds a copy of this project's own
// .claude/skills/worktree-studio/SKILL.md so it can be installed into the
// user's global ~/.claude/skills/ directory (making it available from any
// project, not just when working inside this repo's own checkout) without
// depending on the running binary knowing where its source checkout lives
// — the binary might be built and copied elsewhere, or invoked from a
// different working directory.
//
// This IS a duplicate of the real file, kept in sync by hand — go:embed
// can't reach across into .claude/ (Go's toolchain ignores dot-prefixed
// directories as package roots, so a source file can't live there), and
// this project has no codegen step (see internal/audit/events.go and
// web/src/auditEvents.ts for the same tradeoff made elsewhere). Drift is
// caught automatically, not just by convention: skillasset_test.go
// compares this embedded copy byte-for-byte against the real file at test
// time and fails if they differ, so `go test ./...` — already part of
// this project's standing verification bar — catches a forgotten update
// immediately rather than silently shipping a stale skill.
package skillasset

import _ "embed"

//go:embed SKILL.md
var Content string
