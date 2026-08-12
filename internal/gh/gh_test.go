package gh

import (
	"os/exec"
	"testing"
)

func requireGhCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not found on PATH")
	}
	// `gh pr view` needs to actually reach GitHub's API to tell "no PR"
	// apart from "not authenticated" — skip in an environment with no
	// configured auth rather than failing on something unrelated to this
	// package's own logic.
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		t.Skip("gh CLI not authenticated")
	}
}

// TestPRForBranchNoPR verifies the (nil, nil) "no PR, not an error" path
// against this real repo's own main branch — confirmed by hand to have no
// open pull request, and exercising the real `gh` CLI's actual stderr
// wording rather than a guessed/mocked one.
func TestPRForBranchNoPR(t *testing.T) {
	requireGhCLI(t)

	pr, err := PRForBranch(".", "main")
	if err != nil {
		t.Fatalf("PRForBranch: %v", err)
	}
	if pr != nil {
		t.Errorf("PRForBranch(\"main\") = %+v, want nil (no PR expected for this branch)", pr)
	}
}

// TestPRForBranchInvalidRepo verifies a real error (not silently treated
// as "no PR") when repoPath isn't even a git repo at all.
func TestPRForBranchInvalidRepo(t *testing.T) {
	requireGhCLI(t)

	_, err := PRForBranch(t.TempDir(), "main")
	if err == nil {
		t.Fatal("PRForBranch against a non-git directory: want an error, got nil")
	}
}
