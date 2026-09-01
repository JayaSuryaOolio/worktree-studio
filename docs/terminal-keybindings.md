# Terminal word-navigation keybindings: how they work and why

Kept separate from `docs/terminal-clipboard.md` on purpose — same "deep, low-level xterm.js/tmux mechanism" shelf, but a different mechanism (key encoding, not the clipboard/mouse-tracking pipeline). Read this only when a keyboard shortcut inside a terminal panel does the wrong thing (or nothing).

## Symptom (as reported)

Ctrl+Left/Ctrl+Right (word navigation) and other Emacs/readline-style shortcuts didn't work in a terminal panel, in contrast to a native terminal app (e.g. Warp) using the same shell/dotfiles.

## The three independent links in the chain

A modified arrow key has to survive three separate translation steps between a browser keydown and the shell's readline/zle actually recognizing it as "move by word." Any one of them silently swallowing the modifier looks identical from the outside ("the arrow key just moves the cursor one character").

1. **Browser → xterm.js.** xterm.js's own default keymap already emits the standard `\x1b[1;5C` / `\x1b[1;5D` CSI sequences for Ctrl+Right/Ctrl+Left — no custom handler was needed for that part, and `Terminal.tsx`'s `attachCustomKeyEventHandler` doesn't (and shouldn't) special-case it. The one real gap here is **macOS Option**: by default, Option+key produces a special Unicode character (Option+B → "∫"), not the ESC-prefixed `\x1bb`/`\x1bf` sequence Emacs-style bindings expect — xterm.js only sends that sequence for Option if told `macOptionIsMeta: true`, which wasn't set. Fixed in `Terminal.tsx`'s `XTerm` constructor options.

2. **xterm.js → tmux.** Input never goes through `tmux send-keys` for live typing (see `internal/term/attach.go`) — it's written directly into the pty master fd of a `tmux attach-session` process, so whatever bytes xterm.js produced arrive at tmux exactly as sent. But tmux itself then has to correctly interpret and re-forward those bytes to the pane's actual shell, which depends on two things that weren't previously set anywhere in this codebase:
   - **tmux's `xterm-keys` option** (global, default-on since tmux 2.4, but never explicitly set here — left implicit). Without it, tmux can collapse a modified arrow-key sequence down to a bare one before the shell ever sees it, silently dropping the Ctrl/Alt modifier. Now set explicitly in `CreateSession` (new sessions) and `CorrectGlobalKeyEncodingSettings` (existing installs, called once at startup — same idiom as `CorrectGlobalMouseAndPassthroughSettings`), alongside `extended-keys on` (tmux 3.2+, improves fidelity for combinations `xterm-keys` alone doesn't fully disambiguate).
   - **The outer `TERM` tmux's attaching client presents.** `internal/term.Attach` runs `tmux attach-session` under a fresh pty via `pty.Start`, and until this fix, that process inherited whatever `TERM` the Go server's own process happened to have — a login shell, a background/launchd context, an editor task runner, all different, none guaranteed to be one whose terminfo entry actually describes xterm-style modified-key sequences. `Attach` now forces `TERM=xterm-256color` on that one process via `attachEnv()` regardless of how `worktree-studio` itself was launched, so tmux's `xterm-keys` translation always has a terminfo it can rely on.

3. **tmux → the shell's readline/zle.** Even with the modifier surviving both hops above, the shell still has to be bound to *do* something with `\x1b[1;5C`/`\x1b[1;5D` — this is **not** automatic or hardcoded; it's ordinary shell configuration (`bindkey '^[[1;5C' forward-word` in `~/.zshrc`, or `.inputrc`/`.editrc` for bash), same as it would need to be in any other terminal emulator. If word-navigation still doesn't work after the two fixes above, check whether the worktree's shell config actually binds these sequences at all (`bindkey | grep '\[1;5'` in zsh) — that's a dotfiles question, not something worktree-studio can or should override on someone's behalf.

## Fix summary

- `web/src/Terminal.tsx`: `macOptionIsMeta: true` on the `XTerm` constructor.
- `internal/term/term.go`: `xterm-keys on` / `extended-keys on` set in `CreateSession` (new sessions) and a new `CorrectGlobalKeyEncodingSettings()` (existing tmux servers, called from `main.go` at startup).
- `internal/term/attach.go`: `Attach` now forces `TERM=xterm-256color` on the `tmux attach-session` process it spawns, via a new `attachEnv()` helper, instead of inheriting the Go server's own ambient `TERM`.

## Deliberately out of scope

Rebinding word-navigation (or any other shortcut) inside the shell itself — that's the user's own dotfiles, same as in any other terminal emulator. This fix is only responsible for the modifier surviving the browser→xterm.js→tmux→shell pipeline intact; whether the shell then does something useful with it is between the user and their `.zshrc`/`.inputrc`.
