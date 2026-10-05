package domain

import "testing"

func TestTicks(t *testing.T) {
	cases := []struct {
		g    GitStatus
		want string
	}{
		{GitStatus{HasUpstream: true, Ahead: 2, Behind: 1}, "↑2↓1"},
		{GitStatus{HasUpstream: true, Ahead: 3}, "↑3"},
		{GitStatus{HasUpstream: false, Ahead: 3}, ""},
		{GitStatus{HasUpstream: true}, ""},
	}
	for _, c := range cases {
		if got := c.g.Ticks(); got != c.want {
			t.Errorf("%+v: got %q want %q", c.g, got, c.want)
		}
	}
}

func TestAttentionApply(t *testing.T) {
	a := Attention{}.Apply(AttentionEvent{Snapshot: true, Pending: Attention{"w1": "hi"}})
	if a["w1"] != "hi" {
		t.Fatal("snapshot not applied")
	}
	b := a.Apply(AttentionEvent{Worktree: "w2", IsPending: true, Message: "x"})
	if len(b) != 2 || len(a) != 1 {
		t.Fatal("update must add without mutating the original")
	}
	if c := b.Apply(AttentionEvent{Worktree: "w1"}); len(c) != 1 || c["w2"] != "x" {
		t.Fatal("clear failed")
	}
}

func TestBranchChoicesPrependsMissingDefault(t *testing.T) {
	b := NewBranchChoices([]string{"dev", "origin/main"}, "main")
	if b.Branches[0] != "main" || b.DefaultIndex() != 0 || len(b.Branches) != 3 {
		t.Fatalf("%+v", b)
	}
	b = NewBranchChoices([]string{"dev", "main"}, "main")
	if len(b.Branches) != 2 || b.DefaultIndex() != 1 {
		t.Fatalf("%+v", b)
	}
}

func TestBranchFilter(t *testing.T) {
	b := NewBranchChoices([]string{"main", "origin/Feat-X", "dev"}, "main")
	if got := b.Filter("feat"); len(got) != 1 || got[0] != "origin/Feat-X" {
		t.Fatalf("%v", got)
	}
	if len(b.Filter(" ")) != 3 || len(b.Filter("zzz")) != 0 {
		t.Fatal("blank returns all, miss returns none")
	}
}
