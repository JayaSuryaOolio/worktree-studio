package ui

import (
	"context"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"worktree-studio/tui/app"
	"worktree-studio/tui/domain"
)

type (
	draftMsg struct {
		repo  domain.RepoID
		draft app.Draft
		err   error
	}
	createdMsg struct{ err error }
)

const (
	fieldName = iota
	fieldBranch
	fieldCount
)

// newWorktreeDialog is the modal form behind `n`: a name input and a source
// branch picker. It owns its own focus/keys; the sidebar only routes to it.
type newWorktreeDialog struct {
	ctx        context.Context
	uc         *app.NewWorktree
	repo       domain.RepoID
	name       textinput.Model
	filter     textinput.Model
	branches   domain.BranchChoices
	branchIdx  int
	focus      int
	loading    bool
	submitting bool
	err        error
}

// openNewWorktree returns the dialog plus the command that loads its draft.
func openNewWorktree(ctx context.Context, uc *app.NewWorktree, repo domain.RepoID) (*newWorktreeDialog, tea.Cmd) {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "generating suggestion…"
	in.CharLimit = 100
	in.Focus()
	f := textinput.New()
	f.Prompt = "/ "
	f.Placeholder = "type to filter branches"
	d := &newWorktreeDialog{ctx: ctx, uc: uc, repo: repo, name: in, filter: f, loading: true}
	load := func() tea.Msg {
		draft, err := uc.Prepare(ctx, repo)
		return draftMsg{repo, draft, err}
	}
	return d, tea.Batch(textinput.Blink, load)
}

// update handles one message. done reports that the dialog should close;
// created reports that a worktree was made (so the sidebar must reload).
func (d *newWorktreeDialog) update(msg tea.Msg) (cmd tea.Cmd, done, created bool) {
	switch msg := msg.(type) {
	case draftMsg:
		d.loading = false
		if msg.err != nil {
			d.err = msg.err
			return nil, false, false
		}
		d.branches, d.branchIdx = msg.draft.Branches, msg.draft.Branches.DefaultIndex()
		d.name.SetValue(msg.draft.Name)
		d.name.CursorEnd()
		d.name.Placeholder = ""
	case createdMsg:
		d.submitting = false
		if msg.err != nil {
			d.err = msg.err
			return nil, false, false
		}
		return nil, true, true
	case tea.KeyMsg:
		return d.onKey(msg)
	}
	return nil, false, false
}

func (d *newWorktreeDialog) onKey(k tea.KeyMsg) (tea.Cmd, bool, bool) {
	if d.submitting {
		return nil, false, false
	}
	switch k.String() {
	case "esc":
		return nil, true, false
	case "tab", "shift+tab":
		d.focus = (d.focus + 1) % fieldCount
		if d.focus == fieldName {
			d.filter.Blur()
			return d.name.Focus(), false, false
		}
		d.name.Blur()
		return d.filter.Focus(), false, false
	case "enter":
		shown := d.shown()
		if d.loading || len(shown) == 0 {
			return nil, false, false
		}
		d.submitting, d.err = true, nil
		name, src := d.name.Value(), shown[d.branchIdx]
		return func() tea.Msg {
			_, err := d.uc.Create(d.ctx, d.repo, name, src)
			return createdMsg{err}
		}, false, false
	}
	if d.focus == fieldBranch {
		n := len(d.shown())
		switch k.String() {
		case "left", "up", "ctrl+p":
			if n > 0 {
				d.branchIdx = (d.branchIdx + n - 1) % n
			}
			return nil, false, false
		case "right", "down", "ctrl+n":
			if n > 0 {
				d.branchIdx = (d.branchIdx + 1) % n
			}
			return nil, false, false
		}
		before := d.filter.Value()
		var cmd tea.Cmd
		d.filter, cmd = d.filter.Update(k)
		if d.filter.Value() != before {
			d.branchIdx = 0 // best (first) match
		}
		return cmd, false, false
	}
	var cmd tea.Cmd
	d.name, cmd = d.name.Update(k)
	return cmd, false, false
}

// shown is the branch list after applying the filter.
func (d *newWorktreeDialog) shown() []string { return d.branches.Filter(d.filter.Value()) }

func (d *newWorktreeDialog) view() string {
	label := func(f int, s string) string {
		if d.focus == f {
			return accentStyle.Render(s)
		}
		return mutedStyle.Render(s)
	}
	branch := "loading branches…"
	if !d.loading {
		branch = dimStyle.Render("no matching branch")
		if shown := d.shown(); len(shown) > 0 {
			branch = "‹ " + shown[d.branchIdx] + " ›" + mutedStyle.Render(" "+strconv.Itoa(d.branchIdx+1)+"/"+strconv.Itoa(len(shown)))
		}
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("New worktree") + "\n\n")
	b.WriteString(label(fieldName, "Name (branch + directory)") + "\n" + d.name.View() + "\n\n")
	b.WriteString(label(fieldBranch, "Create from") + "\n" + d.filter.View() + "\n" + branch + "\n\n")
	switch {
	case d.submitting:
		b.WriteString(dimStyle.Render("Creating…"))
	case d.err != nil:
		b.WriteString(errStyle.Render(d.err.Error()))
	default:
		b.WriteString(dimStyle.Render("tab switch  ←/→ pick branch  enter create  esc cancel"))
	}
	return boxStyle.Render(b.String())
}
