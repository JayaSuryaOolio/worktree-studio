package files

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
}

// newTestRepo creates a throwaway git repo with one tracked file (in a
// subdirectory, to exercise tree nesting), one untracked-but-not-ignored
// file, and one gitignored file — the three cases ListTree needs to get
// right.
func newTestRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "test")
	run("config", "user.email", "test@example.com")

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "src/main.go", "README.md")
	run("commit", "-q", "-m", "initial commit")

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".gitignore")
	run("commit", "-q", "-m", "add gitignore")

	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("not yet added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("should never appear\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestResolvePathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"../outside.txt",
		"../../etc/passwd",
		"a/../../b",
		"/etc/passwd",
	}
	for _, rel := range cases {
		if _, err := ResolvePath(root, rel); !errors.Is(err, ErrPathEscapesWorktree) {
			t.Errorf("ResolvePath(%q) = %v, want ErrPathEscapesWorktree", rel, err)
		}
	}
}

func TestResolvePathAllowsWithin(t *testing.T) {
	root := t.TempDir()
	got, err := ResolvePath(root, "src/main.go")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	want := filepath.Join(root, "src", "main.go")
	if got != want {
		t.Errorf("ResolvePath = %q, want %q", got, want)
	}
}

func TestListTree(t *testing.T) {
	repo := newTestRepo(t)
	tree, err := ListTree(repo)
	if err != nil {
		t.Fatalf("ListTree: %v", err)
	}

	var names []string
	var srcChildren []string
	for _, n := range tree {
		names = append(names, n.Name)
		if n.Name == "src" {
			for _, c := range n.Children {
				srcChildren = append(srcChildren, c.Name)
			}
		}
	}

	wantTop := map[string]bool{"src": true, "README.md": true, ".gitignore": true, "untracked.txt": true}
	for name := range wantTop {
		found := false
		for _, n := range names {
			if n == name {
				found = true
			}
		}
		if !found {
			t.Errorf("ListTree top level missing %q, got %v", name, names)
		}
	}
	for _, n := range names {
		if n == "ignored.txt" {
			t.Errorf("ListTree included gitignored file: %v", names)
		}
	}

	if len(srcChildren) != 1 || srcChildren[0] != "main.go" {
		t.Errorf("src/ children = %v, want [main.go]", srcChildren)
	}
}

// TestListTreeCollapsesNodeModules verifies node_modules is shown as a
// folder entry but never expanded into — even when its contents are
// tracked/untracked-and-not-gitignored (not just the common gitignored
// case, which git ls-files would already exclude on its own).
func TestListTreeCollapsesNodeModules(t *testing.T) {
	repo := newTestRepo(t)

	if err := os.MkdirAll(filepath.Join(repo, "node_modules", "some-pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "node_modules", "some-pkg", "index.js"), []byte("module.exports = {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "node_modules", "top-level.js"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := ListTree(repo)
	if err != nil {
		t.Fatalf("ListTree: %v", err)
	}

	var nodeModules *FileNode
	for i := range tree {
		if tree[i].Name == "node_modules" {
			nodeModules = &tree[i]
		}
	}
	if nodeModules == nil {
		t.Fatalf("ListTree did not include a node_modules entry, got %+v", tree)
	}
	if nodeModules.Type != "dir" {
		t.Errorf("node_modules.Type = %q, want %q", nodeModules.Type, "dir")
	}
	if len(nodeModules.Children) != 0 {
		t.Errorf("expected node_modules to have no children, got %+v", nodeModules.Children)
	}
}

func TestReadWriteFile(t *testing.T) {
	repo := newTestRepo(t)

	content, err := ReadFile(repo, "README.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "hello\n" {
		t.Errorf("ReadFile = %q, want %q", content, "hello\n")
	}

	if err := WriteFile(repo, "README.md", []byte("updated\n")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	content, err = ReadFile(repo, "README.md")
	if err != nil {
		t.Fatalf("ReadFile after write: %v", err)
	}
	if string(content) != "updated\n" {
		t.Errorf("ReadFile after write = %q, want %q", content, "updated\n")
	}
}

func TestReadFileRejectsTraversal(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := ReadFile(repo, "../../etc/passwd"); !errors.Is(err, ErrPathEscapesWorktree) {
		t.Errorf("ReadFile traversal = %v, want ErrPathEscapesWorktree", err)
	}
}

func TestWriteFileRejectsTraversal(t *testing.T) {
	repo := newTestRepo(t)
	if err := WriteFile(repo, "../../tmp/evil.txt", []byte("x")); !errors.Is(err, ErrPathEscapesWorktree) {
		t.Errorf("WriteFile traversal = %v, want ErrPathEscapesWorktree", err)
	}
}

func TestReadFileRejectsTooLarge(t *testing.T) {
	repo := newTestRepo(t)
	big := make([]byte, MaxFileSize+1)
	if err := os.WriteFile(filepath.Join(repo, "big.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(repo, "big.bin"); !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("ReadFile big file = %v, want ErrFileTooLarge", err)
	}
}
