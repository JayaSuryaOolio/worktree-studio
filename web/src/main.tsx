import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { shouldAllowNativeContextMenu } from "./contextMenuPolicy";
import { applyTheme, getStoredTheme, watchSystemMode } from "./theme";
import "./style.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>
);

// Keeps the "System" mode choice actually live. index.html's inline
// snippet already resolved and applied the theme before first paint —
// this only matters afterwards, when the OS flips light/dark (or a
// scheduled Night Shift-style switch fires) while the app is open. A
// no-op unless the stored mode is "system": applyTheme re-resolves from
// matchMedia each time, so an explicit dark/light choice re-applies to
// the same value it already had.
watchSystemMode(() => applyTheme(getStoredTheme()));

// Registering a service worker (even a no-op one) is what makes Chrome/Edge
// treat this as an installable PWA — see public/sw.js.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js");
  });
}

// This is meant to feel like a native app, not a page — the browser's own
// right-click context menu (with its "Reload"/"Inspect"/"View Page Source"
// entries) doesn't belong here on the app's own chrome (sidebar, buttons,
// dialogs, ...). App-level, not React-owned, since it's a permanent
// document-wide behavior override rather than UI state. See
// contextMenuPolicy.ts for which surfaces are exempted and why.
document.addEventListener("contextmenu", (e) => {
  if (shouldAllowNativeContextMenu(e.target)) return;
  e.preventDefault();
});
