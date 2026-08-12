// The main settings modal's Logs tab: this server's own recent
// ERROR-level output, plus the file path it's written to — so a person
// can jump to something wrong at a glance, or open/tail/grep the real
// file directly for anything this bounded view leaves out (WARN/INFO
// lines, or more than maxLogLines of history).
package api

import (
	"bufio"
	"net/http"
	"os"
	"strings"
)

// maxLogLines caps how many recent error lines the Logs tab returns — the
// log file itself is never rotated (see main.go's logFilePath, same
// "deliberately basic, no rotation" call internal/audit.Logger makes for
// the audit log), so this keeps the response bounded without needing to
// touch anything on disk.
const maxLogLines = 200

type logsResponse struct {
	// Path is "" if this server has no durable log file to read (e.g. its
	// home directory couldn't be resolved at startup) — Lines is always
	// empty in that case too, there's nothing else to fall back to.
	Path  string   `json:"path"`
	Lines []string `json:"lines"`
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	if s.LogFilePath == "" {
		writeJSON(w, http.StatusOK, logsResponse{Lines: []string{}})
		return
	}

	lines, err := tailErrorLines(s.LogFilePath, maxLogLines)
	if err != nil {
		s.Log.Error("read server log", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read server log: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, logsResponse{Path: s.LogFilePath, Lines: lines})
}

// tailErrorLines returns the last max lines of path containing
// "level=ERROR" — slog's text handler always emits that exact substring
// for an Error-level record, so this is a plain substring check, not a
// real log parser (a WARN/INFO line embedding that literal string in some
// other field would be a false positive, but that's an acceptable
// trade-off for a debug-aid view, not a security or correctness boundary).
// Scans the whole file top to bottom every call — fine at this tool's
// scale (a single local server's own log), same trade-off
// audit.Logger.ReadAll makes for the audit log.
func tailErrorLines(path string, max int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	// A simple ring buffer: once full, each new match evicts the oldest
	// rather than growing the slice unbounded across a potentially large
	// file.
	ring := make([]string, 0, max)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "level=ERROR") {
			continue
		}
		if len(ring) == max {
			ring = ring[1:]
		}
		ring = append(ring, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ring, nil
}
