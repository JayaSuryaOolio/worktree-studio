import { describe, expect, it } from "vitest";
import { shouldAllowNativeContextMenu } from "./contextMenuPolicy";

function elementIn(html: string, selector: string): Element {
  const root = document.createElement("div");
  root.innerHTML = html;
  document.body.appendChild(root);
  const el = root.querySelector(selector);
  if (!el) throw new Error(`no element matching ${selector} in ${html}`);
  return el;
}

describe("shouldAllowNativeContextMenu", () => {
  it("disallows plain app chrome (a button, a div)", () => {
    expect(shouldAllowNativeContextMenu(elementIn("<button>x</button>", "button"))).toBe(false);
    expect(shouldAllowNativeContextMenu(elementIn("<div class='sidebar-worktree'>x</div>", "div"))).toBe(false);
  });

  it("allows an input/textarea", () => {
    expect(shouldAllowNativeContextMenu(elementIn("<input />", "input"))).toBe(true);
    expect(shouldAllowNativeContextMenu(elementIn("<textarea></textarea>", "textarea"))).toBe(true);
  });

  it("allows a contenteditable element", () => {
    expect(shouldAllowNativeContextMenu(elementIn("<div contenteditable='true'>x</div>", "div"))).toBe(true);
  });

  it("allows inside a terminal (xterm.js's .xterm root) even on a nested child", () => {
    const nested = elementIn("<div class='xterm'><div class='xterm-screen'><span>x</span></div></div>", "span");
    expect(shouldAllowNativeContextMenu(nested)).toBe(true);
  });

  it("allows inside a CodeMirror editor (.cm-editor)", () => {
    const nested = elementIn("<div class='cm-editor'><div class='cm-content'>x</div></div>", "div.cm-content");
    expect(shouldAllowNativeContextMenu(nested)).toBe(true);
  });

  it("handles a null target (not an Element) as disallowed", () => {
    expect(shouldAllowNativeContextMenu(null)).toBe(false);
  });
});
