import { describe, expect, it } from "vitest";
import { detectTerminalApp } from "./terminalAppDetection";

describe("detectTerminalApp", () => {
  it("detects claude from its real observed OSC 0 title", () => {
    // The exact bytes `claude` was confirmed (via a real pty capture) to
    // emit: "\x1b]0;⏳ Claude Code\x07" — xterm.js hands onTitleChange just
    // the text payload, without the OSC wrapper or terminator.
    expect(detectTerminalApp("⏳ Claude Code")).toEqual({ kind: "claude", label: "Claude" });
  });

  it("matches regardless of the leading status glyph", () => {
    expect(detectTerminalApp("✳ Claude Code")).toEqual({ kind: "claude", label: "Claude" });
    expect(detectTerminalApp("Claude Code")).toEqual({ kind: "claude", label: "Claude" });
  });

  it("returns null for a plain shell prompt title", () => {
    expect(detectTerminalApp("user@host: ~/worktree")).toBeNull();
  });

  it("returns null for an empty title", () => {
    expect(detectTerminalApp("")).toBeNull();
  });

  it("does not false-positive on an unrelated title merely containing 'Claude'", () => {
    expect(detectTerminalApp("Claude's notes.txt — vim")).toBeNull();
  });
});
