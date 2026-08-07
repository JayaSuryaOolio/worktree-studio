package skillasset

import (
	"os"
	"testing"
)

// TestEmbeddedCopyMatchesRealSkillFile guards against the exact drift
// this package's doc comment warns about: SKILL.md here is a hand-synced
// duplicate of the real .claude/skills/worktree-studio/SKILL.md (go:embed
// can't reach into a dot-prefixed directory), so nothing else would catch
// it silently going stale after a real skill-file edit.
func TestEmbeddedCopyMatchesRealSkillFile(t *testing.T) {
	real, err := os.ReadFile("../../.claude/skills/worktree-studio/SKILL.md")
	if err != nil {
		t.Fatalf("read real skill file: %v", err)
	}
	if string(real) != Content {
		t.Fatal("internal/skillasset/SKILL.md is out of sync with .claude/skills/worktree-studio/SKILL.md — copy the real file over this one (see this package's doc comment)")
	}
}
