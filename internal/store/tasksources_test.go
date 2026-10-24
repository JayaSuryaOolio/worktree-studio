package store

import "testing"

func TestTaskSourcesAndRefresh(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddTaskSource(TaskSource{ID: "jira", Name: "Jira", Command: "/bin/true"}); err != nil {
		t.Fatalf("AddTaskSource: %v", err)
	}
	if err := s.AddTaskSource(TaskSource{ID: "jira", Name: "x", Command: "x"}); err == nil {
		t.Fatal("duplicate id accepted")
	}
	srcs, _ := s.ListTaskSources()
	if len(srcs) != 1 || srcs[0].Command != "/bin/true" {
		t.Fatalf("ListTaskSources = %+v", srcs)
	}

	if _, err := s.EnsureTask("jira", "P-1", "https://hand-set"); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshTask("jira", "P-1", "From Jira", "https://jira/P-1"); err != nil {
		t.Fatal(err)
	}
	task, _ := s.GetTask("jira", "P-1")
	if task.Title != "From Jira" || task.URL != "https://hand-set" {
		t.Fatalf("after refresh: %+v", task)
	}

	if err := s.DeleteTaskSource("jira"); err != nil {
		t.Fatalf("DeleteTaskSource: %v", err)
	}
	if err := s.DeleteTaskSource("jira"); err == nil {
		t.Fatal("deleting a missing source should fail")
	}
}
