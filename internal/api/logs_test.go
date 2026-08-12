package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetLogsReturnsErrorLinesAndPath(t *testing.T) {
	ts, srv := newTestServer(t)

	logPath := filepath.Join(t.TempDir(), "server.log")
	content := "time=2026-01-01T00:00:00Z level=INFO msg=\"worktree-studio listening\"\n" +
		"time=2026-01-01T00:00:01Z level=ERROR msg=\"list repos\" err=\"boom\"\n" +
		"time=2026-01-01T00:00:02Z level=WARN msg=\"pruned stale terminal session rows\"\n" +
		"time=2026-01-01T00:00:03Z level=ERROR msg=\"spotlight start\" err=\"root dirty\"\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	srv.LogFilePath = logPath

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/settings/logs", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/settings/logs: status = %d, want 200", resp.StatusCode)
	}
	var body logsResponse
	decodeInto(t, resp, &body)

	if body.Path != logPath {
		t.Errorf("Path = %q, want %q", body.Path, logPath)
	}
	if len(body.Lines) != 2 {
		t.Fatalf("Lines = %v, want exactly 2 ERROR lines", body.Lines)
	}
	for _, line := range body.Lines {
		if !strings.Contains(line, "level=ERROR") {
			t.Errorf("non-ERROR line leaked into Lines: %q", line)
		}
	}
	if !strings.Contains(body.Lines[0], "list repos") || !strings.Contains(body.Lines[1], "spotlight start") {
		t.Errorf("Lines = %v, want them in file order (list repos, then spotlight start)", body.Lines)
	}
}

func TestGetLogsWithNoLogFileConfigured(t *testing.T) {
	ts, srv := newTestServer(t)
	srv.LogFilePath = "" // the default from newTestServer, asserted explicitly for clarity

	resp := doJSON(t, http.MethodGet, ts.URL+"/api/settings/logs", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/settings/logs: status = %d, want 200", resp.StatusCode)
	}
	var body logsResponse
	decodeInto(t, resp, &body)
	if body.Path != "" || len(body.Lines) != 0 {
		t.Errorf("body = %+v, want empty Path and Lines", body)
	}
}

// TestTailErrorLinesCapsAtMax verifies the ring-buffer eviction: with more
// ERROR lines than max, only the most recent max survive, in order.
func TestTailErrorLinesCapsAtMax(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "server.log")
	var content string
	for i := 0; i < 10; i++ {
		content += fmt.Sprintf("time=2026-01-01T00:00:00Z level=ERROR msg=\"err %d\"\n", i)
	}
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lines, err := tailErrorLines(logPath, 3)
	if err != nil {
		t.Fatalf("tailErrorLines: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	for i, want := range []string{"err 7", "err 8", "err 9"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("lines[%d] = %q, want it to contain %q", i, lines[i], want)
		}
	}
}

func TestTailErrorLinesMissingFile(t *testing.T) {
	lines, err := tailErrorLines(filepath.Join(t.TempDir(), "does-not-exist.log"), 10)
	if err != nil {
		t.Fatalf("tailErrorLines on missing file: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("lines = %v, want empty for a missing file", lines)
	}
}
