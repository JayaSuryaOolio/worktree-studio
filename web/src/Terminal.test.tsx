import { describe, expect, it } from "vitest";
import { classifyTerminalKeyEvent } from "./Terminal";

// Terminal.tsx itself (xterm.js + websocket wiring) isn't unit-tested here
// — same established practice as elsewhere in this project (WorktreeDetail
// mocks it away entirely) since jsdom doesn't meaningfully exercise
// xterm's canvas rendering or real clipboard permissions. This tests the
// one piece of Terminal.tsx that's pure logic, independent of any of
// that: which keydown combos should be treated as copy/paste vs. passed
// through to xterm's normal key handling.
describe("classifyTerminalKeyEvent", () => {
  it("classifies Ctrl+C as copy", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: true, metaKey: false, shiftKey: false, key: "c" })
    ).toBe("copy");
  });

  it("classifies Cmd+C as copy too (macOS)", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: false, metaKey: true, shiftKey: false, key: "c" })
    ).toBe("copy");
  });

  it("classifies Ctrl+V and Cmd+V as paste", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: true, metaKey: false, shiftKey: false, key: "v" })
    ).toBe("paste");
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: false, metaKey: true, shiftKey: false, key: "v" })
    ).toBe("paste");
  });

  it("is case-insensitive on the key (Shift can flip case without meaning Ctrl+Shift+C)", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: true, metaKey: false, shiftKey: false, key: "C" })
    ).toBe("copy");
  });

  it("passes through Ctrl+Shift+C untouched (a different, common copy-in-terminal shortcut some users rely on)", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: true, metaKey: false, shiftKey: true, key: "c" })
    ).toBe("pass");
  });

  it("passes through plain 'c'/'v' with no modifier", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: false, metaKey: false, shiftKey: false, key: "c" })
    ).toBe("pass");
  });

  it("passes through unrelated Ctrl-combos (e.g. Ctrl+D)", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: true, metaKey: false, shiftKey: false, key: "d" })
    ).toBe("pass");
  });

  it("ignores keyup entirely, even for a copy/paste-shaped combo", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keyup", ctrlKey: true, metaKey: false, shiftKey: false, key: "c" })
    ).toBe("pass");
  });

  it("classifies Shift+Enter as newline", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: false, metaKey: false, shiftKey: true, key: "Enter" })
    ).toBe("newline");
  });

  it("passes through plain Enter (no shift) untouched", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keydown", ctrlKey: false, metaKey: false, shiftKey: false, key: "Enter" })
    ).toBe("pass");
  });

  it("ignores Shift+Enter on keyup", () => {
    expect(
      classifyTerminalKeyEvent({ type: "keyup", ctrlKey: false, metaKey: false, shiftKey: true, key: "Enter" })
    ).toBe("pass");
  });
});
