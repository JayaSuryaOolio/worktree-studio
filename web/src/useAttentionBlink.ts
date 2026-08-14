import { useEffect, useRef, useState } from "react";

// How long the sidebar's attention dot pulses before settling into a
// steady, still-visible state — direct feedback that blinking forever
// while a claude session sits waiting read as more alarming than useful;
// the dot itself must never disappear on its own regardless (only an
// explicit clear — opening that worktree's detail page, see
// RepoContext.tsx — removes it).
const BLINK_DURATION_MS = 10_000;

/** Given the current set of pending worktree ids (Sidebar.tsx's
 * `Object.keys(attentionPending)`), returns the subset still within their
 * initial 10s blink window since *first* becoming pending. A worktree that
 * goes pending -> cleared -> pending again gets a fresh 10s window each
 * time (its earlier timer/start-time is forgotten the moment it's no
 * longer in the pending set). */
export function useAttentionBlink(pendingWorktreeIds: string[]): Set<string> {
  const [blinking, setBlinking] = useState<Set<string>>(new Set());
  const startedRef = useRef<Set<string>>(new Set());
  const timersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const pendingKey = pendingWorktreeIds.join(",");

  useEffect(() => {
    const pendingSet = new Set(pendingKey ? pendingKey.split(",") : []);

    for (const id of pendingSet) {
      if (startedRef.current.has(id)) continue;
      startedRef.current.add(id);
      setBlinking((prev) => new Set(prev).add(id));
      const timer = setTimeout(() => {
        timersRef.current.delete(id);
        setBlinking((prev) => {
          if (!prev.has(id)) return prev;
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
      }, BLINK_DURATION_MS);
      timersRef.current.set(id, timer);
    }

    for (const id of Array.from(startedRef.current)) {
      if (pendingSet.has(id)) continue;
      startedRef.current.delete(id);
      const timer = timersRef.current.get(id);
      if (timer) {
        clearTimeout(timer);
        timersRef.current.delete(id);
      }
      setBlinking((prev) => {
        if (!prev.has(id)) return prev;
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  }, [pendingKey]);

  // Only on real unmount — clears every still-pending timer so none of
  // them fire a setState after this component tree is gone.
  useEffect(() => {
    const timers = timersRef.current;
    return () => timers.forEach((t) => clearTimeout(t));
  }, []);

  return blinking;
}
