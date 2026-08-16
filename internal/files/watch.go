package files

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounceWindow coalesces a rapid burst of fsnotify events for one
// logical change (a git checkout/pull touching many files, or an editor's
// own save round-trip) into a single notification per path, rather than
// firing once per raw OS event — see docs/editor-plan.md pitfall #5.
const debounceWindow = 300 * time.Millisecond

// ownWriteWindow is how long after this server's own WriteFile call the
// resulting fsnotify event for that same path is suppressed, so a save
// doesn't come back around as a false "changed on disk externally" —
// see docs/editor-plan.md pitfall #6.
const ownWriteWindow = 2 * time.Second

// ChangeEvent is reported for a file that changed on disk and was NOT the
// result of this server's own just-completed write.
type ChangeEvent struct {
	Path string // relative to the worktree root, forward-slash separated
}

// Watcher watches one worktree's tree recursively (skipping .git, which is
// both noisy and irrelevant to the editor) and reports debounced,
// own-write-filtered change events. Not meant to be constructed directly
// by API handlers — see Manager, which shares one Watcher across every ws
// subscriber for a given worktree.
type Watcher struct {
	root      string
	fsWatcher *fsnotify.Watcher
	events    chan ChangeEvent
	closeOnce sync.Once
	done      chan struct{}

	mu           sync.Mutex
	recentWrites map[string]time.Time // relPath -> when WriteFile last touched it
	pending      map[string]*time.Timer
}

// NewWatcher starts watching worktreePath. Callers must call Close when
// done to release the underlying OS watch handles.
func NewWatcher(worktreePath string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		root:         worktreePath,
		fsWatcher:    fsw,
		events:       make(chan ChangeEvent, 32),
		done:         make(chan struct{}),
		recentWrites: map[string]time.Time{},
		pending:      map[string]*time.Timer{},
	}

	if err := w.addTreeRecursive(worktreePath); err != nil {
		fsw.Close()
		return nil, err
	}

	go w.loop()
	return w, nil
}

func (w *Watcher) addTreeRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // best-effort: a file disappearing mid-walk isn't fatal
		}
		if d.IsDir() {
			// Same opaqueDirNames list files.go's tree listing uses to avoid
			// descending into node_modules/build — the watcher needs it even
			// more: recursively fsnotify.Add()-ing every directory inside a
			// large node_modules tree exhausts the process's file descriptor
			// limit ("too many open files"), which was a real reported
			// failure, not hypothetical.
			if d.Name() == ".git" || opaqueDirNames[d.Name()] {
				return filepath.SkipDir
			}
			return w.fsWatcher.Add(path)
		}
		return nil
	})
}

// Events returns the channel of debounced, own-write-filtered change
// events. Closed when the Watcher is closed.
func (w *Watcher) Events() <-chan ChangeEvent {
	return w.events
}

// MarkOwnWrite records that this server itself just wrote relPath, so the
// fsnotify event(s) that write produces are suppressed rather than
// reported as an external change.
func (w *Watcher) MarkOwnWrite(relPath string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.recentWrites[relPath] = time.Now()
}

func (w *Watcher) Close() {
	w.closeOnce.Do(func() {
		close(w.done)
		_ = w.fsWatcher.Close()
	})
}

func (w *Watcher) loop() {
	defer close(w.events)
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleRawEvent(event)
		case <-w.fsWatcher.Errors:
			// Best-effort: a watch error for one path shouldn't take down
			// the whole watcher; there's nothing actionable to do with it
			// beyond not propagating it as a change event.
		}
	}
}

func (w *Watcher) handleRawEvent(event fsnotify.Event) {
	// A newly created directory needs its own watch added so files
	// created inside it later are also seen.
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			_ = w.addTreeRecursive(event.Name)
			return
		}
	}

	rel, err := filepath.Rel(w.root, event.Name)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)

	w.mu.Lock()
	defer w.mu.Unlock()

	if t, ok := w.recentWrites[rel]; ok && time.Since(t) < ownWriteWindow {
		return // suppressed: this is almost certainly our own WriteFile call
	}

	if timer, ok := w.pending[rel]; ok {
		timer.Reset(debounceWindow)
		return
	}
	w.pending[rel] = time.AfterFunc(debounceWindow, func() {
		w.mu.Lock()
		delete(w.pending, rel)
		w.mu.Unlock()
		select {
		case w.events <- ChangeEvent{Path: rel}:
		case <-w.done:
		}
	})
}
