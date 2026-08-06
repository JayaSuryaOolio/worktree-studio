package spotlight

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func requireSpotlight(t *testing.T) string {
	t.Helper()
	bin, err := BinaryPath()
	if err != nil {
		t.Skipf("spotlight CLI not available: %v", err)
	}
	if _, err := exec.LookPath("fswatch"); err != nil {
		t.Skip("fswatch not found on PATH (required by the spotlight CLI itself)")
	}
	return bin
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return string(out)
}

// newTestRepoWithWorktree creates a throwaway git repo (root) with one
// commit and a worktree branched off it, returning both paths.
func newTestRepoWithWorktree(t *testing.T) (root, worktree string) {
	t.Helper()
	root = t.TempDir()
	runGit(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "initial")

	// Something that's already in root but gitignored, to prove --delete
	// doesn't wipe it out via the flattened-exclude-file fix.
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "marker"), []byte("do-not-delete\n"), 0o644); err != nil {
		t.Fatalf("write node_modules/marker: %v", err)
	}

	worktree = t.TempDir() + "-wt"
	runGit(t, root, "worktree", "add", "-b", "feature", worktree)
	return root, worktree
}

func TestStartMirrorsWorktreeIntoRoot(t *testing.T) {
	requireSpotlight(t)
	root, worktree := newTestRepoWithWorktree(t)
	t.Cleanup(func() { _ = Stop(root) })

	if err := os.WriteFile(filepath.Join(worktree, "hello.txt"), []byte("from worktree\n"), 0o644); err != nil {
		t.Fatalf("write hello.txt: %v", err)
	}

	gotRoot, err := Start(worktree)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if gotRoot != root {
		// root paths may differ by symlink resolution (e.g. /tmp vs /private/tmp on macOS)
		gotResolved, _ := filepath.EvalSymlinks(gotRoot)
		wantResolved, _ := filepath.EvalSymlinks(root)
		if gotResolved != wantResolved {
			t.Fatalf("Start returned root %s, want %s", gotRoot, root)
		}
	}

	data, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil {
		t.Fatalf("expected hello.txt to be mirrored into root: %v", err)
	}
	if string(data) != "from worktree\n" {
		t.Fatalf("unexpected mirrored content: %q", data)
	}

	// The gitignored node_modules/marker in root must survive the initial
	// sync's --delete, proving the exclude-file safety fix is effective.
	if _, err := os.Stat(filepath.Join(root, "node_modules", "marker")); err != nil {
		t.Fatalf("expected gitignored root content to survive sync: %v", err)
	}

	status, err := StatusForRoot(root)
	if err != nil {
		t.Fatalf("StatusForRoot: %v", err)
	}
	if status == nil {
		t.Fatal("expected an active mirror for root, got none")
	}
	if status.Worktree != worktree {
		gotResolved, _ := filepath.EvalSymlinks(status.Worktree)
		wantResolved, _ := filepath.EvalSymlinks(worktree)
		if gotResolved != wantResolved {
			t.Fatalf("status.Worktree = %s, want %s", status.Worktree, worktree)
		}
	}
}

func TestLiveEditPropagates(t *testing.T) {
	requireSpotlight(t)
	root, worktree := newTestRepoWithWorktree(t)
	t.Cleanup(func() { _ = Stop(root) })

	if _, err := Start(worktree); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// fswatch's watcher takes a moment to actually start watching after the
	// CLI backgrounds it; give it a beat before relying on it to notice a
	// write (matches this project's other pty/tmux tests' same allowance
	// for external-process startup lag).
	time.Sleep(500 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(worktree, "live.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write live.txt: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(root, "live.txt")); err == nil && string(data) == "v1\n" {
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("live edit in worktree was not mirrored into root within 5s")
}

func TestStopRestoresCleanRoot(t *testing.T) {
	requireSpotlight(t)
	root, worktree := newTestRepoWithWorktree(t)

	if err := os.WriteFile(filepath.Join(worktree, "hello.txt"), []byte("from worktree\n"), 0o644); err != nil {
		t.Fatalf("write hello.txt: %v", err)
	}
	if _, err := Start(worktree); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := Stop(root); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	status := runGit(t, root, "status", "--porcelain")
	if status != "" {
		t.Fatalf("expected root to be restored to a clean state after Stop, got status: %q", status)
	}
	if _, err := os.Stat(filepath.Join(root, "hello.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected hello.txt (never committed to root) to be gone after Stop, stat err = %v", err)
	}

	remaining, err := StatusForRoot(root)
	if err != nil {
		t.Fatalf("StatusForRoot after Stop: %v", err)
	}
	if remaining != nil {
		t.Fatalf("expected no active mirror after Stop, got %+v", remaining)
	}
}

func TestStartRefusesDirtyRoot(t *testing.T) {
	requireSpotlight(t)
	root, worktree := newTestRepoWithWorktree(t)

	if err := os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt in root: %v", err)
	}

	_, err := Start(worktree)
	if err == nil {
		t.Fatal("expected Start to refuse a dirty root, got nil error")
	}
	if !errors.Is(err, ErrRootDirty) {
		t.Fatalf("expected an ErrRootDirty-wrapping error, got: %v", err)
	}
}

func TestStartSwitchingWorktreeStopsPrevious(t *testing.T) {
	requireSpotlight(t)
	root, worktree1 := newTestRepoWithWorktree(t)
	t.Cleanup(func() { _ = Stop(root) })

	worktree2 := t.TempDir() + "-wt2"
	runGit(t, root, "worktree", "add", "-b", "feature2", worktree2)

	if err := os.WriteFile(filepath.Join(worktree1, "from-one.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(worktree1); err != nil {
		t.Fatalf("Start worktree1: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktree2, "from-two.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(worktree2); err != nil {
		t.Fatalf("Start worktree2: %v", err)
	}

	status, err := StatusForRoot(root)
	if err != nil {
		t.Fatalf("StatusForRoot: %v", err)
	}
	if status == nil {
		t.Fatal("expected an active mirror after starting worktree2")
	}
	gotResolved, _ := filepath.EvalSymlinks(status.Worktree)
	wantResolved, _ := filepath.EvalSymlinks(worktree2)
	if gotResolved != wantResolved {
		t.Fatalf("expected the active mirror to now be worktree2, got %s", status.Worktree)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
