# Terminal copy/paste: how it works and why

Kept separate from `docs/architecture.md` on purpose — this is deep, low-level xterm.js/tmux mechanism detail that's rarely needed (the 80% you skip), not the core architecture every session needs to re-read (the 20% that matters). Read this only when copy/paste in a terminal panel is broken or behaving unexpectedly.

## Problem 1: Ctrl+C/Ctrl+V did nothing at all

**Symptom** (as reported): in a plain shell inside a terminal panel, selecting text and pressing Ctrl+C copied nothing; Ctrl+V pasted nothing.

**Root cause**: xterm.js's own key handling calls `preventDefault()` on virtually every keystroke it translates into pty bytes — including Ctrl+C, which needs to reach the shell as a real `0x03`/SIGINT byte for a terminal to work at all. That same `preventDefault()` also blocks the browser's native copy action from ever firing. This is a known, common gap in xterm.js integrations, not a regression — the embedding app is expected to wire this up itself.

**Fix** (`Terminal.tsx`, `term.attachCustomKeyEventHandler`):
- Ctrl+C/Cmd+C: if there's an active `term.getSelection()`, copy it via `navigator.clipboard.writeText` and suppress the keystroke (`return false`) so it doesn't *also* send a literal SIGINT. With no selection, let it through as a normal interrupt — this is what keeps Ctrl+C at a running prompt working exactly as before.
- Ctrl+V/Cmd+V: read the clipboard explicitly (`navigator.clipboard.readText()` → `term.paste(text)`) rather than relying on xterm's own native-`paste`-event wiring (which does exist — a listener on its hidden textarea — but firing *in addition to* explicit handling would double-paste; see the bug below).
- The key-combo decision logic (`classifyTerminalKeyEvent`, exported from `Terminal.tsx`) is a small pure function extracted specifically so it's unit-testable without a real browser (`Terminal.test.tsx`) — the clipboard/xterm glue itself isn't testable in this environment (no browser automation tool connected).

**A bug found immediately after, while explaining the design** (not caught by the test suite — pure key-combo classification tests can't see this): the paste branch returned `false` from `attachCustomKeyEventHandler` but never called `event.preventDefault()`. Returning `false` only skips xterm.js's *own* keydown→bytes translation — it does not suppress the browser's native default action. Since xterm.js already has its own listener for the native `paste` ClipboardEvent (a separate code path entirely, not gated by `attachCustomKeyEventHandler`), leaving the keydown un-prevented meant that listener could still fire alongside the explicit `clipboard.readText()`/`term.paste()` call, pasting the clipboard twice. Fixed by adding `event.preventDefault()` to the paste branch. (The copy branch doesn't need it — a redundant native copy of the exact same selection text is harmless.)

## Problem 2: copy still didn't work *inside* a `claude` session (though it worked in a plain shell)

**Symptom**: the fix above worked for a plain shell, but selecting text inside a running `claude` session and pressing Ctrl+C still copied nothing.

**Root cause, confirmed empirically, not guessed**:
1. `strings` on the `claude` binary shows `[?1000h`/`[?1006h` (mouse-tracking-enable escape sequences) and `mouseTrack` identifiers — `claude`'s TUI turns on mouse reporting, presumably for its own scroll/click UI.
2. Reading `@xterm/xterm`'s own source directly confirms the consequence:
   ```js
   this.coreMouseService.areMouseEventsActive
     ? this._selectionService.disable()
     : this._selectionService.enable();
   ```
   When an app has mouse tracking active, xterm.js **completely disables its own text-selection mechanism** — there's no modifier-key override in this version (`@xterm/xterm` v6). So there was never a selection for Ctrl+C to act on in the first place; this is a different, deeper issue than Problem 1, not a regression of the Problem 1 fix.

This isn't specific to `claude` — any TUI that enables mouse tracking (`vim`, `less -R`, `fzf`, `tmux` itself) has the exact same effect. Every terminal emulator, native or web, has to solve this the same way (there's no way to know "the user wants to copy their mouse selection" vs. "the running program wants this exact mouse event" except by checking, at the moment of interaction, whether a selection exists — information that only exists in the terminal-emulator layer, never in the byte stream the shell/tmux/claude ever see).

**Considered and rejected**: stripping the mouse-tracking-enable escape sequences before they reach xterm.js, so it never learns an app asked for mouse mode. Rejected because it would silently take away any real mouse-driven UI feature `claude`'s TUI has (not verified how much it has, if any) — trading "can't copy" for "can't click things in claude," not actually removing the tradeoff.

**Fix chosen: tmux copy-mode + OSC 52 clipboard passthrough.** This is the mechanism actually designed for "a program grabbed my mouse" — it operates at the tmux layer, entirely independent of whether the program in the pane has mouse tracking on, since tmux *is* the terminal from that program's point of view and copy-mode suspends forwarding input to it while active.

- **`internal/term.Manager.CreateSession`** now runs `tmux set-option -g set-clipboard on` right after creating each session. This is a tmux **server** option (no per-session scope exists) — it affects every tmux session on the machine, including ones the user created outside worktree-studio, which is deliberate: it only enables OSC 52 relay (nothing about keybindings or mouse behavior changes) and is a commonly-recommended tmux setting on its own merits. Best-effort — a failure here doesn't affect the session's usability as a shell, only whether copy-mode's clipboard integration works.
- **`Terminal.tsx`** loads the official `@xterm/addon-clipboard` (`ClipboardAddon`), which registers an OSC 52 handler and, on receiving one, writes the decoded text to the browser clipboard via its default `BrowserClipboardProvider` (`navigator.clipboard`). Requires `allowProposedApi: true` on the `Terminal` constructor — `registerOscHandler`, which the addon uses internally, is gated behind that flag in this xterm.js version.
- **How to actually copy now, inside something like `claude` that's grabbed the mouse**: enter tmux's copy-mode (default prefix `Ctrl+b` then `[`, or whatever prefix is configured), move/select with arrow keys or vi-style `hjkl`+`v`, then `Enter` (or `y` in vi mode) to copy and exit copy-mode. That copy triggers tmux's OSC 52 emission, which the addon catches and writes to the real clipboard — independent of whatever the running program's own mouse mode is doing.
- **Real, empirical verification performed** (not just "should work" reasoning): created a throwaway tmux session, confirmed `set-clipboard` was `off` beforehand, ran `internal/term.CreateSession`'s equivalent command and confirmed it flipped to `on`. Separately, attached a real pty (Python's `pty` module, mirroring exactly what `creack/pty` does server-side) to a throwaway tmux session with `set-clipboard on`, drove `tmux copy-mode` + `select-line` + `copy-selection-and-cancel` via `tmux send-keys -X`, and captured the raw bytes coming out of that pty — confirmed a genuine `\x1b]52;;<base64>\x07` sequence, and decoded the base64 to confirm it was exactly the selected line's text. This proves the tmux-side half of the mechanism for real, not by assumption. The receiving half (`@xterm/addon-clipboard`'s OSC 52 handling → `navigator.clipboard.writeText`) is the officially-maintained xterm.js team addon's own responsibility, not custom code here — confirmed present in the built bundle (`registerOscHandler` string) but not click-through verified in an actual browser (no browser automation tool connected this session).

## A related, NOT-yet-tried option worth knowing about

While reading xterm.js's default options, found `macOptionClickForcesSelection: false` — a real xterm.js constructor option, currently unset/default here. Its docs describe forcing selection when the Option key is held on macOS. **Not implemented, not verified** whether it would actually override `_selectionService.disable()` when mouse tracking is active (the source shows that disable happening unconditionally when `areMouseEventsActive` is true, so this option may only affect a different, narrower ambiguity and not help here at all) — flagged here as a candidate to investigate later, not a promise it works.

## If copy/paste still doesn't work after all this

1. Confirm `tmux show-options -g set-clipboard` actually reports `on` on the machine running the server — if a terminal was created before this feature existed (i.e., an old tmux session from before this fix shipped), the option only gets set the *next* time `CreateSession` runs, not retroactively for already-running sessions (though since it's a server-wide option, creating even one *new* terminal anywhere turns it on for all existing sessions too).
2. Confirm the browser actually granted clipboard permission — `navigator.clipboard.writeText`/`readText` can silently fail if permission was denied; check the browser's own site-permission UI for `localhost:8787` (or whatever port).
3. Remember Problem 2's fix requires using tmux's copy-mode keyboard shortcut, not mouse-drag, while inside a program that has grabbed the mouse (`claude`, `vim`, etc.) — mouse-drag selection is expected to not work there at all, by design, per Problem 2's root cause above.
