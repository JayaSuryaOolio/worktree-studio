package attention

import (
	"testing"
	"time"
)

func recvOrTimeout(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func TestSetPendingBroadcastsToSubscribers(t *testing.T) {
	tr := NewTracker()
	ch, unsubscribe := tr.Subscribe()
	defer unsubscribe()

	tr.SetPending("wt1", "Claude needs your permission to use Bash")

	got := recvOrTimeout(t, ch)
	want := Event{WorktreeID: "wt1", Pending: true, Message: "Claude needs your permission to use Bash"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestClearBroadcastsPendingFalse(t *testing.T) {
	tr := NewTracker()
	tr.SetPending("wt1", "waiting")
	ch, unsubscribe := tr.Subscribe()
	defer unsubscribe()

	tr.Clear("wt1")

	got := recvOrTimeout(t, ch)
	want := Event{WorktreeID: "wt1", Pending: false}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestClearOnNeverPendingWorktreeIsANoOpNotAnError(t *testing.T) {
	tr := NewTracker()
	ch, unsubscribe := tr.Subscribe()
	defer unsubscribe()

	tr.Clear("never-pending")

	got := recvOrTimeout(t, ch)
	if got.Pending {
		t.Errorf("got Pending=true, want false")
	}
}

func TestSnapshotReflectsCurrentPendingSet(t *testing.T) {
	tr := NewTracker()
	tr.SetPending("wt1", "msg1")
	tr.SetPending("wt2", "msg2")
	tr.Clear("wt1")

	snap := tr.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot = %+v, want exactly 1 entry", snap)
	}
	if snap["wt2"] != "msg2" {
		t.Errorf("snapshot[wt2] = %q, want %q", snap["wt2"], "msg2")
	}
}

func TestUnsubscribeStopsFurtherDelivery(t *testing.T) {
	tr := NewTracker()
	ch, unsubscribe := tr.Subscribe()
	unsubscribe()

	tr.SetPending("wt1", "msg")

	// The channel should be closed, not just quiet — receiving from a
	// closed channel returns the zero value immediately rather than
	// blocking, which is what proves unsubscribe actually tore it down
	// rather than just leaving it unfed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after unsubscribe, got a real value instead")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out — channel was neither closed nor delivered to, unsubscribe leaked it")
	}
}

func TestSecondUnsubscribeIsSafe(t *testing.T) {
	tr := NewTracker()
	_, unsubscribe := tr.Subscribe()
	unsubscribe()
	unsubscribe() // must not double-close the channel and panic
}

func TestSlowSubscriberDoesNotBlockOthers(t *testing.T) {
	tr := NewTracker()
	slow, unsubscribeSlow := tr.Subscribe()
	defer unsubscribeSlow()
	fast, unsubscribeFast := tr.Subscribe()
	defer unsubscribeFast()

	// Overflow the slow subscriber's buffer (16) without ever draining it.
	for i := 0; i < 32; i++ {
		tr.SetPending("wt1", "spam")
	}

	// The fast subscriber must still have received at least one event —
	// proving broadcast() didn't block on the full slow channel.
	select {
	case <-fast:
	case <-time.After(time.Second):
		t.Fatal("fast subscriber received nothing — broadcast blocked on the slow one")
	}
	_ = slow
}
