import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { shouldAllowNativeContextMenu } from "./contextMenuPolicy";
import "./style.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>
);

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
