// Package spotlight wraps the standalone `spotlight` CLI
// (github.com/JayaSuryaOolio/spotlight, installed separately from
// worktree-studio) rather than reimplementing its sync logic. That tool
// already mirrors a git worktree's source files into its repo's root
// checkout — continuously, via fswatch+rsync — so path-bound tools and
// the root's already-installed dependencies/build output always reflect
// whichever worktree is "in focus". worktree-studio's job here is purely
// to start/stop/query it and surface that in the UI. See PLAN.md section 2
// for the full design (and the correction recorded there: this used to be
// designed backwards, copying deps INTO worktrees, before this existing
// tool was found).
package spotlight

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrRootDirty is returned by Start when the target repo's root checkout
// has uncommitted changes — the spotlight CLI itself refuses to start in
// that case, to avoid clobbering unsaved root work.
var ErrRootDirty = errors.New("root repo has uncommitted changes")

// ErrNotFound is returned when the spotlight binary can't be located.
var ErrNotFound = errors.New("spotlight CLI not found on PATH or in ~/.local/bin (see github.com/JayaSuryaOolio/spotlight)")

// BinaryPath locates the installed `spotlight` CLI: PATH first, falling
// back to the tool's documented default install location, since a
// worktree-studio server process may not inherit an interactive shell's
// PATH (e.g. launched from a non-shell parent).
func BinaryPath() (string, error) {
	if p, err := exec.LookPath("spotlight"); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".local", "bin", "spotlight")
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotFound
}

// MirrorStatus is one active spotlight mirror, as reported by `spotlight list`.
type MirrorStatus struct {
	Root     string `json:"root"`
	Worktree string `json:"worktree"`
	PID      int    `json:"pid"`
}

// Start begins mirroring worktreePath into its repo's root checkout,
// returning the resolved root path on success. Wraps `spotlight start` run
// with its working directory set to worktreePath (the CLI resolves the
// worktree/root pair from cwd via git itself).
func Start(worktreePath string) (root string, err error) {
	bin, err := BinaryPath()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, "start")
	cmd.Dir = worktreePath
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if strings.Contains(text, "uncommitted changes") {
			return "", fmt.Errorf("%w: %s", ErrRootDirty, text)
		}
		return "", fmt.Errorf("spotlight start: %w (%s)", err, text)
	}

	lines := strings.Split(text, "\n")
	root = strings.TrimSpace(lines[len(lines)-1])
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("spotlight start: unexpected output, couldn't find root path: %s", text)
	}
	return root, nil
}

// Stop tears down the mirror for the given repo root.
func Stop(root string) error {
	bin, err := BinaryPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "stop", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("spotlight stop: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns every currently active spotlight mirror.
func List() ([]MirrorStatus, error) {
	bin, err := BinaryPath()
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(bin, "list").Output()
	if err != nil {
		return nil, fmt.Errorf("spotlight list: %w", err)
	}

	text := strings.TrimSpace(string(out))
	if text == "" || text == "no spotlight instances running" {
		return []MirrorStatus{}, nil
	}

	var statuses []MirrorStatus
	for _, line := range strings.Split(text, "\n") {
		// Output is either "root  <-  worktree  (pid N)" (literal
		// double-space printf) or the same content re-flowed by `column
		// -t`, depending on which spotlight version is installed — both
		// tokenize to the same 5 fields via whitespace-splitting, so parse
		// on that rather than fixed column widths.
		fields := strings.Fields(line)
		if len(fields) != 5 || fields[1] != "<-" || fields[3] != "(pid" {
			continue // skip anything we don't recognize rather than erroring the whole list
		}
		pidStr := strings.TrimSuffix(fields[4], ")")
		var pid int
		_, _ = fmt.Sscanf(pidStr, "%d", &pid)
		statuses = append(statuses, MirrorStatus{Root: fields[0], Worktree: fields[2], PID: pid})
	}
	if statuses == nil {
		statuses = []MirrorStatus{}
	}
	return statuses, nil
}

// StatusForRoot returns the mirror status for a specific repo root, if any
// mirror is currently active for it. Compares resolved paths — the
// spotlight CLI normalizes root/worktree paths via its own absolute-path
// resolution, which on macOS diverges from a caller's raw path whenever
// /tmp, /var, etc. are themselves symlinks into /private (e.g. a t.TempDir()
// path is /var/folders/... but spotlight reports /private/var/folders/...
// for the exact same directory) — a naive string compare would wrongly
// report "no active mirror" for a real, running one.
func StatusForRoot(root string) (*MirrorStatus, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	wantRoot := resolveBestEffort(root)
	for _, s := range all {
		if resolveBestEffort(s.Root) == wantRoot {
			return &s, nil
		}
	}
	return nil, nil
}

// resolveBestEffort returns the symlink-resolved absolute path, or the
// original string if resolution fails (e.g. the path doesn't exist).
func resolveBestEffort(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}
