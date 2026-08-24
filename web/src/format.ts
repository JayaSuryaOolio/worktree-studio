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
 * The strict form: use it where a single differing row should keep the
 * value per-row. For the common table case, prefer `dominantValue` —
 * see the note there.
 */
export function commonValue<T>(items: T[], pick: (item: T) => string | undefined | null): string | null {
  if (items.length === 0) return null;
  const first = pick(items[0]);
  if (!first) return null;
  return items.every((item) => pick(item) === first) ? first : null;
}

export interface Dominant {
  /** The value most rows share. */
  value: string;
  /** How many rows don't. */
  exceptions: number;
}

/**
 * The value *most* items share, plus how many don't.
 *
 * `commonValue` is all-or-nothing, and in practice that isn't enough: a
 * repo with eleven worktrees, nine of them created from `origin/master`
 * and two attached from elsewhere, kept printing "origin/master" nine
 * times because two rows disagreed. One outlier shouldn't defeat the
 * whole point — hoist what nine rows share into the caption and let the
 * two that differ be the only ones that say anything, which is exactly
 * the behaviour that makes them findable.
 *
 * Returns null when hoisting wouldn't help: fewer than three items (a
 * caption costs a line, and saving two repetitions isn't worth one), or
 * no value covering at least 60% of them (at that point the column is
 * genuinely carrying per-row information).
 */
export function dominantValue<T>(
  items: T[],
  pick: (item: T) => string | undefined | null
): Dominant | null {
  if (items.length < 3) return null;

  const counts = new Map<string, number>();
  for (const item of items) {
    const value = pick(item);
    if (!value) continue;
    counts.set(value, (counts.get(value) ?? 0) + 1);
  }

  let best = "";
  let bestCount = 0;
  for (const [value, count] of counts) {
    // Ties resolve to whichever value sorts first, so the caption doesn't
    // flip between renders on an unstable Map ordering.
    if (count > bestCount || (count === bestCount && value < best)) {
      best = value;
      bestCount = count;
    }
  }

  if (bestCount < Math.ceil(items.length * 0.6)) return null;
  return { value: best, exceptions: items.length - bestCount };
}

/** Everything before the last "/" — "" for a path with no directory part. */
export function parentDir(path: string): string {
  const i = path.lastIndexOf("/");
  return i <= 0 ? "" : path.slice(0, i);
}

/** Everything after the last "/". */
export function leafName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}
