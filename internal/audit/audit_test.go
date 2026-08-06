package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLogAppendsValidJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "audit.log.jsonl")
	l, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := l.Log("repo.add", map[string]any{"repo_id": "r1", "name": "adelaide"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := l.Log("worktree.create", map[string]any{"repo_id": "r1", "worktree_id": "w1"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open log file: %v", err)
	}
	defer f.Close()

	var lines []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("line %q is not valid JSON: %v", scanner.Text(), err)
		}
		lines = append(lines, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	for _, entry := range lines {
		if _, ok := entry["ts"]; !ok {
			t.Errorf("entry %+v missing ts field", entry)
		}
		if _, ok := entry["event"]; !ok {
			t.Errorf("entry %+v missing event field", entry)
		}
	}
	if lines[0]["event"] != "repo.add" || lines[0]["repo_id"] != "r1" {
		t.Errorf("first entry = %+v, want event=repo.add repo_id=r1", lines[0])
	}
	if lines[1]["event"] != "worktree.create" || lines[1]["worktree_id"] != "w1" {
		t.Errorf("second entry = %+v, want event=worktree.create worktree_id=w1", lines[1])
	}
}

func TestNewCreatesParentDirLazily(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")
	path := filepath.Join(dir, "audit.log.jsonl")

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("precondition: dir %q should not exist yet", dir)
	}

	if _, err := New(path); err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("New should have created parent dir %q: %v", dir, err)
	}
	// New itself must not create the file (it's created lazily on first Log).
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("New should not create the log file itself before Log is called")
	}
}
