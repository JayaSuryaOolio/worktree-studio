// Whether desktop notifications for "a claude session needs your input" are
// wanted at all — a single global preference (localStorage), independent of
// (but layered under) the browser's own Notification.permission. Defaults
// to enabled: the sidebar's attention dot/sound already fire regardless, so
// this being on by default just means a person who never opens the
// settings modal still gets the extra desktop-level nudge, and can turn it
// off from Settings -> Appearance if they don't want it — same "on unless
// you say otherwise" shape as the notification permission auto-request in
// RepoContext.tsx.
const STORAGE_KEY = "worktree-studio-desktop-notifications";

export function getNotificationsEnabled(): boolean {
  return localStorage.getItem(STORAGE_KEY) !== "false"; // default: enabled
}

export function setNotificationsEnabled(enabled: boolean): void {
  localStorage.setItem(STORAGE_KEY, String(enabled));
}
