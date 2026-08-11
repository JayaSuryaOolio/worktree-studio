import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { ClipboardAddon } from "@xterm/addon-clipboard";
import "@xterm/xterm/css/xterm.css";
import { terminalWsUrl } from "./api";

interface Props {
  terminalId: string;
  // Fired whenever the pty stream sets the terminal's window title via an
  // OSC 0/2 escape sequence (xterm.js's own onTitleChange) — see
  // terminalAppDetection.ts for what WorktreeDetail.tsx does with it.
  onTitleChange?: (title: string) => void;
}

export type TerminalKeyAction = "copy" | "paste" | "newline" | "pass";

/**
 * Decides what a keydown means for copy/paste, independent of xterm/
 * clipboard APIs so it's unit-testable without a real browser. xterm.js's
 * own key handling calls preventDefault() on virtually every keystroke it
 * translates into pty bytes (including Ctrl+C, which needs to reach the
 * shell as a real 0x03/SIGINT byte) — that preventDefault also blocks the
 * browser's native copy action from ever firing, which is why "select
 * text, press Ctrl+C" silently does nothing in a stock xterm.js terminal.
 * Handling both Ctrl and Cmd (metaKey) since this app runs on macOS too,
 * where Cmd+C/Cmd+V are the muscle-memory shortcut but Ctrl+C/Ctrl+V are
 * common habit for anyone coming from Linux/Windows or a plain terminal.
 *
 * Also classifies Shift+Enter as "newline": a real terminal profile
 * (iTerm2's `/terminal-setup`, VS Code's integrated terminal, etc.) remaps
 * Shift+Enter to send ESC followed by CR (`\x1b\r`) instead of a plain
 * `\r`, which is the sequence Claude Code's Ink-based input recognizes as
 * "insert a newline" rather than "submit". xterm.js has no such remap
 * built in — left alone it sends the same `\r` for Shift+Enter as for
 * plain Enter — so this mirrors that terminal-profile behavior at the
 * xterm layer instead.
 */
export function classifyTerminalKeyEvent(event: {
  type: string;
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
  key: string;
}): TerminalKeyAction {
  if (event.type !== "keydown") return "pass";
  if (event.shiftKey && event.key === "Enter") return "newline";
  if (event.shiftKey) return "pass";
  const mod = event.ctrlKey || event.metaKey;
  if (!mod) return "pass";
  const key = event.key.toLowerCase();
  if (key === "c") return "copy";
  if (key === "v") return "paste";
  return "pass";
}

/**
 * A real terminal, not an exec-and-capture box: xterm.js renders whatever
 * the server's pty->tmux attach sends over the websocket, and every
 * keystroke goes straight back over the wire. See PLAN.md section 3 — the
 * backing tmux session is what makes this survive a worktree-studio server
 * restart, not anything client-side.
 */
export default function Terminal({ terminalId, onTitleChange }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  // A ref, not a dependency of the effect below: onTitleChange is commonly
  // a fresh inline function on every parent render (see WorktreeDetail.tsx's
  // TerminalPanel), and this effect creates the real xterm instance + ws
  // connection — it must not tear that down and reconnect just because the
  // callback identity changed.
  const onTitleChangeRef = useRef(onTitleChange);
  useEffect(() => {
    onTitleChangeRef.current = onTitleChange;
  }, [onTitleChange]);

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 13,
      fontFamily:
        "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
      // Required for @xterm/addon-clipboard below: it registers its OSC
      // 52 handler via registerOscHandler, which xterm.js gates behind
      // this flag as a "proposed" (not-yet-fully-stabilized) API.
      allowProposedApi: true,
      // xterm.js already parses OSC 8 hyperlinks (the escape sequence
      // `claude` itself uses for its clickable-looking links — confirmed
      // via `strings` on the binary) with no addon needed for that part.
      // But its DEFAULT link-open behavior, with no linkHandler supplied,
      // is a native browser confirm() dialog ("Do you want to navigate to
      // ...? WARNING: This link could potentially be dangerous") before
      // it ever calls window.open — found by reading xterm.js's own
      // source. Providing our own skips that dialog. See
      // docs/terminal-clipboard.md for the still-open question of whether
      // link clicks are *also* being swallowed by mouse-tracking mode the
      // same way text selection is (unconfirmed without a real browser).
      linkHandler: {
        activate: (_event, uri) => {
          window.open(uri, "_blank", "noopener,noreferrer");
        },
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    // OSC 52 clipboard passthrough: when a program running inside the
    // terminal (tmux copy-mode, if `set-clipboard on` — see
    // internal/term.CreateSession — or any inner app that emits OSC 52
    // itself) sets "the terminal clipboard," this addon catches that
    // escape sequence and writes it to the real browser clipboard. This
    // is the mechanism that makes copying work even when the pane's
    // program has grabbed mouse tracking (e.g. `claude`'s TUI) and
    // disabled xterm.js's own drag-to-select entirely — see
    // docs/terminal-clipboard.md for the full story. Uses the addon's
    // default BrowserClipboardProvider (navigator.clipboard), no custom
    // provider needed.
    term.loadAddon(new ClipboardAddon());
    term.open(containerRef.current);
    fit.fit();

    const ws = new WebSocket(terminalWsUrl(terminalId));
    ws.binaryType = "arraybuffer";

    // Copy-on-selection + explicit clipboard paste — see
    // classifyTerminalKeyEvent's doc comment for why xterm.js needs this
    // wired up by hand rather than "just working." Returning false tells
    // xterm not to also process the keystroke itself (i.e. don't ALSO
    // send Ctrl+C as a literal SIGINT byte when the user meant "copy") —
    // but that alone does NOT call the browser's own preventDefault(), so
    // for paste specifically we call it ourselves: xterm.js already has
    // its own listener for the browser's native `paste` ClipboardEvent
    // (which fires on Ctrl+V unless prevented), and without preventDefault
    // here that would fire *in addition to* our explicit clipboard read
    // below, pasting the clipboard twice.
    term.attachCustomKeyEventHandler((event) => {
      switch (classifyTerminalKeyEvent(event)) {
        case "copy": {
          const selection = term.getSelection();
          if (!selection) return true; // no selection: real Ctrl+C, send SIGINT as normal
          navigator.clipboard?.writeText(selection).catch(() => {
            // Clipboard permission denied/unavailable — nothing else to
            // do; the selection is still visible for a manual right-
            // click copy.
          });
          return false;
        }
        case "paste":
          event.preventDefault(); // see comment above — avoids a double-paste via xterm's own native `paste` listener
          navigator.clipboard?.readText().then((text) => term.paste(text)).catch(() => {
            // Clipboard permission denied/unavailable. Deliberately no
            // fallback to the native paste event here (we just prevented
            // it) — a failed programmatic read is rare (this app runs on
            // localhost, treated as a secure context, so the Clipboard
            // API is available) and not worth the complexity of
            // conditionally un-preventing default after the fact.
          });
          return false;
        case "newline":
          event.preventDefault(); // stop xterm sending its own plain \r for this Enter
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(new TextEncoder().encode("\x1b\r"));
          }
          return false;
        default:
          return true;
      }
    });

    const sendResize = () => {
      fit.fit();
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows })
        );
      }
    };

    ws.onopen = sendResize;
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(ev.data));
      } else if (typeof ev.data === "string") {
        term.write(ev.data);
      }
    };
    ws.onerror = () => {
      term.writeln("\r\n[connection error]");
    };
    ws.onclose = () => {
      term.writeln("\r\n[disconnected]");
    };

    const dataDisposable = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });

    const titleDisposable = term.onTitleChange((title) => {
      onTitleChangeRef.current?.(title);
    });

    const resizeObserver = new ResizeObserver(sendResize);
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
      dataDisposable.dispose();
      titleDisposable.dispose();
      ws.close();
      term.dispose();
    };
  }, [terminalId]);

  return (
    <div
      ref={containerRef}
      className="terminal-pane"
      style={{ width: "100%", height: "100%" }}
    />
  );
}
