package store

import (
	"errors"
	"testing"
)

func TestTaskStatusesSeededAndEditable(t *testing.T) {
	s := newTestStore(t)

	sts, err := s.ListTaskStatuses()
	if err != nil {
		t.Fatalf("ListTaskStatuses: %v", err)
	}
	if len(sts) != 3 || sts[0].Name != "In progress" || sts[2].Name != "Done" {
		t.Fatalf("seeded statuses = %+v", sts)
	}

	qa, err := s.AddTaskStatus("QA")
	if err != nil {
		t.Fatalf("AddTaskStatus: %v", err)
	}
	if err := s.ReorderTaskStatuses([]int64{sts[0].ID, sts[1].ID, qa.ID, sts[2].ID}); err != nil {
		t.Fatalf("ReorderTaskStatuses: %v", err)
	}
	if err := s.RenameTaskStatus(qa.ID, "Testing"); err != nil {
		t.Fatalf("RenameTaskStatus: %v", err)
	}
	sts, _ = s.ListTaskStatuses()
	if sts[2].Name != "Testing" || sts[3].Name != "Done" {
		t.Fatalf("after reorder+rename = %+v", sts)
	}

	task, err := s.CreateLocalTask("x", "")
	if err != nil {
		t.Fatalf("CreateLocalTask: %v", err)
	}
	if _, err := s.UpdateTask(task.Source, task.Key, TaskPatch{StatusID: &qa.ID}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if err := s.DeleteTaskStatus(qa.ID); !errors.Is(err, ErrTaskStatusInUse) {
		t.Fatalf("DeleteTaskStatus in use = %v, want ErrTaskStatusInUse", err)
	}
}

func TestLocalTaskKeysAreSequential(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateLocalTask("a", "")
	b, _ := s.CreateLocalTask("b", "https://example.com")
	if a.Key != "1" || b.Key != "2" || b.URL != "https://example.com" {
		t.Fatalf("keys = %q, %q (url %q)", a.Key, b.Key, b.URL)
	}
	sts, _ := s.ListTaskStatuses()
	if a.StatusID != sts[0].ID {
		t.Fatalf("new task status = %d, want first status %d", a.StatusID, sts[0].ID)
	}
}

func TestEnsureTaskKeepsHandSetURL(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.EnsureTask("jira", "PROJ-1", ""); err != nil {
		t.Fatalf("EnsureTask: %v", err)
	}
	url := "https://mine"
	if _, err := s.UpdateTask("jira", "PROJ-1", TaskPatch{URL: &url}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	got, err := s.EnsureTask("jira", "PROJ-1", "https://other")
	if err != nil {
		t.Fatalf("EnsureTask again: %v", err)
	}
	if got.URL != url {
		t.Fatalf("url = %q, want hand-set %q kept", got.URL, url)
	}
}

func TestSettleTaskStatus(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddRepo(Repo{ID: "r", Name: "r", Path: "/tmp/r"}); err != nil {
		t.Fatal(err)
	}
	task, _ := s.CreateLocalTask("t", "")
	for _, id := range []string{"w1", "w2"} {
		if err := s.AddWorktree(Worktree{ID: id, RepoID: "r", Name: id, Branch: id, Path: "/tmp/" + id, TaskSource: task.Source, TaskKey: task.Key}); err != nil {
			t.Fatal(err)
		}
	}
	sts, _ := s.ListTaskStatuses()
	first, last := sts[0].ID, sts[len(sts)-1].ID

	status := func() int64 {
		t.Helper()
		if err := s.SettleTaskStatus(task.Source, task.Key); err != nil {
			t.Fatalf("SettleTaskStatus: %v", err)
		}
		got, _ := s.GetTask(task.Source, task.Key)
		return got.StatusID
	}

	s.SetWorktreeStatus("w1", WorktreeStatusArchived)
	if got := status(); got != first {
		t.Fatalf("one of two archived: status = %d, want first %d", got, first)
	}
	s.SetWorktreeStatus("w2", WorktreeStatusArchived)
	if got := status(); got != last {
		t.Fatalf("all archived: status = %d, want last %d", got, last)
	}
	s.SetWorktreeStatus("w2", WorktreeStatusActive)
	if got := status(); got != first {
		t.Fatalf("unarchived: status = %d, want first %d", got, first)
	}
}
