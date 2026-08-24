package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// spotlightLogPrefix namespaces this subcommand's own error output the
// same way openFileLogPrefix does for open-file.
const spotlightLogPrefix = "worktree-studio__spotlight: "

const spotlightUsage = "usage: worktree-studio spotlight --start|--stop|--status [--stash] [path]"

// runSpotlightCommand handles the `spotlight --start|--stop|--status
// [--stash] [path]` subcommand: tells the running worktree-studio server to
// start/stop/query spotlight for a worktree *through* the server itself
// (see internal/api/spotlight.go's handleSpotlightCLIStart/Stop/Status),
// rather than shelling out to the external `spotlight` binary directly and
// bypassing worktree-studio's own audit log and UI status view — the same
// "go through the server, don't reimplement its job" idiom open-file
// already uses (openfile.go). This is what lets Claude (or anyone else
// scripting against a worktree) start spotlight for a worktree by path
// without needing that worktree's id, or needing to cd into it first.
//
// path is optional: if omitted, it defaults to the current working
// directory, same as open-file's implicit cwd resolution — handy when
// running from inside the worktree's own terminal pane. When given
// explicitly, it can be any path inside the target worktree (including the
// worktree root itself), letting a caller elsewhere on disk (e.g. an agent
// that hasn't cd'ed into the worktree) target it directly.
//
// Returns whether args[0] was this subcommand at all — false means main()
// should fall through to running the server, same convention as
// runOpenFileCommand/runHooksCommand.
func runSpotlightCommand(args []string) bool {
	if len(args) == 0 || args[0] != "spotlight" {
		return false
	}

	var action, path string
	var stash bool
	for _, a := range args[1:] {
		switch a {
		case "--start", "--stop", "--status":
			if action != "" {
				fmt.Fprintln(os.Stderr, spotlightLogPrefix+"only one of --start, --stop, --status may be given")
				os.Exit(1)
			}
			action = a
		case "--stash":
			stash = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, spotlightLogPrefix+"unrecognized flag %q\n%s\n", a, spotlightUsage)
				os.Exit(1)
			}
			if path != "" {
				fmt.Fprintln(os.Stderr, spotlightLogPrefix+"only one path may be given")
				os.Exit(1)
			}
			path = a
		}
	}
	if action == "" {
		fmt.Fprintln(os.Stderr, spotlightLogPrefix+spotlightUsage)
		os.Exit(1)
	}
	if action != "--start" && stash {
		fmt.Fprintln(os.Stderr, spotlightLogPrefix+"--stash only applies to --start")
		os.Exit(1)
	}

	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, spotlightLogPrefix+"resolve cwd: %v\n", err)
			os.Exit(1)
		}
		path = cwd
	}

	os.Exit(spotlightAction(action, path, stash))
	return true
}

func spotlightAction(action, path string, stash bool) int {
	addr := defaultAddr
	if v := os.Getenv("WORKTREE_STUDIO_ADDR"); v != "" {
		addr = v
	}
	selfBaseURL := "http://localhost" + addrPort(addr)

	client := &http.Client{Timeout: 15 * time.Second}
	var resp *http.Response
	var err error
	switch action {
	case "--status":
		resp, err = client.Get(selfBaseURL + "/api/spotlight/status?path=" + url.QueryEscape(path))
	case "--start", "--stop":
		body, merr := json.Marshal(map[string]any{"path": path, "stash": stash})
		if merr != nil {
			fmt.Fprintf(os.Stderr, spotlightLogPrefix+"encode request: %v\n", merr)
			return 1
		}
		endpoint := "/api/spotlight/start"
		if action == "--stop" {
			endpoint = "/api/spotlight/stop"
		}
		resp, err = client.Post(selfBaseURL+endpoint, "application/json", bytes.NewReader(body))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, spotlightLogPrefix+"is worktree-studio running? request failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, spotlightLogPrefix+"server returned %d: %s\n", resp.StatusCode, respBody)
		return 1
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status == "no matching worktree" {
		fmt.Fprintf(os.Stderr, spotlightLogPrefix+"%q isn't inside any worktree-studio-tracked worktree\n", path)
		return 1
	}

	fmt.Fprintln(os.Stdout, string(respBody))
	return 0
}
