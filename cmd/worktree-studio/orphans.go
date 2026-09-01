package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// orphansLogPrefix namespaces this subcommand's own error output the same
// way spotlightLogPrefix/openFileLogPrefix do for their subcommands.
const orphansLogPrefix = "worktree-studio__orphans: "

const orphansUsage = "usage: worktree-studio orphans [--kill] [--min-age <duration>]"

// defaultOrphansMinAge mirrors api.defaultOrphanMinAge — kept as a literal
// here rather than imported, since this file talks to the server over HTTP
// like every other CLI subcommand, not by calling into internal/ directly.
const defaultOrphansMinAge = 7 * 24 * time.Hour

// runOrphansCommand handles the `orphans [--kill] [--min-age <duration>]`
// subcommand: lists (or, with --kill, prunes) live tmux sessions in
// worktree-studio's own namespace that have no terminal_sessions row —
// e.g. leaked test sessions, or ones created by hand outside the app.
//
// This exists specifically so cleaning these up is never again a one-off
// exercise of hand-running `tmux kill-session` against whatever looks
// orphaned by eye — that's exactly what swept up real, still-in-use
// sessions once already (see PROGRESS.md). Every prune through here goes
// through the server's own 7-day-by-default activity safeguard (see
// internal/term.KillOrphanTmuxSessions) — a session touched more recently
// than --min-age is never killed, and there is no flag to bypass that
// check, only to choose a different (still real, still enforced) window.
//
// Returns whether args[0] was this subcommand at all — false means main()
// should fall through to running the server, same convention as
// runSpotlightCommand/runOpenFileCommand/runHooksCommand.
func runOrphansCommand(args []string) bool {
	if len(args) == 0 || args[0] != "orphans" {
		return false
	}

	kill := false
	minAge := defaultOrphansMinAge
	for _, a := range args[1:] {
		switch {
		case a == "--kill":
			kill = true
		case strings.HasPrefix(a, "--min-age="):
			d, err := time.ParseDuration(strings.TrimPrefix(a, "--min-age="))
			if err != nil {
				fmt.Fprintf(os.Stderr, orphansLogPrefix+"invalid --min-age: %v\n", err)
				os.Exit(1)
			}
			minAge = d
		default:
			fmt.Fprintf(os.Stderr, orphansLogPrefix+"unrecognized argument %q\n%s\n", a, orphansUsage)
			os.Exit(1)
		}
	}

	os.Exit(orphansAction(kill, minAge))
	return true
}

func orphansAction(kill bool, minAge time.Duration) int {
	addr := defaultAddr
	if v := os.Getenv("WORKTREE_STUDIO_ADDR"); v != "" {
		addr = v
	}
	selfBaseURL := "http://localhost" + addrPort(addr)

	minAgeHours := strconv.FormatFloat(minAge.Hours(), 'f', -1, 64)
	client := &http.Client{Timeout: 15 * time.Second}
	var resp *http.Response
	var err error
	if kill {
		resp, err = client.Post(selfBaseURL+"/api/orphan-tmux-sessions/prune?min_age_hours="+url.QueryEscape(minAgeHours), "application/json", nil)
	} else {
		resp, err = client.Get(selfBaseURL + "/api/orphan-tmux-sessions?min_age_hours=" + url.QueryEscape(minAgeHours))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, orphansLogPrefix+"is worktree-studio running? request failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, orphansLogPrefix+"server returned %d: %s\n", resp.StatusCode, respBody)
		return 1
	}

	if kill {
		var result struct {
			Killed    []string `json:"killed"`
			Protected []string `json:"protected"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			fmt.Fprintf(os.Stderr, orphansLogPrefix+"decode response: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "killed %d orphan tmux session(s): %v\n", len(result.Killed), result.Killed)
		if len(result.Protected) > 0 {
			fmt.Fprintf(os.Stdout, "protected (active within %s, left alone): %v\n", minAge, result.Protected)
		}
		return 0
	}

	var orphans []struct {
		Name         string `json:"name"`
		LastActivity string `json:"last_activity"`
		Protected    bool   `json:"protected"`
	}
	if err := json.Unmarshal(respBody, &orphans); err != nil {
		fmt.Fprintf(os.Stderr, orphansLogPrefix+"decode response: %v\n", err)
		return 1
	}
	if len(orphans) == 0 {
		fmt.Fprintln(os.Stdout, "no orphan tmux sessions found")
		return 0
	}
	for _, o := range orphans {
		status := "would be killed"
		if o.Protected {
			status = fmt.Sprintf("protected — active within %s", minAge)
		}
		fmt.Fprintf(os.Stdout, "%s  last_activity=%s  %s\n", o.Name, o.LastActivity, status)
	}
	fmt.Fprintln(os.Stdout, "\nrun with --kill to prune the ones not marked protected")
	return 0
}
