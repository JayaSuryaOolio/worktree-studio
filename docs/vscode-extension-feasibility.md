# Feasibility: worktree-studio + Conveyor as a VS Code extension

Evaluates whether worktree-studio's worktree-switching/terminal UI, plus the Conveyor kanban feature described in `kanban-board-prd.md`, could be built as a VS Code extension instead of (or alongside) the current browser-based UI — specifically: instant (<500ms) switching between worktrees inside one VS Code window, hooking into terminals and the sidebar, and controlling other extensions (notably the official Claude Code VS Code extension). Researched via four parallel deep-dives (VS Code extension API docs, VS Code source, the installed Claude Code extension bundle, and real-world GitHub issues/marketplace data) rather than from general knowledge alone — sources are cited inline throughout.

## 0. Recommendation up front

**Feasible, with one real trick and one real dead end.** The <500ms worktree-switch requirement is achievable — but not by "hooking into branch switching" (wrong mental model — Section 1). There's a different, already-proven mechanism that gets you there, and one existing niche extension already ships it. Terminal/tmux reuse carries over at ~90% fidelity — genuinely good news, since that's most of worktree-studio's actual engineering (Section 2). The sidebar kanban UI is straightforward (Section 3). But "controlling the official Claude Code extension" is not realistic — its `cwd` is hardcoded to the first workspace folder, it has no exported API, and no status readback exists (Section 4). That one non-negotiable fact reshapes the whole design: you'd drive the `claude` CLI directly (which the Conveyor PRD already does), and the VS Code extension would sit next to it, not on top of it.

---

## 1. Instant worktree switching — the branch-checkout analogy is wrong, but there's a real fix

`Git: Checkout to...` never changes which folder VS Code has open — it's `git checkout` inside the *same already-open directory*; VS Code just reacts to file-change events (source: [VS Code docs, branches & worktrees](https://code.visualstudio.com/docs/sourcecontrol/branches-worktrees)). A worktree is a genuinely different directory. There's no "branch switch" machinery to hook into, because worktrees and branch-checkout aren't the same kind of operation under the hood, even though they look similar to a user.

The actual mechanisms, in order of how VS Code treats them:

| Mechanism | Reload cost | Verdict |
|---|---|---|
| `vscode.openFolder({forceReuseWindow:true})` | Full extension-host restart, editors/terminals killed. Real-world reports: 10–15s on nontrivial repos ([#186121](https://github.com/microsoft/vscode/issues/186121)), 20s+ ([#158620](https://github.com/microsoft/vscode/issues/158620)) | This is what every existing "git worktree" extension uses today. Not close to 500ms. |
| Custom `FileSystemProvider` (virtual filesystem trick, à la vscode.dev) | No reload | Turns your window into a *virtual workspace* — extensions can opt out entirely, debugging breaks, LSP filesystem support is still incomplete (source: [Virtual Workspaces guide](https://code.visualstudio.com/api/extension-guides/virtual-workspaces), [wiki](https://github.com/microsoft/vscode/wiki/Virtual-Workspaces)). Wrong tool for real local editing. Rejected. |
| `workspace.updateWorkspaceFolders()`, appending/removing a **non-first** slot in an already-multi-root workspace | No restart | **The only viable path.** Confirmed in VS Code's own source: the ext-host restart only fires "if the first workspace folder is added, removed or changed" ([#165010](https://github.com/microsoft/vscode/issues/165010)) — Microsoft even has an open backlog issue to remove this restart entirely ([#69335](https://github.com/microsoft/vscode/issues/69335), still open, Backlog). |

That third row isn't theoretical — **[`tmokmss/vscode-git-worktree-switcher`](https://github.com/tmokmss/vscode-git-worktree-switcher)** already ships exactly this: a permanent pinned "anchor" folder in slot 0, worktrees swapped in slots 1..n via `updateWorkspaceFolders`, no reload. Its own README calls out preserving terminal sessions and Claude Code panel state across the switch — literally this project's exact scenario, already validated in the wild.

**The real compromise this forces**: the window is permanently a multi-root workspace with a stub anchor folder, not a clean single-root project. VS Code's explorer will always show a "MULTI-ROOT WORKSPACE" framing, and it requires discipline to prune folders not actively in use (add one, remove the previous) rather than accumulating N worktrees in the sidebar forever. That's a real, visible UX cost — not free.

Additional context on why the restart is structural, not a bug: `vscode.d.ts`'s own doc comment on `updateWorkspaceFolders` warns extensions "may be terminated and restarted." bpasero (VS Code maintainer), [#69335](https://github.com/microsoft/vscode/issues/69335): single-folder → multi-root "will always require an extension host restart because the workspace ID changes and thus the state location." The underlying blocker is the deprecated `workspace.rootPath`, still used across hundreds of extensions in the wild — Microsoft has stated it cannot be removed for compatibility reasons, which is exactly why the "first folder never changes" workaround is the only cheap lever available.

## 2. Terminal/tmux reuse — the good news, and it's most of the engineering

`vscode.window.createTerminal` with a custom `Pseudoterminal` lets an extension own a terminal's I/O completely — no VS Code-spawned shell at all (source: [Pseudoterminal API reference](https://code.visualstudio.com/api/references/vscode-api#Pseudoterminal), [extHostTerminalService.ts](https://github.com/microsoft/vscode/blob/main/src/vs/workbench/api/common/extHostTerminalService.ts)). You can pipe it straight to/from the exact same tmux-attach architecture worktree-studio already runs (`internal/term/attach.go` in the main repo) — swapping "websocket → xterm.js in browser" for "in-process bridge → VS Code's own terminal renderer." Full ANSI/CSI passthrough works; resize works (`setDimensions` → `term.Resize`); multiple simultaneous terminal tabs work, including as real editor-tab terminals (`TerminalLocation.Editor`) — arguably closer to worktree-studio's current dockview layout than the sidebar terminal panel would be. `TerminalSplitLocationOptions.parentTerminal` gives true tmux-pane-style splits ([#131278](https://github.com/microsoft/vscode/issues/131278)).

Four real degradations, not deal-breakers:

- **String-based I/O, not raw bytes** — `onDidWrite: Event<string>` / `handleInput(data: string)`. The bridge needs incremental UTF-8 decoding to avoid mangling multi-byte runes split across a read boundary; genuinely binary output is lossy.
- **No flow control** — extension-owned terminals have no backpressure ack (`acknowledgeDataEvent()` is documented as a no-op: *"flow control is not supported in extension owned terminals"*). High-throughput output has a lower ceiling than a native terminal or the current direct websocket path.
- **No native persistence across a window reload** — `shouldPersist = false` is hardcoded for extension-owned terminals; VS Code's terminal owner confirmed on record this is architecturally impossible to change from the extension's side ([vscode-discussions#199](https://github.com/microsoft/vscode-discussions/discussions/199)): *"since Pseudoterminal-based terminals are connected to the extension host which gets taken down, this is not possible… you could do it on the extension's end manually by tracking the state of the terminal so that after a relaunch you can re-create the Pseudoterminal and rehydrate it."* This matters less here than for most extensions, though — state already lives in tmux, not in VS Code. On reactivation, enumerate live sessions via the existing REST API and re-`createTerminal` + re-attach one per session; `tmux attach-session` full-repaints the pane, so running processes and visible content come back intact. The visible cost is a brief tab flicker/reorder on reload, and scrollback *above* the currently-visible pane is lost unless pre-captured (`tmux capture-pane -pe -S -3000` before the attach stream).
- **No Shell Integration API** (CWD detection, command-exit tracking) — it's delivered via shell-startup scripts emitting OSC 633 sequences that a custom pty never gets, and even hand-emitted sequences misbehave ([#179913](https://github.com/microsoft/vscode/issues/179913), closed *not planned*; [#190253](https://github.com/microsoft/vscode/issues/190253)). Not a real loss: `tmux display-message -p '#{pane_current_path}'` (already used in `internal/term/attach.go`) covers the same need.

Mouse reporting inside the integrated terminal has a documented history of rough edges ([#96058](https://github.com/microsoft/vscode/issues/96058), [#14627](https://github.com/microsoft/vscode/issues/14627)) — worth a real test given worktree-studio's own recent tmux-mouse investigation (`docs/terminal-clipboard.md`'s Problem 6).

Prior art: Remote-SSH/WSL do *not* use this API — they run a full VS Code Server + remote pty host on the other side, so their terminals are real ptys with full persistence and Shell Integration (source: [Remote-SSH docs](https://code.visualstudio.com/docs/remote/ssh)). Nobody gets extension-owned-terminal persistence; closest comparable implementations are [ShMcK/vscode-pseudoterminal](https://github.com/ShMcK/vscode-pseudoterminal) and [cybersader/vscode-terminal-workspaces](https://github.com/cybersader/vscode-terminal-workspaces/blob/main/docs/tmux-integration.md).

**Summary: ~90% parity.** Fully achievable: attach, I/O, ANSI, resize, multi-tab, lifecycle, title-driven tab labels (via `Pseudoterminal.onDidChangeName`, since `Terminal.name` itself is read-only). Degraded: reload flicker + lost scrollback-above-pane, no flow control at high throughput, UTF-8-only stream. Impossible: native terminal restoration and Shell Integration for these terminals — neither is a real loss given the existing tmux-backed design.

## 3. Sidebar kanban UI — buildable, one architectural rule

A real kanban board (columns, drag-between-lanes) needs a `WebviewView`, not `TreeView` (rows only, no columns). Anthropic's own Claude Code extension already does exactly this for its own sidebar panel (`registerWebviewViewProvider`). A full React app works inside it; the real constraints are (source: [Webview API guide](https://code.visualstudio.com/api/extension-guides/webview)):

- All extension↔webview traffic is async `postMessage` with JSON-serializable payloads — fine for card-status deltas at human tempo, bad for high-frequency streaming (batch/coalesce).
- CSP should be `default-src 'none'` plus nonce'd scripts from `localResourceRoots` — a bundled React app is fine, CDN imports/`eval` are not.
- **Don't open the websocket/polling connection inside the webview.** Keep it in the extension host (real Node, no CSP, not torn down when the view is hidden) and relay state deltas via `postMessage`. A webview's connection dies whenever the view is hidden unless `retainContextWhenHidden: true`, which carries "high memory overhead" per VS Code's own guidance.
- Sidebar width (~300–500px) is the real limiter, not performance. Practical split: sidebar = compact vertical card list with status badges; a `WebviewPanel` editor tab = the actual multi-column board, with click-to-navigate via `vscode.window.showTextDocument`/the deep-link support already planned in the PRD (Section 3.8).

## 4. Controlling the Claude Code extension — dead end, and it reshapes the design

This is the one finding that changes how the whole feature should be designed, not just a caveat. Direct inspection of the installed extension bundle (`anthropic.claude-code-2.1.241-darwin-arm64`) plus its documented deep-link API confirms:

- **No exported API** — `activate()` returns nothing, so `getExtension('anthropic.claude-code').exports` is `undefined`. No `api` field, no `enabledApiProposals`.
- **24 public commands, but the one that matters (`claude-vscode.editor.open(sessionId?, prompt?, viewColumn?)`) hardcodes `cwd` to `workspaceFolders[0]`** (confirmed in `extension.js`: `setupPanel` → `realpathSync(i[0] || homedir())`). It's impossible to tell the extension "run this session against worktree X" — it only ever knows about "the folder this window has open." There's also a documented gotcha: calling it when a session is already open shows *"Session is already open. Your prompt was not applied,"* rather than starting a new one.
- **No status readback at all** — nothing tells an external caller whether a session finished, is waiting on input, or failed. The "N waiting for input" indicator is internal statusbar-only.
- The officially documented deep link `vscode://anthropic.claude-code/open?prompt=<urlencoded>&session=<id>` only pre-fills a prompt — it does not submit it automatically (source: [Claude Code VS Code docs](https://code.claude.com/docs/en/vs-code)).
- `~/.claude/ide/<port>.lock` (`{pid, workspaceFolders, transport:"ws", authToken}`) is the extension acting as an MCP *server* exposing `openDiff`/`getDiagnostics`/`openFile` **to** the CLI — it drives the IDE, the IDE doesn't drive it back.
- Community requests for a real API exist ([anthropics/claude-code#5495](https://github.com/anthropics/claude-code/issues/5495), [#8727](https://github.com/anthropics/claude-code/issues/8727)) with no committed Anthropic response.

More generally, cross-extension interaction in VS Code is bounded to exactly two channels, both requiring the *target* extension's cooperation: `extensions.getExtension(id).exports` (only what `activate()` explicitly returns) and `commands.executeCommand(id, ...)` (only for commands the target extension chose to register publicly, with a private/unversioned argument contract). There is no API to read another extension's internal state, webview contents, or UI without that cooperation.

The upshot: don't design around "orchestrating the Claude Code extension." Drive the real `claude` CLI directly — headless (`claude -p --output-format stream-json`) or headful in a Pseudoterminal-backed terminal (Section 2) — exactly what the Conveyor PRD's `WorktreeProvider` interface already does (`kanban-board-prd.md` Section 3.5–3.7). That design already sidesteps this dead end structurally; it just wasn't framed as "avoiding extension control" explicitly until now. At most, add one "Open in Claude Code" card action using the documented deep link as a convenience — never as the control plane, and never assume it can target a different worktree than whatever the window currently has open.

## 5. Prior art — nobody else does in-place swapping, and the real competitive bar has already moved past "IDE plugin"

### Existing "git worktree" VS Code extensions (Marketplace install counts)

| Extension | Installs | Switch model |
|---|---|---|
| [jackiotyu/git-worktree-manager](https://github.com/jackiotyu/git-worktree-manager) | ~31,000 | New window by default; also "Add Folder to Workspace" |
| [alexiszamanidis/vscode-git-worktrees](https://github.com/alexiszamanidis/vscode-git-worktrees) | ~23,000 | New window (`openNewVscodeWindow` defaults `true`) |
| [philstainer/git-worktree](https://github.com/philstainer/git-worktree) | ~9,700 | Opens folder, new or current window |
| [CodeInKlingon/vscode-git-worktree](https://github.com/CodeInKlingon/vscode-git-worktree) | ~5,600 | "Open worktree in separate window" |

**Nobody swaps in-place.** Every one shells out to `git worktree` then calls `openFolder` or `updateWorkspaceFolders` with a full reload. Friction complaints in their issue trackers are about state loss on reload (profile resets, terminal loss, "not opened as a workspace" — [CodeInKlingon#21](https://github.com/CodeInKlingon/vscode-git-worktree/issues/21), [jackiotyu#25](https://github.com/jackiotyu/git-worktree-manager/issues/25)), not about switch speed in absolute terms — users have collectively normalized "reload is the cost of switching projects." [`alefragnani/project-manager`](https://github.com/alefragnani/vscode-project-manager) (7.5M installs) still just opens a folder, and its own issues report 10+ second sidebar-appear delays ([#264](https://github.com/alefragnani/vscode-project-manager/issues/264)) that users tolerate.

### The competitive floor has already moved past "VS Code extension"

The tools that actually solved "many parallel AI agents, switch between them fast" did it by **decoupling from the editor window entirely** — a separate lightweight process-supervisor UI (often literally a local web app, i.e. worktree-studio's current shape) that treats the editor as a disposable, swappable view rather than the home of the switching logic:

- **[Vibe Kanban](https://github.com/BloopAI/vibe-kanban)** — 27.9k GitHub stars, server + web UI, worktree per card. The closest existing analog to Conveyor's own design.
- **Conductor** — YC-backed, $22M raised, same "worktree per task, watch it in a UI" model.
- **[claude-squad](https://github.com/smtg-ai/claude-squad)** (8.4k stars), **[Crystal](https://github.com/stravu/crystal)** (3.1k stars) — same pattern.
- **Cursor 3.0's "Agents Window"** is the one real counterexample that tried to do this *inside* the editor — and its own community forum already has complaints about layout thrash on mode switch ([forum thread](https://forum.cursor.com/t/166937)).
- **Claude Code itself** now ships native `--worktree` support and the desktop app auto-worktrees each session ([docs](https://code.claude.com/docs/en/worktrees)).
- **JetBrains 2026.1** shipped native worktree support directly in the IDE ([IJPL-204771](https://youtrack.jetbrains.com/issue/IJPL-204771)).

### The Arduino analogy is a mismatch

No prior art exists for applying it here, and the reason is structural, not incidental: Arduino's board/port dropdown swaps *compile-time metadata* — no path, no LSP root, no process identity changes. A worktree switch changes every absolute path the window is rooted at, forcing VS Code to relocate workspace-scoped storage, re-root language servers, and re-key file watchers. The Arduino pattern is achievable only for the *presentation layer* (a status-bar dropdown that triggers the underlying folder swap) — it can never be the mechanism that makes the swap itself cheap, because the underlying operations aren't the same class of change.

---

## 6. Realistic recommendation

Build it, but scope it honestly:

1. **The instant-switch feature is real and worth building** — anchor-folder + non-first-slot `updateWorkspaceFolders`, following the pattern `vscode-git-worktree-switcher` has already shipped in production. Budget for the multi-root-workspace UX cost (a permanent stub folder, explorer clutter to manage), not just the switch latency.
2. **Reuse ~90% of worktree-studio's existing tmux backend as-is** via a custom `Pseudoterminal` bridge — this is the strongest part of the pitch, since it's mostly wiring onto an already-working backend, not new engineering, and it gets real editor-tab terminals instead of a sidebar panel.
3. **Build the kanban UI as a `WebviewView` + `WebviewPanel` pair**, keeping all networking in the extension host, never inside the webview itself.
4. **Drop "controlling the Claude Code extension" from the design entirely.** Drive `claude`/`claude -p` directly, exactly as the Conveyor PRD's `WorktreeProvider` already plans — this was already the right call, now confirmed correct for the extension-specific reason too (its hardcoded single-workspace-folder `cwd` makes it structurally incapable of targeting a specific worktree anyway).
5. **Go in with eyes open that the strongest prior art (Vibe Kanban, Conductor, Crystal) chose *not* to be VS Code extensions** — they stayed exactly where worktree-studio already is (a standalone local web app). The genuine unique win of doing this as an extension instead is inheriting VS Code's real editor (LSP, debugger, GitLens, etc.) in place of CodeMirror — that's a real reason to do it, but it's a narrower win than "instant switching" alone would suggest, and it's worth being explicit about whether that's the actual goal before committing engineering time to the multi-root-workspace plumbing this requires.
