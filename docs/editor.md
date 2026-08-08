# In-browser editor (CodeMirror-backed)

See `docs/editor-plan.md` for the design rationale (why CodeMirror instead of Monaco, the layered adapter architecture, and the full pitfall list this implementation was built against). This doc covers what actually shipped.

## What it is, and isn't

A light, syntax-highlighted, non-modal editor for quick touch-ups in a worktree — not an IDE. `vim`/`nano`/anything else is always one keystroke away in the same worktree's terminal; this exists for the gap terminal editors don't fill well. For anything beyond light editing, the "💻 VS Code" toolbar button shells out to the real VS Code (`code <worktree-path>`) rather than growing this editor into one.

## Layered editor engine

`web/src/editors/` defines a small adapter interface (`EditorAdapter.ts`) that any editing engine implements as a plain React component. `registry.ts` maps a `kind` string to an adapter component — today just `{ codemirror: CodeMirrorEditor }`. `EditorPanel.tsx` (the dockview panel shell) only ever imports from `EditorAdapter.ts`/`registry.ts`, never from `codemirror`/`@codemirror/*` directly, so:

- Swapping the engine later means writing a new adapter component and changing one line in `registry.ts` — no changes to `EditorPanel.tsx` or `FileTree.tsx`.
- Supporting more than one engine at once (a per-user or per-file-type choice) is a registry with more than one entry — the picker UI for that doesn't exist yet (deliberately: building a dropdown for a one-item registry would be premature), but the seam is already there.

Each adapter is **uncontrolled after mount** — it takes `content` once and reports changes via `onChange`, the same way a native `<textarea defaultValue>` works, not a controlled `<input value>`. To reset an adapter to different content (accepting an external-change reload, see below), the caller remounts it via a changed React `key` — the same pattern `WorktreeDetail.tsx` already uses to reset dockview state when navigating between worktrees.

`CodeMirrorEditor.tsx` is the only place CodeMirror-specific code exists: its extensions, the `vscodeDark` theme (`@uiw/codemirror-theme-vscode`), and language-grammar resolution via `@codemirror/language-data`'s `LanguageDescription.matchFilename` (covers 100+ languages/conventional filenames like `Dockerfile` for free, dynamically imported per-language so the initial bundle doesn't pay for languages that are never opened — confirmed via `bun run build`'s output, which code-splits each language into its own lazy chunk).

## Backend: `internal/files`

- `ListTree(worktreePath)` — `git ls-files` (tracked) unioned with `git ls-files --others --exclude-standard` (untracked, not gitignored), nested into a directory tree server-side.
- `ReadFile`/`WriteFile(worktreePath, relPath, ...)` — every call goes through `ResolvePath`, which rejects any `relPath` that would resolve outside `worktreePath` (`ErrPathEscapesWorktree`) — this is the first place the app takes an untrusted filesystem path from the browser, so this check is load-bearing. `ReadFile` also rejects files over `MaxFileSize` (5MB) and the API layer rejects non-UTF-8 content (binary files aren't meant to open here — use VS Code instead).
- REST: `GET .../files/tree`, `GET/PUT .../files/content?path=...`, `POST .../open-in-vscode`.

## External-change push (fsnotify)

`internal/files.Watcher` watches a worktree's tree recursively (skipping `.git`) and reports changes over `GET /ws/files/:worktreeId` as `{"type":"changed","path":"..."}`. Two things this had to get right, both found and fixed via real end-to-end verification (a raw ws client + a live server), not just unit tests:

- **Debouncing**: a burst of raw OS events for one logical change (a `git checkout`, an editor's own save) collapses into one notification per path (300ms window), not one per raw event.
- **Own-write suppression**: a save made through `PUT .../files/content` must not come back around as a false "changed externally" push to the tab that just made it. `internal/files.Manager` shares one `Watcher` per worktree across every ws subscriber (so the suppression state isn't duplicated per tab) and `MarkOwnWrite` is called **before** `WriteFile`, not after — a real race was found here during manual verification: the OS can emit (and the watcher can process) the fsnotify event as soon as the write syscall completes, which can beat whatever the handler does next if suppression is marked afterward. `internal/api.TestPutFileContentSuppressesOwnWriteEvent` is a regression test for this specific ordering (confirmed to fail reliably with the order reversed before being fixed).

`EditorPanel.tsx` opens one ws connection per open file (mirrors `Terminal.tsx`'s one-connection-per-panel pattern rather than a shared multiplexed channel — same simplicity call already made for terminals). On a change event for its own path: if the buffer is clean, it reloads silently; if dirty, it shows a banner ("this file changed on disk — Reload / Keep editing") rather than either silently discarding unsaved edits or silently ignoring a real external change.

## One buffer per file, per worktree

`WorktreeDetail.tsx`'s `handleOpenFile` reuses an already-open panel for a path instead of creating a second one (`dockviewApi.getPanel(id)` check, panel `id = "editor:" + path`), so there's never more than one buffer open on the same file at once — this is what makes "what if two panels edit the same file and diverge" a non-question by construction, rather than something needing a shared-model design.

## Save

Explicit save only — no autosave. Cmd/Ctrl+S is bound inside `CodeMirrorEditor.tsx` via a CodeMirror keymap that both calls the adapter's `onSaveRequested` callback and returns `true` (not just `preventDefault`), which is what stops the browser's native "Save Page As" from firing for that key.

## VS Code escape hatch

`POST .../open-in-vscode` shells out to `code <worktree-path>`. `code` requires a one-time manual "Shell Command: Install 'code' command in PATH" step even when VS Code itself is installed, so this can fail on an otherwise normal machine — the frontend checks `GET /api/settings/dependencies`'s `vscode_cli` entry (same detection pattern already used for tmux/spotlight in `SettingsModal.tsx`'s Installation tab) and disables the toolbar button when it's not detected, with a real error surfaced (not a silent no-op) if the exec call still fails despite the check passing.

## Not built (explicitly out of scope for this pass)

Git diff view, inline review comments, "send to agent" — `PLAN.md` step 4b, its own follow-up once this lands. Autosave. An engine picker UI (the registry supports more than one engine; nothing built that lets a user choose yet).
