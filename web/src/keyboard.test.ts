import { describe, expect, it } from "vitest";
import { hasSafeModifier, isTextEntryTarget } from "./keyboard";

function el(html: string): HTMLElement {
  const host = document.createElement("div");
  host.innerHTML = html;
  return host.firstElementChild as HTMLElement;
}

describe("isTextEntryTarget", () => {
  it("is true for the elements that consume keystrokes", () => {
    expect(isTextEntryTarget(el("<input />"))).toBe(true);
    expect(isTextEntryTarget(el("<textarea></textarea>"))).toBe(true);
    expect(isTextEntryTarget(el("<select></select>"))).toBe(true);
  });

  it("is true for a contenteditable", () => {
    const div = el('<div contenteditable="true"></div>');
    // jsdom doesn't implement isContentEditable from the attribute alone.
    Object.defineProperty(div, "isContentEditable", { value: true });
    expect(isTextEntryTarget(div)).toBe(true);
  });

  it("covers xterm, which receives keys through a hidden textarea", () => {
    expect(isTextEntryTarget(el('<textarea class="xterm-helper-textarea"></textarea>'))).toBe(true);
  });

  it("is false for ordinary elements and for nothing at all", () => {
    expect(isTextEntryTarget(el("<div></div>"))).toBe(false);
    expect(isTextEntryTarget(el("<button></button>"))).toBe(false);
    expect(isTextEntryTarget(null)).toBe(false);
  });
});

describe("hasSafeModifier", () => {
  const ev = (init: Partial<KeyboardEvent> & { target?: EventTarget }) =>
    ({ metaKey: false, ctrlKey: false, target: null, ...init }) as unknown as KeyboardEvent;

  it("accepts Cmd anywhere — it never reaches tmux", () => {
    expect(hasSafeModifier(ev({ metaKey: true }))).toBe(true);
    expect(hasSafeModifier(ev({ metaKey: true, target: el("<textarea></textarea>") }))).toBe(true);
  });

  it("accepts Ctrl outside a terminal", () => {
    expect(hasSafeModifier(ev({ ctrlKey: true, target: el("<div></div>") }))).toBe(true);
  });

  it("refuses Ctrl inside a terminal, which would steal tmux's prefix", () => {
    const xterm = el('<textarea class="xterm-helper-textarea"></textarea>');
    expect(hasSafeModifier(ev({ ctrlKey: true, target: xterm }))).toBe(false);
  });

  it("is false with no modifier at all", () => {
    expect(hasSafeModifier(ev({}))).toBe(false);
  });
});
