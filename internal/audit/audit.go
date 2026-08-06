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
