// Command wts-tui is a terminal frontend for a running worktree-studio
// server — a prototype alongside the browser UI in web/.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	tea "github.com/charmbracelet/bubbletea"

	"worktree-studio/tui/app"
	"worktree-studio/tui/infra/httpapi"
	"worktree-studio/tui/ui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := httpapi.FromEnv()
	model := ui.NewSidebar(ctx, app.NewSidebar(client), app.NewNewWorktree(client), client)
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "wts-tui:", err)
		os.Exit(1)
	}
}
