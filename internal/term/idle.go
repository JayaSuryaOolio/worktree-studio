package term

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"worktree-studio/internal/ptyd"
)

// IdleReapSettingKey is the settings-table key for how many days a ptyd
// terminal may sit idle before the reaper closes it; unset or 0 is off.
const IdleReapSettingKey = "ptyd_idle_reap_days"

// IdleReapDays is the idle reaper setting; 0 means off.
func (m *Manager) IdleReapDays() int {
	v, _ := m.Store.GetSetting(IdleReapSettingKey)
	n, _ := strconv.Atoi(v)
	return max(n, 0)
}

func (m *Manager) SetIdleReapDays(days int) error {
	if days < 0 {
		return fmt.Errorf("idle reap days must be 0 (off) or more, got %d", days)
	}
	return m.Store.SetSetting(IdleReapSettingKey, strconv.Itoa(days))
}

// IdleTerminal is a ptyd terminal with no attached client and no output for
// the reaper's window. Busy names the program in its foreground when that
// isn't the shell itself (a claude, a dev server, vim): those are never
// reaped, only reported.
type IdleTerminal struct {
	TerminalID   string    `json:"terminal_id"`
	WorktreeID   string    `json:"worktree_id"`
	TabLabel     string    `json:"tab_label"`
	LastActivity time.Time `json:"last_activity"`
	Busy         string    `json:"busy,omitempty"`
}

// idleState is the reaper's rule for one session: idle means nobody is
// attached and nothing was printed for minIdle; reapable additionally needs
// the shell itself in the foreground of its tty (fgPgrp == its pid), so a
// quiet but running program is never killed. fgPgrp 0 (unknown) is busy.
func idleState(s ptyd.Session, fgPgrp int, minIdle time.Duration, now time.Time) (idle, reapable bool) {
	idle = s.Clients == 0 && now.Sub(s.LastActivity) >= minIdle
	return idle, idle && fgPgrp == s.Pid
}

// foregroundPgrp is the foreground process group of pid's controlling tty,
// or 0 if unknown.
func foregroundPgrp(pid int) int {
	out, err := exec.Command("ps", "-o", "tpgid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	fg, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return max(fg, 0)
}

// FindIdle lists the ptyd terminals idle for at least days (see idleState),
// with Busy set on the ones the reaper would leave alone. Sessions with no
// terminal row are the orphan prune's business, not this.
func (m *Manager) FindIdle(days int) ([]IdleTerminal, error) {
	c, err := m.ptydClient()
	if err != nil || days <= 0 {
		return nil, nil
	}
	sessions, err := c.List()
	if err != nil {
		return nil, err
	}
	all, err := m.Store.ListAllTerminalSessions()
	if err != nil {
		return nil, err
	}
	rows := map[string]int{}
	for i, ts := range all {
		if ts.Backend == BackendPtyd {
			rows[ts.ID] = i
		}
	}
	minIdle, now := time.Duration(days)*24*time.Hour, time.Now()
	var out []IdleTerminal
	for _, s := range sessions {
		i, ok := rows[s.ID]
		if !ok {
			continue
		}
		fg := foregroundPgrp(s.Pid)
		idle, reapable := idleState(s, fg, minIdle, now)
		if !idle {
			continue
		}
		it := IdleTerminal{TerminalID: s.ID, WorktreeID: all[i].WorktreeID, TabLabel: all[i].TabLabel, LastActivity: s.LastActivity}
		if !reapable {
			it.Busy = "unknown"
			if out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(fg)).Output(); fg > 0 && err == nil {
				it.Busy = strings.TrimSpace(string(out))
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// ReapIdle closes (tab and all, like the user closing it) every idle ptyd
// terminal that isn't busy, and returns those plus the busy ones it left.
func (m *Manager) ReapIdle(days int) (reaped, busy []IdleTerminal, err error) {
	idle, err := m.FindIdle(days)
	if err != nil {
		return nil, nil, err
	}
	for _, it := range idle {
		if it.Busy != "" {
			busy = append(busy, it)
			continue
		}
		ts, err := m.Store.GetTerminalSession(it.TerminalID)
		if err == nil && m.CloseSession(ts) == nil {
			reaped = append(reaped, it)
		}
	}
	return reaped, busy, nil
}
