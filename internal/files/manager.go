package files

import "sync"

// Manager owns one Watcher per worktree, shared across every ws subscriber
// for that worktree (ref-counted), rather than one Watcher per browser
// tab. This matters for correctness, not just efficiency: own-write
// suppression (Watcher.MarkOwnWrite) and debounce state need to be shared
// so a save made from one tab doesn't come back as a false
// "changed externally" push to every tab watching that worktree,
// including the one that made the save.
type Manager struct {
	mu       sync.Mutex
	watchers map[string]*refCountedWatcher // worktreeID -> ...
}

type refCountedWatcher struct {
	watcher *Watcher
	refs    int
}

func NewManager() *Manager {
	return &Manager{watchers: map[string]*refCountedWatcher{}}
}

// Subscribe returns the shared Watcher for worktreeID, creating it (and
// starting the underlying fsnotify watch) if this is the first subscriber.
// Callers must call Unsubscribe exactly once when done, from the same
// worktreeID.
func (m *Manager) Subscribe(worktreeID, worktreePath string) (*Watcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rc, ok := m.watchers[worktreeID]
	if !ok {
		w, err := NewWatcher(worktreePath)
		if err != nil {
			return nil, err
		}
		rc = &refCountedWatcher{watcher: w}
		m.watchers[worktreeID] = rc
	}
	rc.refs++
	return rc.watcher, nil
}

// Unsubscribe releases one reference to worktreeID's watcher, closing and
// dropping it once the last subscriber is gone.
func (m *Manager) Unsubscribe(worktreeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rc, ok := m.watchers[worktreeID]
	if !ok {
		return
	}
	rc.refs--
	if rc.refs <= 0 {
		rc.watcher.Close()
		delete(m.watchers, worktreeID)
	}
}

// MarkOwnWrite records relPath as just written by this server for
// worktreeID's watcher, if one is currently active. A no-op when nobody is
// currently subscribed (no watcher means nothing to suppress an event on).
func (m *Manager) MarkOwnWrite(worktreeID, relPath string) {
	m.mu.Lock()
	rc, ok := m.watchers[worktreeID]
	m.mu.Unlock()
	if ok {
		rc.watcher.MarkOwnWrite(relPath)
	}
}
