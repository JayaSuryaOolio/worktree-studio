package githook

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo also isolates $HOME to a temp dir, since Install's backup path
// (~/.worktree-studio/backups) must never touch the real developer machine
// running these tests.
func initRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	return dir
}

func TestInstallFreshRepo(t *testing.T) {
	repo := initRepo(t)

	installed, err := IsInstalled(repo)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("expected not installed before Install")
	}

	if err := Install(repo, "http://localhost:8787"); err != nil {
		t.Fatal(err)
	}

	installed, err = IsInstalled(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("expected installed after Install")
	}

	path, err := hookPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatal("expected hook script to be executable")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "#!/bin/sh\n") {
		t.Fatalf("expected shebang first line, got: %s", data)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	repo := initRepo(t)
	if err := Install(repo, "http://localhost:8787"); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "http://localhost:8787"); err != nil {
		t.Fatal(err)
	}

	path, err := hookPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), beginMarker) != 1 {
		t.Fatalf("expected exactly one marker block after two installs, got content:\n%s", data)
	}
}

func TestInstallPreservesExistingHook(t *testing.T) {
	repo := initRepo(t)
	path, err := hookPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "#!/bin/sh\necho 'someone elses hook'\n"
	if err := os.WriteFile(path, []byte(existing), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Install(repo, "http://localhost:8787"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "echo 'someone elses hook'") {
		t.Fatalf("expected pre-existing hook content to survive, got:\n%s", data)
	}
	if !strings.Contains(string(data), beginMarker) {
		t.Fatalf("expected our marker block to be present, got:\n%s", data)
	}

	// A backup of the pre-install content should have been made.
	home := os.Getenv("HOME")
	backupDir := filepath.Join(home, ".worktree-studio", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "post-checkout-") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a post-checkout-* backup in %s, got %v", backupDir, entries)
	}
}

func TestUninstallRemovesFileWhenWeOwnedIt(t *testing.T) {
	repo := initRepo(t)
	if err := Install(repo, "http://localhost:8787"); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(repo); err != nil {
		t.Fatal(err)
	}
	path, err := hookPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected hook file to be removed, stat err: %v", err)
	}
	installed, err := IsInstalled(repo)
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("expected not installed after Uninstall")
	}
}

func TestUninstallPreservesUnrelatedHookContent(t *testing.T) {
	repo := initRepo(t)
	path, err := hookPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'someone elses hook'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Install(repo, "http://localhost:8787"); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(repo); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), beginMarker) {
		t.Fatalf("expected our block to be gone, got:\n%s", data)
	}
	if !strings.Contains(string(data), "echo 'someone elses hook'") {
		t.Fatalf("expected unrelated hook content to survive, got:\n%s", data)
	}
}

func TestUninstallWithoutInstallIsNoop(t *testing.T) {
	repo := initRepo(t)
	if err := Uninstall(repo); err != nil {
		t.Fatalf("expected no error uninstalling a never-installed hook, got: %v", err)
	}
}

func TestSharedAcrossWorktrees(t *testing.T) {
	repo := initRepo(t)
	// A repo needs at least one commit before `git worktree add` will work.
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("add", "f")
	run("commit", "-q", "-m", "init")

	if err := Install(repo, "http://localhost:8787"); err != nil {
		t.Fatal(err)
	}

	worktreePath := filepath.Join(t.TempDir(), "wt")
	run("worktree", "add", "-b", "feature", worktreePath)

	installed, err := IsInstalled(worktreePath)
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("expected the hook installed on the main checkout to be visible from a worktree, since they share hooks/")
	}
}
