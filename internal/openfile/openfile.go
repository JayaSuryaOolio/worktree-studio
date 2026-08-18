// Package openfile fans out "open this file in the editor" events to every
// connected browser tab, fed by the `worktree-studio open-file <path>` CLI
// subcommand (see cmd/worktree-studio/openfile.go and
// internal/api/hooks.go's handleOpenFile) running inside a worktree's tmux
// pane.
//
// Deliberately simpler than internal/attention, which this is modeled on:
// attention tracks persistent state (a claude session is either still
// waiting or it isn't), worth replaying to a client that connects late via
// Snapshot. "Open this file" is a one-shot, transient instruction with no
// meaningful state to replay — there is nothing to send a newly-connected
// subscriber, so this Tracker has no pending set and no Snapshot.
package openfile

import "sync"

// Event is broadcast to every subscriber whenever a file should be opened.
type Event struct {
	WorktreeID string `json:"worktree_id"`
	Path       string `json:"path"`
}

// Tracker fans Publish calls out to every current subscriber.
type Tracker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewTracker returns an empty, ready-to-use Tracker.
func NewTracker() *Tracker {
	return &Tracker{subs: map[chan Event]struct{}{}}
}

// Publish fans event out to every current subscriber. A subscriber whose
// buffer is already full (16 unread events — this is a low-frequency
// stream, so that should never happen in practice) has this event dropped
// for it rather than blocking every other subscriber or the caller.
func (t *Tracker) Publish(event Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ch := range t.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// Subscribe registers a new listener and returns its event channel plus an
// unsubscribe func the caller must call exactly once (typically on
// websocket disconnect) to stop leaking the channel and its goroutine slot.
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
