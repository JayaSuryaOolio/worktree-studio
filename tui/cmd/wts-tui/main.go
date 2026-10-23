// Command wts-tui is a terminal frontend for a running worktree-studio
// server. The same UI ships inside the main binary as `wts tui`.
package main

import (
	"fmt"
	"os"

	"worktree-studio/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "wts-tui:", err)
		os.Exit(1)
	}
}
