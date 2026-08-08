# In-browser editor — implementation plan

Status: **implemented.** Written before build per this project's convention (see `docs/ui-overhaul-plan.md` for precedent); kept as the historical design record. See `docs/editor.md` for what actually shipped and `docs/architecture.md`'s "In-browser editor" section for the current architecture.

## Scope, restated from direct user instruction

- **Not core, a feature.** The editor is an optional convenience layered on top of what already works — `vim`/`nano`/anything else is always one keystroke away in the existing terminal, and stays the answer for anyone who wants it. The in-browser editor exists for the gap terminal editors don't fill well: **non-modal, syntax-highlighted, mouse/arrow-navigable light editing** for someone doing quick touch-ups between AI-driven changes, not learning vim's modality for it.
- **Editor engine: CodeMirror 6**, not Monaco. Chosen specifically to avoid Monaco's Vite web-worker bundling fight and its heavier footprint — CodeMirror's syntax highlighting runs off plain Lezer grammars, no dedicated worker threads required for the base case. This directly removes what was flagged as the single highest-risk pitfall in the original (Monaco) version of this plan.
- **VS Code Dark theme for CodeMirror**: yes, supported — `@uiw/codemirror-theme-vscode` ships `vscodeDark`/`vscodeLight` themes built for CodeMirror 6 (confirm exact package/version against installed `@codemirror/*` core versions at implementation time; it's a well-established third-party theme package, not something to hand-roll). This also directly answers pitfall #10 from the original plan (Monaco's default theme clashing with Command Deck) — ship `vscodeDark` from day one, no redo needed later.
- **Layered so the engine is swappable, and multiple engines can coexist.** Direct instruction: don't hard-wire CodeMirror into `Editor.tsx`/the panel plumbing the way `Terminal.tsx` hard-wires xterm.js. Instead, define a small **editor adapter interface** that any engine (CodeMirror today, Monaco or something else later, even more than one live at once) implements, and have the panel shell depend only on that interface. This is a direct, non-speculative request — build it now, not as a "just in case" abstraction.
- **Complex editing escape hatch unchanged**: a button that shells out to `code <worktree-path>` for anything beyond light editing.
- **File tree sidebar**: yes, wanted now. **Git diff view**: not now, still `PLAN.md` step 4b, design so it slots in later.

## Editor abstraction (the layering the user asked for)

```
web/src/editors/
  EditorAdapter.ts       # the interface every engine implements
  registry.ts            # kind -> adapter component, e.g. { codemirror: CodeMirrorEditor }
  CodeMirrorEditor.tsx    # default/only adapter for v1
  (MonacoEditor.tsx)      # not built now — the seam this abstraction exists for
```

- `EditorAdapter` is a React component contract, not a class hierarchy — matches how `Terminal.tsx`/`TerminalPanel` already work in this codebase, just parameterized instead of singular:

  ```ts
  interface EditorProps {
    content: string;
    language: string;        // derived from file extension, engine-agnostic
    theme: "vscode-dark";    // the one theme for now; a real union once a second theme exists
    onChange: (value: string) => void;
    onSaveRequested: () => void; // engine calls this on its own save keybinding (Cmd/Ctrl+S)
  }
  type EditorAdapter = React.ComponentType<EditorProps>;
  ```

- `registry.ts` is a plain object map, deliberately mirroring the existing `const components = { terminal: TerminalPanel }` pattern in `web/src/WorktreeDetail.tsx` — same shape, same place in the codebase's mental model, not a new pattern to learn.
- The **editor panel shell** (`EditorPanel.tsx`, the dockview panel component, analogous to today's `TerminalPanel`) owns: fetching file content, tracking dirty state, calling the save API, computing `language` from the file extension, and picking which adapter to render from the registry. It knows nothing about CodeMirror specifically — swapping the registry's `codemirror` entry for a different engine, or adding a second entry a user can pick between (e.g. a per-file or global settings choice), touches zero code in the shell.
- **Multiple engines coexisting** falls out of this for free: the registry can hold more than one adapter at a time, and "which adapter does this panel use" is just a value (a setting, a per-file-type default, a per-user preference) rather than a compile-time choice — worth deferring the actual *picker UI* until there's a second real adapter to pick between (adding a settings dropdown for a one-item registry is the premature part, not the registry itself).
- This does mean the CodeMirror-specific pieces (extensions, the vscodeDark theme, language grammars) live entirely inside `CodeMirrorEditor.tsx` and nowhere else — no CodeMirror import should ever appear in `EditorPanel.tsx`, `FileTree.tsx`, or `WorktreeDetail.tsx`. That boundary is the actual deliverable of "layered for swapping," not just a documentation note — worth a lint rule or at least a code-review checkpoint on that specific boundary before calling this step done.

## Where this plugs into the existing architecture

- `WorktreeDetail.tsx` already hosts an extensible dockview grid where `Terminal.tsx` is one panel component in a `components` map. `EditorPanel` becomes a second entry (`{ terminal: TerminalPanel, editor: EditorPanel }`), reusing the same panel/layout/persistence machinery for free.
- File tree sidebar is a new left-hand panel inside `WorktreeDetail` (persistent, not a modal — same call as the step-7 sidebar).
- New Go package `internal/files` (already named in `PLAN.md`), REST endpoints under `/api/repos/:repoId/worktrees/:worktreeId/files...`, following the existing REST-not-ws pattern (spotlight/status/layout all poll; there's no proven need for push on read/write itself).
- fsnotify-driven "changed on disk" push is the one genuinely event-driven piece — reuse the ws-endpoint-per-resource pattern from `internal/term`, not a new protocol shape.

## Backend: `internal/files` (unchanged by the engine swap — this is all engine-agnostic)

- `ListTree(worktreePath string) ([]FileNode, error)` — `git ls-files` (tracked) unioned with `git ls-files --others --exclude-standard` (untracked, not ignored), same pattern `internal/gitops.Status` already uses for tracked/untracked. Build the nested tree server-side so `FileTree.tsx` stays a pure render component.
- `ReadFile`/`WriteFile(worktreePath, relPath string, ...)` — plain `os.ReadFile`/`os.WriteFile` scoped under the worktree root.
  - **Path traversal is a real risk**: `relPath` comes straight from client input. Resolve `filepath.Join(worktreePath, relPath)` and verify the result is still lexically under `worktreePath` before touching the filesystem, every time — this is the first place the app takes untrusted filesystem paths from the browser (git/tmux/spotlight all operate on server-constructed paths). Treat with the same seriousness as `handleAddRepo`'s existing absolute-path rejection, and write a deliberate `../` traversal test case.
  - Cap read size (refuse/truncate past a few MB) — matters for browser-side editor responsiveness regardless of which engine renders it.
- `WatchWorktree(worktreePath string, changes chan<- FileChangeEvent) (stop func(), err error)` — one `fsnotify.Watcher` per open worktree, same lifecycle scope as a terminal session.
- REST surface (mirrors `terminals.go`/`status.go`):
  - `GET .../files/tree`, `GET/PUT .../files/content?path=...`, `GET /ws/files/:worktreeId` (push `{"type":"changed","path":...}`), `POST .../open-in-vscode`.

## Pitfalls

Updated for the CodeMirror pivot — several Monaco-specific risks from the original draft are gone; the ones that remain are engine-agnostic (they'd apply to any embedded browser editor) plus one new one from the abstraction itself.

1. **Dockview panel remount/reuse across worktree navigation.** Same class of bug already hit and fixed for terminals (`WorktreeDetail`'s `key={repoId:worktreeId}` fix, see `PROGRESS.md`'s "Post-step-7.5 fixes" entry) — the fix already covers new panel types, but verify with the same round-trip discipline (w1→w2→w1, not just w1→w2, since dockview hides rather than unmounts and a one-hop test can pass on buggy code). Watch specifically for two worktrees that both have a `src/index.ts` — the editor's internal state must be keyed by something that includes the worktree, not just the relative path, or worktree B's panel can open holding worktree A's in-memory content.

2. **CodeMirror + Vite**: much lower risk than Monaco was, but still verify before committing — CodeMirror 6 is a pure-ESM package tree with no worker requirement for the base editing/highlighting experience, which is exactly why it was chosen, but confirm the actual language-grammar packages picked (e.g. `@codemirror/lang-javascript`, `@codemirror/lang-go`) tree-shake cleanly under this project's existing Vite config rather than assuming zero-config just because the worker problem specifically is gone.

3. **Theme package version compatibility.** `@uiw/codemirror-theme-vscode` (or whichever VS Code Dark theme package is used) pins against specific `@codemirror/*` core versions — verify compatible versions resolve cleanly in `bun install` before building on top of it, the same kind of check already habitual in this project for any new dependency (e.g. the `xterm`/`@xterm/addon-*` version pinning already in `web/package.json`).

4. **Keybinding ownership fights: browser vs. OS vs. editor** — the direct parallel to tmux's `mouse on` tradeoff (`docs/terminal-clipboard.md`). Less severe with CodeMirror's non-modal defaults than it would've been layering custom bindings onto Monaco, but still real:
   - Cmd/Ctrl+S is the browser's native "Save Page As." The adapter's `onSaveRequested` callback needs a CodeMirror keymap binding that calls `preventDefault` reliably — verify it fires correctly even right after clicking into the editor from the file tree (a focus-timing edge, not just a binding-exists check).
   - Cmd/Ctrl+W, Cmd/Ctrl+N are not interceptable at all (full browser reservations) — don't map "close panel"/"new file" onto them.
   - Document explicit "editor owns this key, full stop" vs. "best-effort" bindings up front rather than discovering the boundary one bug report at a time.

5. **fsnotify external-change vs. in-editor-unsaved-changes conflict.** A file changing on disk (a `claude` terminal in the same worktree editing it, a `git checkout`) while it's open with unsaved edits must not silently clobber the buffer. `PLAN.md`'s "file changed on disk, reload?" is the right shape: fsnotify event → check the panel's dirty flag client-side → prompt only if dirty, silently sync if clean. Debounce fsnotify events (a `git pull`/checkout/the editor's own save round-trip can fire several rapid events for one logical change).

6. **Own-write feedback loop.** A `PUT` from the editor's own save will itself trigger the fsnotify watcher on that same file. Tag "we just wrote this path within the last N ms" and suppress the resulting self-triggered event — same shape of problem as any file-sync tool watching a tree it also writes to (worth comparing against how `internal/spotlight`'s rsync-triggered fswatch loop handles, or doesn't handle, this already-present shape of bug in this codebase).

7. **"Open in VS Code" isn't universally available.** `code` (VS Code's CLI) requires a manual one-time "Install 'code' command in PATH" step inside VS Code — not guaranteed present. Follow `SettingsModal.tsx`'s existing Installation-tab pattern exactly (`exec.LookPath` detection + install hint, no auto-install on the user's behalf): detect once, disable/hide the button accordingly, surface a real error rather than a silent no-op if the exec call fails despite passing the check.

8. **Multiple panels open on the same file.** Decide up front: shared editing state between two open views of the same file (edits in one show live in the other) vs. two independent buffers that can silently overwrite each other's saves on save. Document the choice before building `EditorPanel.tsx`, not after finding it as a bug.

9. **Large/binary files.** `git ls-files` will list `.png`/`.woff2`/lockfiles/generated bundles alongside source. Use the same size/extension boundary from the read-size cap above to return "not previewable, open in VS Code instead" rather than shipping raw bytes into a text editor.

10. **The abstraction itself must not leak.** The one risk specific to the layering the user asked for: it's easy for the *first* adapter's assumptions (CodeMirror's specific extension/theme API shapes) to quietly become part of the "interface" instead of staying behind it, so that adding a second adapter later requires reshaping the interface anyway — defeating the point. Concretely: `EditorProps` above should never grow a field that's really "a CodeMirror `Extension[]`" or similarly engine-specific; if something CodeMirror-specific needs to be configurable, it's an option internal to `CodeMirrorEditor.tsx`; test the boundary for real by sketching (not necessarily building) what a second, meaningfully different adapter would need before finalizing `EditorAdapter.ts`.

## What's explicitly out of scope for this pass (confirmed with user)

- Git diff view / inline review comments / "send to agent" — `PLAN.md` step 4b, its own follow-up once this lands. `internal/gitops` already has what a diff view would need (`git show`, `git diff --name-status`) when that step starts.
- Multi-user / CRDT collab.
- Autosave — explicit save only for v1 (avoids hammering the write endpoint / fsnotify loop, matches "simple editor for light edits").
- An actual engine-picker UI (settings dropdown to choose CodeMirror vs. a future second adapter) — the registry supports it structurally, but building picker UI for a one-item registry is premature; add it when a second adapter is real.

## Build-order sketch (for when this moves from plan to implementation)

1. `internal/files`: tree/read/write with path-traversal guards + tests (including a deliberate `../` traversal test).
2. REST wiring (`internal/api`) + a manual curl-driven verification pass against a real worktree before touching the frontend, same discipline as every prior step.
3. `EditorAdapter.ts` + `registry.ts` + `CodeMirrorEditor.tsx` as a standalone spike (bare page, not yet in dockview) — proves the CodeMirror+Vite+vscodeDark combination works and exercises the adapter boundary (pitfall #10) before it's load-bearing.
4. `FileTree.tsx` + `EditorPanel.tsx` wired into `WorktreeDetail`'s dockview; multi-panel-same-file decision (pitfall #8) made and documented before this lands.
5. fsnotify watcher + ws push + dirty-check-gated reload prompt (pitfalls #5, #6).
6. "Open in VS Code" button + detection in `SettingsModal.tsx`'s Installation tab (pitfall #7), following the existing tmux/spotlight detection pattern exactly.
7. Docs: `docs/editor.md` (parallel to `docs/session-persistence.md`), append a section to `.claude/skills/worktree-studio/SKILL.md`, update `docs/architecture.md` (including the adapter-registry layering, since it's a new architectural pattern worth documenting for future engine additions).

Each numbered piece gets its own commit checkpoint per the standing policy in `PLAN.md`'s "Execution approach".
