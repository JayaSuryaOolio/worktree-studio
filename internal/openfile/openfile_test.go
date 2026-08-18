package openfile

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

func TestPublishBroadcastsToSubscribers(t *testing.T) {
	tr := NewTracker()
	ch, unsubscribe := tr.Subscribe()
	defer unsubscribe()

	tr.Publish(Event{WorktreeID: "wt1", Path: "src/main.go"})

	got := recvOrTimeout(t, ch)
	want := Event{WorktreeID: "wt1", Path: "src/main.go"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestUnsubscribeStopsFurtherDelivery(t *testing.T) {
	tr := NewTracker()
	ch, unsubscribe := tr.Subscribe()
	unsubscribe()

	tr.Publish(Event{WorktreeID: "wt1", Path: "x"})

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

	for i := 0; i < 32; i++ {
		tr.Publish(Event{WorktreeID: "wt1", Path: "spam"})
	}

	select {
	case <-fast:
	case <-time.After(time.Second):
		t.Fatal("fast subscriber received nothing — Publish blocked on the slow one")
	}
	_ = slow
}
