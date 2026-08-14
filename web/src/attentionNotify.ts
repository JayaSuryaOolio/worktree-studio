// Side effects for "a claude session needs your input and you're not
// looking at that worktree right now" — see useAttentionStream.ts for the
// push channel that triggers these, and Sidebar.tsx for the "not looking
// at it" focus check. Two independent channels, either of which can work
// on its own depending on what the browser/OS allows at the time: a short
// synthesized tone (no asset to bundle, no permission needed, but subject
// to the browser's autoplay policy — silently does nothing until the page
// has received at least one user gesture) and a real desktop Notification
// (needs explicit permission — see requestNotificationPermission, called
// automatically on load per RepoContext.tsx's "on by default" policy, and
// also offered as a manual button in SettingsModal.tsx for a browser that
// needs a real user gesture to grant it. Independently gated on
// notificationPreference.ts's own on/off toggle, also in that same tab —
// permission granted but the toggle off means this stays silent).
import { getNotificationsEnabled } from "./notificationPreference";

let audioCtx: AudioContext | null = null;

/** Plays a short, two-note "ring" using the Web Audio API — deliberately
 * not an <audio> file, so there's nothing to bundle or fetch. Fails
 * silently (e.g. before any user gesture has unlocked audio on this page,
 * or in a test/jsdom environment with no real AudioContext) since a missed
 * sound is never worth surfacing as an error. */
export function playAttentionSound() {
  try {
    const Ctx = window.AudioContext ?? (window as any).webkitAudioContext;
    if (!Ctx) return;
    audioCtx ??= new Ctx();
    const ctx = audioCtx;
    const now = ctx.currentTime;

    [880, 1108.73].forEach((freq, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = "sine";
      osc.frequency.value = freq;
      const start = now + i * 0.12;
      gain.gain.setValueAtTime(0, start);
      gain.gain.linearRampToValueAtTime(0.2, start + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.001, start + 0.18);
      osc.connect(gain).connect(ctx.destination);
      osc.start(start);
      osc.stop(start + 0.2);
    });
  } catch {
    // Best-effort only — see doc comment above.
  }
}

/** Whether the user has already granted (or could still be asked for)
 * desktop notification permission — used by SettingsModal.tsx to decide
 * what its toggle button should say. `Notification` may not exist at all
 * (older browsers, some embedded webviews); treated the same as "denied". */
export function notificationPermission(): NotificationPermission | "unsupported" {
  if (typeof Notification === "undefined") return "unsupported";
  return Notification.permission;
}

/** Must be called from a user gesture (a click handler) on most browsers —
 * see SettingsModal.tsx's "Enable desktop notifications" button. */
export function requestNotificationPermission(): Promise<NotificationPermission | "unsupported"> {
  if (typeof Notification === "undefined") return Promise.resolve("unsupported");
  return Notification.requestPermission();
}

/** Fires a real OS-level desktop notification if permission was granted
 * AND the user hasn't turned the feature off in Settings; a silent no-op
 * otherwise (this is a "nice to have" channel — the sidebar's own dot
 * badge, plus playAttentionSound above, are the channels that always work
 * regardless of permission/preference). `onClick`, if given, is wired to
 * the notification itself — clicking it focuses this tab and runs it (see
 * RepoContext.tsx, which navigates to the worktree that triggered it) —
 * and closes the notification afterward.
 *
 * This is the plain web `Notification` API — a real OS notification in
 * today's browser-based app, but NOT confirmed to work the same way if
 * this project is ever packaged as a Wails app instead (WKWebView/
 * WebKitGTK's support for it is inconsistent compared to a real browser;
 * see PLAN.md's "revisit desktop notifications if/when this moves to
 * Wails" TODO). This function is the one seam to swap for a Wails native
 * notification binding if that turns out to be necessary — no caller
 * needs to change. */
export function maybeDesktopNotify(title: string, body: string, onClick?: () => void) {
  if (typeof Notification === "undefined" || Notification.permission !== "granted") return;
  if (!getNotificationsEnabled()) return;
  try {
    const notification = new Notification(title, { body });
    if (onClick) {
      notification.onclick = () => {
        window.focus();
        onClick();
        notification.close();
      };
    }
  } catch {
    // Best-effort only.
  }
}
