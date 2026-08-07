// Package audit provides a minimal append-only JSONL audit logger.
//
// Every mutating action in worktree-studio (registering a repo, creating or
// removing a worktree, etc.) should call Log so there is a single
// human-greppable, script-parseable record of what happened and when.
//
// Deliberately basic for v1: no rotation, no querying UI — just a file you
// can `tail -f` or `jq` through. See PLAN.md section 6.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultPath returns the default audit log location: ~/.worktree-studio/audit.log.jsonl
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".worktree-studio", "audit.log.jsonl"), nil
}

// Logger appends JSON-lines audit events to a file.
type Logger struct {
	path string
	mu   sync.Mutex
}

// New creates a Logger writing to path, creating its parent directory if
// missing. The file itself is created lazily on first Log call (via
// O_APPEND|O_CREATE) so New never fails just because the file doesn't exist
// yet.
func New(path string) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create audit log dir %s: %w", dir, err)
	}
	return &Logger{path: path}, nil
}

// NewDefault creates a Logger at DefaultPath().
func NewDefault() (*Logger, error) {
	p, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return New(p)
}

// Log appends one JSON line: {"ts":..., "event":..., <fields...>}.
// Errors are returned so callers can decide whether to surface them, but a
// failure to audit-log should generally not fail the underlying mutating
// operation itself — callers typically log the error and continue.
func (l *Logger) Log(event string, fields map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for k, v := range fields {
		entry[k] = v
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit log %s: %w", l.path, err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write audit log %s: %w", l.path, err)
	}
	return nil
}

// ReadAll parses every line of the log file into a map (each entry has at
// least "ts" and "event" keys, plus whatever fields the caller logged). A
// log file that doesn't exist yet (nothing has ever been logged) returns an
// empty slice, not an error — matching Log's own lazy-creation behavior.
// Malformed lines are skipped rather than failing the whole read, since a
// single corrupt line (e.g. a partial write from a crash) shouldn't make
// the rest of the log unreadable.
func (l *Logger) ReadAll() ([]map[string]any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, fmt.Errorf("open audit log %s: %w", l.path, err)
	}
	defer f.Close()

	var entries []map[string]any
	scanner := bufio.NewScanner(f)
	// Audit lines can carry sizeable fields (e.g. a diff comment body in a
	// future event type); raise the default 64KB token limit generously.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read audit log %s: %w", l.path, err)
	}
	return entries, nil
}
