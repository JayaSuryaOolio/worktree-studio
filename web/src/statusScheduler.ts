// A small pub-sub + queue for polling per-id status (git dirty/ahead-behind,
// spotlight indexing state, ...) without the blind "refetch everything on
// one global timer" approach RepoContext used to do directly. Two things
// motivated pulling this into its own module rather than inlining it:
//
// 1. It's reused for two independent status kinds (git + spotlight) in
//    RepoContext, so it earns its keep as a generic, id-keyed scheduler
//    rather than two copies of near-identical polling logic.
// 2. The scheduling policy (touch/idle/refresh-now) is fiddly enough to
//    want its own unit tests independent of React — see
//    statusScheduler.test.ts, which drives it with a fake clock and no
//    DOM at all.
//
// Policy, in one paragraph: an id only gets *background* refreshes while
// it's been "touched" within idleTimeoutMs — subscribing touches it, and
// callers can touch explicitly (e.g. RepoContext touches the worktree
// whose detail page is currently focused). An id that hasn't been touched
// recently just keeps returning its last-known snapshot via peek()/an
// existing subscription — it isn't deleted or actively invalidated, it
// simply stops costing a backend request every tick until something asks
// for it again. refreshNow() is the on-demand escape hatch: fetch
// immediately and push the next scheduled tick out by intervalMs, so an
// explicit refresh never gets immediately followed by a redundant
// background one.

export interface StatusSnapshot<T> {
  data: T | null;
  fetchedAt: number | null;
  refreshing: boolean;
}

interface Entry<T> {
  data: T | null;
  fetchedAt: number | null;
  lastTouchedAt: number;
  nextDueAt: number;
  inFlight: boolean;
  listeners: Set<(snapshot: StatusSnapshot<T>) => void>;
}

export interface StatusSchedulerOptions {
  // How often an actively-touched id gets a background refresh.
  intervalMs: number;
  // How long since the last touch() before an id is excluded from
  // background refreshes (it keeps its last-known data either way).
  idleTimeoutMs: number;
  // Heartbeat resolution for checking which ids are due. Defaults to 1s;
  // exposed mainly so tests can drive it with a small, deterministic value.
  tickMs?: number;
}

const emptySnapshot = <T>(): StatusSnapshot<T> => ({ data: null, fetchedAt: null, refreshing: false });

export class StatusScheduler<T> {
  private entries = new Map<string, Entry<T>>();
  private timer: ReturnType<typeof setInterval> | null = null;
  private disposed = false;

  constructor(
    private readonly fetcher: (id: string) => Promise<T>,
    private readonly opts: StatusSchedulerOptions
  ) {
    this.setPaused(false);
  }

  private getOrCreate(id: string): Entry<T> {
    let entry = this.entries.get(id);
    if (!entry) {
      entry = {
        data: null,
        fetchedAt: null,
        lastTouchedAt: 0,
        nextDueAt: 0,
        inFlight: false,
        listeners: new Set(),
      };
      this.entries.set(id, entry);
    }
    return entry;
  }

  private snapshotOf(entry: Entry<T>): StatusSnapshot<T> {
    return { data: entry.data, fetchedAt: entry.fetchedAt, refreshing: entry.inFlight };
  }

  private notify(id: string) {
    const entry = this.entries.get(id);
    if (!entry) return;
    const snapshot = this.snapshotOf(entry);
    entry.listeners.forEach((listener) => listener(snapshot));
  }

  /** Marks id as recently requested, keeping it eligible for background refresh for idleTimeoutMs. */
  touch(id: string): void {
    this.getOrCreate(id).lastTouchedAt = Date.now();
  }

  /**
   * Registers interest in id's status. Calls listener immediately with the
   * current (possibly empty) snapshot, then again every time fresh data
   * arrives or a refresh starts/finishes. Touches id and, if it has never
   * been fetched, kicks off an immediate fetch so a brand-new id isn't
   * stuck showing an empty snapshot until the next heartbeat.
   */
  subscribe(id: string, listener: (snapshot: StatusSnapshot<T>) => void): () => void {
    const entry = this.getOrCreate(id);
    entry.listeners.add(listener);
    this.touch(id);
    listener(this.snapshotOf(entry));
    if (entry.fetchedAt === null && !entry.inFlight) {
      void this.fetch(id);
    }
    return () => {
      entry.listeners.delete(listener);
    };
  }

  /** Current cached snapshot, if id has ever been touched/subscribed. No side effects. */
  peek(id: string): StatusSnapshot<T> | undefined {
    const entry = this.entries.get(id);
    return entry ? this.snapshotOf(entry) : undefined;
  }

  /**
   * Fetches id right now (bypassing the due check), and pushes its next
   * scheduled background refresh out by intervalMs so it doesn't
   * immediately re-fire on the next heartbeat.
   */
  async refreshNow(id: string): Promise<void> {
    this.touch(id);
    await this.fetch(id);
  }

  private async fetch(id: string): Promise<void> {
    const entry = this.getOrCreate(id);
    if (entry.inFlight) return;
    entry.inFlight = true;
    this.notify(id);
    try {
      entry.data = await this.fetcher(id);
      entry.fetchedAt = Date.now();
    } catch {
      // Leave the last-known data in place; the next due tick tries again.
    } finally {
      entry.inFlight = false;
      entry.nextDueAt = Date.now() + this.opts.intervalMs;
      this.notify(id);
    }
  }

  private tick(): void {
    const now = Date.now();
    for (const [id, entry] of this.entries) {
      if (entry.inFlight) continue;
      if (now - entry.lastTouchedAt > this.opts.idleTimeoutMs) continue;
      if (now < entry.nextDueAt) continue;
      void this.fetch(id);
    }
  }

  /** Stops (or restarts) the background heartbeat — used to pause everything while the tab is hidden. */
  setPaused(paused: boolean): void {
    if (this.disposed) return;
    if (this.timer !== null) {
      clearInterval(this.timer);
      this.timer = null;
    }
    if (!paused) {
      this.timer = setInterval(() => this.tick(), this.opts.tickMs ?? 1000);
      this.tick(); // catch up immediately on resume rather than waiting a full tick
    }
  }

  /** Stops the heartbeat and drops all cached state. For test/component teardown. */
  dispose(): void {
    this.disposed = true;
    if (this.timer !== null) clearInterval(this.timer);
    this.timer = null;
    this.entries.clear();
  }
}

export { emptySnapshot };
