// Seams for the Wails desktop shell (cmd/worktree-studio built with
// `-tags desktop`). In a plain browser none of this is present and every
// helper falls back to the web API, so the web app behaves exactly as before.
interface WailsRuntime {
  ClipboardGetText(): Promise<string>;
  ClipboardSetText(text: string): Promise<boolean>;
  BrowserOpenURL(url: string): void;
}

type DesktopWindow = Window & { runtime?: WailsRuntime; __WTS_WS_BASE__?: string };
const w = () => window as DesktopWindow;

// Wails' webview has no browser clipboard permission model, so go through Go.
export const clipboard = {
  readText: (): Promise<string> =>
    w().runtime ? w().runtime!.ClipboardGetText() : navigator.clipboard.readText(),
  writeText: (text: string): Promise<void> =>
    w().runtime
      ? w().runtime!.ClipboardSetText(text).then(() => undefined)
      : navigator.clipboard.writeText(text),
};

// window.open is a no-op in WKWebView; hand the URL to the OS browser instead.
export function openExternal(uri: string): void {
  const rt = w().runtime;
  if (rt) rt.BrowserOpenURL(uri);
  else window.open(uri, "_blank", "noopener,noreferrer");
}

// Wails can't proxy websockets, so the desktop shell injects the real
// loopback server address (see cmd/worktree-studio/desktop.go).
export function wsOrigin(): string {
  const injected = w().__WTS_WS_BASE__;
  if (injected) return injected;
  return `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}`;
}
