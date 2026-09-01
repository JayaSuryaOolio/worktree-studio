package term

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"worktree-studio/internal/store"
)

// OrphanTmuxSession is a live tmux session in worktree-studio's own
// namespace (the "wts-" prefix, see tmuxNamePrefix) that has no matching
// terminal_sessions row — e.g. one left behind by a test run that exited
// abnormally, or one created by hand outside the app. This is the mirror
// image of what Reconcile handles (a DB row with no live tmux session):
// Reconcile never touches tmux, and this never touches the store.
type OrphanTmuxSession struct {
	Name         string
	LastActivity time.Time
	// Protected is true when LastActivity falls inside the safety window
	// passed to FindOrphanTmuxSessions/KillOrphanTmuxSessions. See
	// KillOrphanTmuxSessions's own comment for why a Protected session can
	// never be killed through this package.
	Protected bool
}

// sessionActivity returns tmux's own #{session_activity} for a session: the
// time of the last activity (any pane output, not just a keystroke) in it —
// so a `claude` session producing output on its own counts as "used" the
// same as a person typing into a plain shell would.
func sessionActivity(tmuxSessionName string) (time.Time, error) {
	out, err := TmuxCmd("display-message", "-p", "-t", tmuxSessionName, "#{session_activity}").Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("tmux display-message session_activity: %w", err)
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse session_activity %q: %w", out, err)
	}
	return time.Unix(secs, 0), nil
}

// FindOrphanTmuxSessions lists every live tmux session in worktree-studio's
// own namespace that has no matching terminal_sessions row, each flagged
// Protected if it's had activity within minAge of now. Read-only: never
// kills or modifies anything, in tmux or the store.
//
// A session whose activity can't be read (e.g. a race where it died
// between listing and querying) is reported as Protected rather than
// dropped or assumed safe — a caller can't prove it's safe to touch, so it
// shouldn't touch it.
func FindOrphanTmuxSessions(st *store.Store, minAge time.Duration) ([]OrphanTmuxSession, error) {
	live, err := ListLiveTmuxSessionNames()
	if err != nil {
		return nil, err
	}
	all, err := st.ListAllTerminalSessions()
	if err != nil {
		return nil, fmt.Errorf("list terminal sessions: %w", err)
	}
	known := make(map[string]bool, len(all))
	for _, ts := range all {
		known[ts.TmuxSessionName] = true
	}

	cutoff := time.Now().Add(-minAge)
	var orphans []OrphanTmuxSession
	for name := range live {
		if !strings.HasPrefix(name, tmuxNamePrefix) || known[name] {
			continue
		}
		activity, err := sessionActivity(name)
		if err != nil {
			orphans = append(orphans, OrphanTmuxSession{Name: name, Protected: true})
			continue
		}
		orphans = append(orphans, OrphanTmuxSession{
			Name:         name,
			LastActivity: activity,
			Protected:    activity.After(cutoff),
		})
	}
	return orphans, nil
}

// KillOrphanTmuxSessions kills every orphan tmux session from
// FindOrphanTmuxSessions that ISN'T Protected, and leaves every Protected
// one strictly alone.
//
// There is deliberately no override for this from outside the package — no
// "--force", nothing. The whole reason this function exists is that a
// future cleanup (run by a person, a script, or an agent moving fast)
// cannot accidentally kill something that was actually used recently just
// because it looked orphaned by some other signal (e.g. "unattached",
// "looks like a test leak") — that mismatch is exactly what caused a real
// cleanup to sweep up live sessions instead of just the test-leaked ones it
// meant to target. A caller that genuinely wants a shorter safety window
// can pass a smaller minAge — a visible, deliberate choice at the call
// site, not a silent bypass of the check itself.
func KillOrphanTmuxSessions(st *store.Store, minAge time.Duration) (killed, protected []string, err error) {
	orphans, err := FindOrphanTmuxSessions(st, minAge)
	if err != nil {
		return nil, nil, err
	}
	for _, o := range orphans {
		if o.Protected {
			protected = append(protected, o.Name)
			continue
		}
		if kerr := TmuxCmd("kill-session", "-t", o.Name).Run(); kerr != nil {
			continue // already gone, or a transient failure — not fatal to the sweep
		}
		killed = append(killed, o.Name)
	}
	return killed, protected, nil
}
