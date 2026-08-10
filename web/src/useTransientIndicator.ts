import { useEffect, useRef, useState } from "react";

export type TransientIndicatorPhase = "hidden" | "visible" | "fading";

export interface TransientIndicatorOptions {
  showDelayMs?: number;
  visibleMs?: number;
  fadeMs?: number;
}

const DEFAULTS: Required<TransientIndicatorOptions> = {
  showDelayMs: 900,
  visibleMs: 1000,
  fadeMs: 300,
};

/**
 * Turns a raw "is this happening right now" boolean into a debounced,
 * minimum-duration, fade-out presence. Built for the sidebar's stale-status
 * refresh cue: a real refresh often completes in well under a second, and
 * a naive on/off indicator tied directly to that boolean reads as an
 * irritating flash rather than a useful cue. Instead:
 *
 * - `active` flipping true for less than showDelayMs shows nothing at all.
 * - `active` staying true past showDelayMs makes the indicator visible,
 *   and it stays visible for at least visibleMs regardless of how quickly
 *   `active` itself goes back to false in the meantime — a fixed, calm
 *   lifecycle rather than one that chases the real fetch's timing.
 * - After visibleMs it fades out over fadeMs, then hides.
 */
export function useTransientIndicator(
  active: boolean,
  options?: TransientIndicatorOptions
): TransientIndicatorPhase {
  const opts = { ...DEFAULTS, ...options };
  const [phase, setPhase] = useState<TransientIndicatorPhase>("hidden");
  const phaseRef = useRef(phase);
  phaseRef.current = phase;

  useEffect(() => {
    if (!active) return;
    if (phaseRef.current === "hidden") {
      const id = setTimeout(() => setPhase("visible"), opts.showDelayMs);
      return () => clearTimeout(id);
    }
    if (phaseRef.current === "fading") {
      // A fresh refresh started before the last one finished fading out —
      // treat it as still-relevant rather than letting it hide then
      // immediately re-debounce.
      setPhase("visible");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active]);

  useEffect(() => {
    if (phase === "visible") {
      const id = setTimeout(() => setPhase("fading"), opts.visibleMs);
      return () => clearTimeout(id);
    }
    if (phase === "fading") {
      const id = setTimeout(() => setPhase("hidden"), opts.fadeMs);
      return () => clearTimeout(id);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase]);

  return phase;
}
