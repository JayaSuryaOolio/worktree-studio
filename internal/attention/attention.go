// Package attention tracks which worktrees currently have a Claude Code
// session waiting on the user (a permission prompt or an idle-waiting-for-
// input notification — see the Notification hook installed by
// internal/claudehook and internal/api/hooks.go's handling of it), and
// fans that out to every connected browser tab over a websocket
// (internal/api/attention.go's /ws/attention) so the sidebar can show a
// "needs your attention" badge even for a worktree that isn't the one
// currently open.
//
// Deliberately in-memory only, not persisted to the store: this is
// ephemeral UI state (a live claude session is either still waiting or
// it isn't), not something that needs to survive a server restart.
package attention

import "sync"

// Event is broadcast to every subscriber whenever a worktree's pending
// state changes.
type Event struct {
	WorktreeID string `json:"worktree_id"`
	Pending    bool   `json:"pending"`
	Message    string `json:"message,omitempty"`
}

// Tracker holds the current pending set and the live subscriber list.
type Tracker struct {
	mu      sync.Mutex
	pending map[string]string // worktreeID -> Claude's own notification message
	subs    map[chan Event]struct{}
}

// NewTracker returns an empty, ready-to-use Tracker.
func NewTracker() *Tracker {
	return &Tracker{
		pending: map[string]string{},
		subs:    map[chan Event]struct{}{},
	}
}

// SetPending marks worktreeID as waiting on the user and notifies every
// subscriber. Overwrites any previous message for the same worktree —
// only the most recent notification matters for a UI badge.
func (t *Tracker) SetPending(worktreeID, message string) {
	t.mu.Lock()
	t.pending[worktreeID] = message
	t.mu.Unlock()
	t.broadcast(Event{WorktreeID: worktreeID, Pending: true, Message: message})
}

// Clear removes worktreeID from the pending set (a no-op if it wasn't
// pending) and notifies every subscriber either way — a client that missed
// the original SetPending should still learn the badge is gone.
func (t *Tracker) Clear(worktreeID string) {
	t.mu.Lock()
	delete(t.pending, worktreeID)
	t.mu.Unlock()
	t.broadcast(Event{WorktreeID: worktreeID, Pending: false})
}

// Snapshot returns a copy of the current pending set (worktreeID ->
// message), sent to a subscriber right after it connects so a browser tab
// opened after the fact still sees whatever's already pending.
func (t *Tracker) Snapshot() map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]string, len(t.pending))
	for k, v := range t.pending {
		out[k] = v
	}
	return out
}

// Subscribe registers a new listener and returns its event channel plus an
// unsubscribe func the caller must call exactly once (typically on
// websocket disconnect) to stop leaking the channel and its goroutine slot.
// The channel is buffered so a slow reader doesn't block SetPending/Clear
// callers; see broadcast for what happens if it's ever actually full.
func (t *Tracker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	t.mu.Lock()
	t.subs[ch] = struct{}{}
	t.mu.Unlock()

	unsubscribe := func() {
		t.mu.Lock()
		if _, ok := t.subs[ch]; ok {
			delete(t.subs, ch)
			close(ch)
		}
		t.mu.Unlock()
	}
	return ch, unsubscribe
}

// broadcast fans event out to every current subscriber. A subscriber whose
// buffer is already full (16 unread events — this is a low-frequency
// stream, so that should never happen in practice) has this event dropped
// for it rather than blocking every other subscriber or the caller.
func (t *Tracker) broadcast(event Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ch := range t.subs {
		select {
		case ch <- event:
		default:
		}
	}
}
