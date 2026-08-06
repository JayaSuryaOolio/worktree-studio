import "@testing-library/jest-dom/vitest";

// jsdom doesn't implement ResizeObserver — cmdk (used by CommandPalette)
// uses it internally to measure item heights for scroll-into-view
// behavior, which isn't relevant in a jsdom test anyway. A no-op stub is
// enough to let the component mount and function normally.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// Same story for scrollIntoView (also used by cmdk to keep the selected
// item visible) — jsdom doesn't implement actual scrolling.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = function () {};
}
