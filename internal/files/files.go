// Package files provides the file tree, read, and write operations behind
// the in-browser editor (see docs/editor-plan.md). The tree comes from
// `git ls-files` (tracked + untracked, INCLUDING gitignored — per direct
// user feedback, .gitignore controls what git tracks, not what's
// browsable here; a stray ignored file like .env shouldn't just vanish).
// The one deliberate exception is opaqueDirNames (node_modules, build):
// those still collapse to a single non-expandable folder entry regardless
// of ignore status, since dependency/build output directories are huge
// and not something anyone browses via this tree.
package files

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ErrPathEscapesWorktree is returned by ResolvePath when the given relative
// path would resolve outside the worktree root — the first place this app
// takes a filesystem path from the browser, so this check is load-bearing,
// not defensive dead code.
var ErrPathEscapesWorktree = errors.New("path escapes worktree root")

// ErrFileTooLarge is returned by ReadFile when the file exceeds MaxFileSize.
var ErrFileTooLarge = errors.New("file too large to open in the editor")

// MaxFileSize is the largest file ReadFile will return. Monaco/CodeMirror
// both become unresponsive well before this; files bigger than this should
// be opened in VS Code instead (see the "open in VS Code" escape hatch).
const MaxFileSize = 5 * 1024 * 1024 // 5MB

// ResolvePath joins relPath onto worktreePath and verifies the result is
// still lexically within worktreePath, rejecting any attempt to escape it
// (e.g. "../../etc/passwd" or an absolute path). Every read/write must go
// through this before touching the filesystem.
func ResolvePath(worktreePath, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", ErrPathEscapesWorktree
	}
	root, err := filepath.Abs(worktreePath)
	if err != nil {
		return "", fmt.Errorf("resolve worktree root: %w", err)
	}
	joined := filepath.Join(root, relPath)
	rel, err := filepath.Rel(root, joined)
	if err != nil {
		return "", ErrPathEscapesWorktree
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrPathEscapesWorktree
	}
	return joined, nil
}

// FileNode is one entry in the file tree, nested so the frontend can render
// directly without building the tree itself client-side.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"` // relative to the worktree root, forward-slash separated
	Type     string     `json:"type"` // "file" or "dir"
	Children []FileNode `json:"children,omitempty"`
}

// ListTree lists every tracked file (`git ls-files`) unioned with every
// untracked file regardless of gitignore status (`git ls-files --others`,
// deliberately without --exclude-standard — see this package's doc
// comment), then nests the flat path list into a directory tree.
func ListTree(worktreePath string) ([]FileNode, error) {
	tracked, err := lsFiles(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	untracked, err := lsFilesOthersIncludingIgnored(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("git ls-files --others: %w", err)
	}

	seen := make(map[string]bool, len(tracked)+len(untracked))
	var paths []string
	for _, p := range tracked {
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	for _, p := range untracked {
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	return buildTree(collapseOpaqueDirs(paths)), nil
}

// opaqueDirNames are directories the tree shows as a folder entry but never
// expands into or lists the contents of — dependency directories are huge,
// not something anyone browses via this editor's file tree, and walking/
// transmitting their full listing (potentially tens of thousands of
// entries) would be pure waste. The folder itself still appears (so its
// presence isn't a surprise), it's just not explorable.
var opaqueDirNames = map[string]bool{
	"node_modules": true,
	"build":        true,
}

// collapseOpaqueDirs truncates any path that passes through an opaque
// directory (see opaqueDirNames) at that directory, deduplicating the
// result — so "node_modules/react/index.js" and "node_modules/react/package.json"
// both collapse to just "node_modules", one entry instead of two, and
// buildTree never sees (or has to fetch) what's inside it.
func collapseOpaqueDirs(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		segments := strings.Split(p, "/")
		truncated := p
		for i, seg := range segments {
			if opaqueDirNames[seg] {
				truncated = strings.Join(segments[:i+1], "/")
				break
			}
		}
		if !seen[truncated] {
			seen[truncated] = true
			out = append(out, truncated)
		}
	}
	return out
}

func lsFiles(worktreePath string) ([]string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n"), nil
}

// lsFilesOthersIncludingIgnored deliberately omits --exclude-standard —
// see this package's doc comment for why gitignored files should still
// show up in the tree (opaqueDirNames is what keeps node_modules/build
// from dumping thousands of entries, independent of this).
func lsFilesOthersIncludingIgnored(worktreePath string) ([]string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "ls-files", "--others")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(string(out), "\n")
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// buildTree nests a flat list of forward-slash-separated relative paths
// into a directory tree, sorted directories-first then alphabetically
// within each directory (the conventional file-tree UI ordering).
func buildTree(paths []string) []FileNode {
	type dirNode struct {
		node     *FileNode
		children map[string]*dirNode
	}
	root := &dirNode{children: map[string]*dirNode{}}

	for _, p := range paths {
		parts := strings.Split(p, "/")
		cur := root
		var walked []string
		for i, part := range parts {
			walked = append(walked, part)
			isLeaf := i == len(parts)-1
			child, ok := cur.children[part]
			if !ok {
				n := FileNode{
					Name: part,
					Path: strings.Join(walked, "/"),
				}
				if isLeaf && !opaqueDirNames[part] {
					n.Type = "file"
				} else {
					// Either a real intermediate directory, or an opaque
					// dir (e.g. node_modules) that collapseOpaqueDirs left
					// as the last segment — either way it's a folder with
					// no children ever attached in the opaque case (no
					// paths beneath it survived collapsing).
					n.Type = "dir"
				}
				child = &dirNode{node: &n, children: map[string]*dirNode{}}
				cur.children[part] = child
			}
			cur = child
		}
	}

	var toSlice func(d *dirNode) []FileNode
	toSlice = func(d *dirNode) []FileNode {
		names := make([]string, 0, len(d.children))
		for name := range d.children {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			ci, cj := d.children[names[i]], d.children[names[j]]
			if (ci.node.Type == "dir") != (cj.node.Type == "dir") {
				return ci.node.Type == "dir" // dirs before files
			}
			return names[i] < names[j]
		})
		out := make([]FileNode, 0, len(names))
		for _, name := range names {
			child := d.children[name]
			n := *child.node
			if n.Type == "dir" {
				n.Children = toSlice(child)
			}
			out = append(out, n)
		}
		return out
	}

	return toSlice(root)
}

// ReadFile reads relPath (relative to worktreePath), rejecting paths that
// escape the worktree root or files over MaxFileSize.
func ReadFile(worktreePath, relPath string) ([]byte, error) {
	abs, err := ResolvePath(worktreePath, relPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", relPath)
	}
	if info.Size() > MaxFileSize {
		return nil, ErrFileTooLarge
	}
	return os.ReadFile(abs)
}

// WriteFile overwrites relPath (relative to worktreePath) with content.
// relPath must already exist as a file — this is an editor for existing
// tracked/untracked files, not a "create new file" flow, so there's no
// need to create parent directories that don't already exist.
func WriteFile(worktreePath, relPath string, content []byte) error {
	abs, err := ResolvePath(worktreePath, relPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", relPath)
	}
	return os.WriteFile(abs, content, info.Mode().Perm())
}
