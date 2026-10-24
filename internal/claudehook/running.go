package claudehook

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RunningSession is one entry from Claude Code's live-session registry,
// ~/.claude/sessions/<pid>.json, which it writes for every running session
// and removes on a clean exit. That file is Claude Code's own internal
// format, not a documented API: only the fields below are read, unknown
// ones are ignored, and an unreadable or malformed file is skipped. If the
// format changes or the directory disappears, the worktree's session list
// falls back to transcripts only (see ListSessionsForCwd).
type RunningSession struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Kind      string `json:"kind"`   // "interactive" or "bg" (`claude --bg`)
	Status    string `json:"status"` // "busy" or "idle"
	Name      string `json:"name"`
	// Tmux is "<session>:<window>.<pane>" when the session runs inside tmux.
	Tmux string `json:"tmux"`
	// JobID is the short id `claude attach` takes; set for bg sessions.
	JobID     string `json:"jobId"`
	UpdatedAt int64  `json:"updatedAt"` // unix ms
}

// ShortID is the id `claude attach <id>` accepts: the jobId, which is the
// first 8 characters of the session id (the full session id is rejected).
func (r RunningSession) ShortID() string {
	if r.JobID != "" {
		return r.JobID
	}
	if len(r.SessionID) > 8 {
		return r.SessionID[:8]
	}
	return r.SessionID
}

// SessionRegistryDir is ~/.claude/sessions.
func SessionRegistryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "sessions"), nil
}

// PidAlive reports whether a process with pid exists (EPERM still means it
// exists, just not ours to signal).
func PidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// ListRunning reads every registry file in dir and returns the sessions
// whose cwd is root or inside it and whose pid alive() says still exists.
// Registry files outlive a crashed process, so stale ones are common.
// ponytail: a reused pid would pass alive(); compare the file's procStart
// with the process start time if that ever shows up in practice.
func ListRunning(dir, root string, alive func(int) bool) []RunningSession {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []RunningSession
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var r RunningSession
		if json.Unmarshal(data, &r) != nil || r.SessionID == "" || r.PID <= 0 {
			continue
		}
		if !inside(r.Cwd, root) || !alive(r.PID) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func inside(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// TmuxSessionName is the tmux session part of a registry Tmux field
// ("wts-abc:@0.%0" -> "wts-abc").
func TmuxSessionName(tmux string) string {
	name, _, _ := strings.Cut(tmux, ":")
	return name
}

// ProcessParents is a pid -> parent pid table of every process, from one
// `ps` call.
func ProcessParents() map[int]int {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	parents := map[int]int{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			parents[pid] = ppid
		}
	}
	return parents
}

// AncestorIn walks pid's parent chain (pid itself included) through parents
// and returns the owner of the first pid found in owners, or "". Used to
// find which ptyd shell (owners: shell pid -> terminal id) a claude process
// runs under.
func AncestorIn(pid int, owners map[int]string, parents map[int]int) string {
	for i := 0; pid > 1 && i < 64; i++ { // bounded in case of a bogus cycle
		if id, ok := owners[pid]; ok {
			return id
		}
		pid = parents[pid]
	}
	return ""
}

// MergeRunning marks transcripts in sessions that are running and appends
// running sessions that have no transcript there (a brand-new session, or
// one started in a subdirectory, whose transcript lives under that
// subdirectory's project). terminalFor maps a running session to the id of
// a worktree-studio terminal hosting it, or "". Running sessions sort first.
func MergeRunning(sessions []SessionSummary, running []RunningSession, terminalFor func(RunningSession) string) []SessionSummary {
	idx := map[string]int{}
	for i, s := range sessions {
		idx[s.SessionID] = i
	}
	for _, r := range running {
		i, ok := idx[r.SessionID]
		if !ok {
			preview, _ := SessionTitle(r.SessionID)
			sessions = append(sessions, SessionSummary{
				SessionID: r.SessionID,
				Preview:   preview,
				UpdatedAt: time.UnixMilli(r.UpdatedAt).UTC().Format(time.RFC3339),
			})
			i = len(sessions) - 1
		}
		s := &sessions[i]
		s.Running, s.Kind, s.Status, s.Name, s.PID = true, r.Kind, r.Status, r.Name, r.PID
		s.ShortID = r.ShortID()
		s.TerminalID = terminalFor(r)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].Running != sessions[j].Running {
			return sessions[i].Running
		}
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})
	return sessions
}
