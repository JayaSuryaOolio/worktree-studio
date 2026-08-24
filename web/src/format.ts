// Formatting helpers for "show the difference, not the data" (see
// docs/design-system.md). Both of these exist because the worktrees table
// was printing the same string on every row and truncating the part that
// differed.

/**
 * A coarse relative time: "4 days ago", "just now".
 *
 * The table used to render `new Date(x).toLocaleString()` —
 * "8/21/2026, 5:16:26 PM" — for a column nobody reads to the second.
 * Precision that fine is noise in a scan, and it's the widest column in
 * the row. Pair every call with the exact timestamp in a `title`, so the
 * precise value is one hover away.
 *
 * Deliberately stops at weeks: past that the exact day stops mattering
 * for a worktree, and "3 months ago" and "14 weeks ago" carry the same
 * decision ("this is old").
 */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  const ms = then.getTime();
  if (Number.isNaN(ms)) return "";

  const seconds = Math.round((now.getTime() - ms) / 1000);
  // Clock skew, or a timestamp written a moment ahead of us.
  if (seconds < 45) return "just now";

  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return plural(minutes, "minute");

  const hours = Math.round(minutes / 60);
  if (hours < 24) return plural(hours, "hour");

  const days = Math.round(hours / 24);
  if (days < 7) return plural(days, "day");

  const weeks = Math.round(days / 7);
  if (weeks < 9) return plural(weeks, "week");

  const months = Math.round(days / 30);
  return plural(months, "month");
}

function plural(n: number, unit: string): string {
  return `${n} ${unit}${n === 1 ? "" : "s"} ago`;
}

/**
 * Keeps the last `segments` path segments, marking the elision with an
 * ellipsis: `…/9af16fe11ec0cf6b/orders-new-id`.
 *
 * Every worktree under a repo shares the same ~62-character prefix
 * (`/Users/you/.worktree-studio/worktrees/<repoId>/`), so printing it in
 * full on every row made the widest column in the table ~84% identical
 * text — and it wrapped over three lines doing it. Always pair with the
 * full path in a `title` and a copy action; the value is still needed,
 * just not at full width on twenty rows at once.
 */
export function shortenPath(path: string, segments = 2): string {
  const parts = path.split("/").filter(Boolean);
  if (parts.length <= segments) return path;
  return `…/${parts.slice(-segments).join("/")}`;
}

/**
 * The value shared by every item, or null if they disagree (or there's
 * nothing to compare).
 *
 * Used to hoist a column out of the table entirely: when all twenty rows
 * say "origin/master", that belongs in one line of caption above the
 * table, not twenty times inside it. When they disagree it stays a
 * per-row value, because then it's actually telling rows apart.
 */
export function commonValue<T>(items: T[], pick: (item: T) => string | undefined | null): string | null {
  if (items.length === 0) return null;
  const first = pick(items[0]);
  if (!first) return null;
  return items.every((item) => pick(item) === first) ? first : null;
}
