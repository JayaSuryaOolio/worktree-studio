package files

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitForEvent(t *testing.T, events <-chan ChangeEvent, timeout time.Duration) (ChangeEvent, bool) {
	t.Helper()
	select {
	case ev := <-events:
		return ev, true
	case <-time.After(timeout):
		return ChangeEvent{}, false
	}
}

func expectNoEvent(t *testing.T, events <-chan ChangeEvent, wait time.Duration) {
	t.Helper()
	select {
	case ev := <-events:
		t.Fatalf("expected no event, got %+v", ev)
	case <-time.After(wait):
	}
}

func TestWatcherReportsExternalChange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(dir)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	ev, ok := waitForEvent(t, w.Events(), 2*time.Second)
	if !ok {
		t.Fatal("expected a change event, got none")
	}
	if ev.Path != "a.txt" {
		t.Errorf("event path = %q, want %q", ev.Path, "a.txt")
	}
}

func TestWatcherSuppressesOwnWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(dir)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	w.MarkOwnWrite("a.txt")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	expectNoEvent(t, w.Events(), 700*time.Millisecond)
}

func TestWatcherDebouncesBurst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(dir)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(path, []byte("burst"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, ok := waitForEvent(t, w.Events(), 2*time.Second); !ok {
		t.Fatal("expected one coalesced event, got none")
	}
	// No second event should follow shortly after — the burst should have
	// collapsed into exactly one notification.
	expectNoEvent(t, w.Events(), 500*time.Millisecond)
}

// Regression test for a real reported failure: recursively fsnotify-
// watching every directory inside a large node_modules tree exhausted the
// process's file descriptor limit ("too many open files"). The watcher
// must skip opaque dirs (files.go's opaqueDirNames) the same way the file
// tree's own listing does.
func TestWatcherDoesNotWatchInsideOpaqueDirs(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "node_modules", "some-pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nested, "index.js")
	if err := os.WriteFile(nestedFile, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(dir)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := os.WriteFile(nestedFile, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	expectNoEvent(t, w.Events(), 700*time.Millisecond)
}

func TestManagerSharesWatcherAcrossSubscribers(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()

	w1, err := m.Subscribe("wt1", dir)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	w2, err := m.Subscribe("wt1", dir)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if w1 != w2 {
		t.Error("expected the same Watcher instance for two subscribers of the same worktree")
	}

	m.Unsubscribe("wt1")
	m.MarkOwnWrite("wt1", "a.txt") // still one subscriber left; must not panic

	m.Unsubscribe("wt1")
	// No subscribers left now — MarkOwnWrite on an unwatched worktree must
	// be a harmless no-op, not a panic.
	m.MarkOwnWrite("wt1", "a.txt")
}
