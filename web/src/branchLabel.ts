// Splitting a branch name so CSS can truncate it in the middle rather
// than at the end.
//
// Why this exists: `text-overflow: ellipsis` always cuts the tail, but
// branch names are prefix-clustered by convention — `hotfix-backend-…`,
// `feature/fetch-users-…`, `migrate/sync-pos-user…` — so the tail is
// exactly the part that tells two rows apart. A sidebar full of rows
// truncated to their shared prefix is a sidebar you have to hover to read.
//
// CSS can't middle-truncate on its own, but it can do this: render the
// head in a shrinking, ellipsised box and the tail in a fixed one that
// never shrinks. The browser still does the measuring, so this stays
// responsive to the actual sidebar width — no character budget to guess
// at, and nothing to re-tune if the sidebar becomes resizable later.

/** Separator characters branch names are conventionally segmented on. */
const SEPARATORS = new Set(["-", "/", "_", "."]);

/**
 * Splits `branch` into a head that may be ellipsised and a tail that must
 * stay visible.
 *
 * The tail is cut at a real segment boundary where one exists in a
 * reasonable window (so `hotfix-backend-services` keeps `-services`
 * rather than a mid-word `vices`), and falls back to a fixed slice
 * otherwise. Short names get an empty tail — nothing will be truncated at
 * that length anyway, and a needless split would let a sub-pixel rounding
 * gap show up between the two spans.
 */
export function splitBranchLabel(branch: string): { head: string; tail: string } {
  // Below this, the name fits in a 250px sidebar at --fs-2 without the
  // browser ever reaching for an ellipsis.
  if (branch.length <= 18) return { head: branch, tail: "" };

  // Look for a separator in the last third-ish of the name: far enough in
  // to leave a meaningful head, far enough from the end to leave a
  // meaningful tail.
  const earliest = Math.max(1, branch.length - 16);
  const latest = branch.length - 3;
  for (let i = latest; i >= earliest; i--) {
    if (SEPARATORS.has(branch[i])) {
      return { head: branch.slice(0, i), tail: branch.slice(i) };
    }
  }

  // No usable boundary (one long unbroken token) — keep a fixed tail so
  // at least the distinguishing end survives.
  return { head: branch.slice(0, -8), tail: branch.slice(-8) };
}
