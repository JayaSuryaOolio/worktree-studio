package domain

import (
	"strings"
	"testing"
)

func sessions(ids ...string) []TerminalSession {
	var out []TerminalSession
	for _, id := range ids {
		out = append(out, TerminalSession{ID: id})
	}
	return out
}

func leaves(t Tab) string { return strings.Join(t.Root.Leaves(), ",") }

// a | b over c:  split a right (b), then b down (c).
func TestSplitRectsAndNeighbors(t *testing.T) {
	root := Leaf("a").Split("a", Right, "b").Split("b", Down, "c")
	rects := root.Rects(Rect{0, 0, 81, 20})
	if (rects["a"] != Rect{0, 0, 40, 20}) || (rects["b"] != Rect{41, 0, 40, 10}) || (rects["c"] != Rect{41, 10, 40, 10}) {
		t.Fatalf("rects %v", rects)
	}
	for _, c := range []struct {
		from string
		d    Dir
		want string
	}{{"a", Right, "b"}, {"c", Left, "a"}, {"b", Down, "c"}, {"c", Up, "b"}, {"a", Left, ""}, {"b", Right, ""}} {
		if got, _ := Neighbor(rects, c.from, c.d); got != c.want {
			t.Errorf("%s %v: got %q want %q", c.from, c.d, got, c.want)
		}
	}
	if got := strings.Join(root.Close("b").Leaves(), ","); got != "a,c" {
		t.Fatalf("close b: %s", got)
	}
	if root.Close("a").Close("b").Close("c") != nil {
		t.Fatal("closing every leaf empties the tree")
	}
}

func TestWorkbenchReconcileSplitPlaceClose(t *testing.T) {
	w := Workbench{}.Reconcile(sessions("s1", "s2"))
	if len(w.Tabs) != 2 || leaves(w.Tabs[1]) != "s2" {
		t.Fatalf("one tab per new session: %+v", w.Tabs)
	}

	// split, then fill the empty pane with a newly created session
	w = w.Split(Right)
	if w.Tab().Focus != "" || leaves(w.Tab()) != "s1," {
		t.Fatalf("split focuses an empty pane: %s", leaves(w.Tab()))
	}
	if w.Split(Down).Tab().Root != w.Tab().Root {
		t.Fatal("only one empty pane at a time")
	}
	w = w.Place("s3").Reconcile(sessions("s1", "s2", "s3"))
	if len(w.Tabs) != 2 || leaves(w.Tab()) != "s1,s3" || w.Tab().Focus != "s3" {
		t.Fatalf("placed session must not also get its own tab: %+v", w.Tabs)
	}

	// closing a pane hides its session; it stays hidden across reconciles
	w = w.ClosePane().Reconcile(sessions("s1", "s2", "s3"))
	if leaves(w.Tab()) != "s1" || len(w.Hidden(sessions("s1", "s2", "s3"))) != 1 {
		t.Fatalf("after close: %s", leaves(w.Tab()))
	}

	// a session that ended disappears, and its tab with it
	w = w.Reconcile(sessions("s1", "s3"))
	if len(w.Tabs) != 1 || leaves(w.Tab()) != "s1" {
		t.Fatalf("ended session's tab: %+v", w.Tabs)
	}

	// closing the last pane leaves an empty pane to pick into
	w = w.ClosePane()
	if len(w.Tabs) != 1 || leaves(w.Tab()) != "" {
		t.Fatalf("last close: %+v", w.Tabs)
	}
}
