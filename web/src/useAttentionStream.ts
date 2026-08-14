import { useEffect, useRef, useState } from "react";
import { attentionWsUrl } from "./api";

// How long to wait before reconnecting after the socket closes (server
// restart, network blip, laptop sleep/wake) — this connection is meant to
// stay open for the entire time the app is open, unlike the per-worktree
// terminal/file-watch sockets that close naturally when their own
// component unmounts.
const RECONNECT_DELAY_MS = 3000;

interface SnapshotMessage {
  type: "snapshot";
  pending: Record<string, string>;
}

interface UpdateMessage {
  type: "update";
  worktree_id: string;
  pending: boolean;
  message?: string;
}

type AttentionMessage = SnapshotMessage | UpdateMessage;

/** Subscribes to /ws/attention (see internal/api/attention.go) for the
 * lifetime of the calling component — meant to be mounted once, high up
 * (RepoProvider), not per-worktree. Returns the current worktreeId ->
 * message map of every worktree with a claude session waiting on the
 * user, and calls onUpdate for every live (post-snapshot) change so the
 * caller can decide whether to play a sound / fire a desktop notification
 * for it (that decision needs "was this worktree already known-pending
 * before this render", which the caller — not this hook — is in the best
 * position to judge against its own focus state). */
export function useAttentionStream(
  onUpdate?: (worktreeId: string, pending: boolean, message: string) => void
): Record<string, string> {
  const [pending, setPending] = useState<Record<string, string>>({});
  const onUpdateRef = useRef(onUpdate);
  onUpdateRef.current = onUpdate;

  useEffect(() => {
    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let disposed = false;

    function connect() {
      if (disposed) return;
      ws = new WebSocket(attentionWsUrl());

      ws.onmessage = (event) => {
        let msg: AttentionMessage;
        try {
          msg = JSON.parse(event.data);
        } catch {
          return;
        }
        if (msg.type === "snapshot") {
          setPending(msg.pending ?? {});
          return;
        }
        if (msg.type === "update") {
          setPending((prev) => {
            if (!msg.pending) {
              if (!(msg.worktree_id in prev)) return prev;
              const next = { ...prev };
              delete next[msg.worktree_id];
              return next;
            }
            return { ...prev, [msg.worktree_id]: msg.message ?? "" };
          });
          onUpdateRef.current?.(msg.worktree_id, msg.pending, msg.message ?? "");
        }
      };

      ws.onclose = () => {
        if (disposed) return;
        reconnectTimer = setTimeout(connect, RECONNECT_DELAY_MS);
      };
    }

    connect();

    return () => {
      disposed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, []);

  return pending;
}
