import { useEffect, useRef } from "react";
import { openFileWsUrl } from "./api";

// Same reconnect posture as useAttentionStream.ts: this connection is meant
// to stay open for the entire time the app is open.
const RECONNECT_DELAY_MS = 3000;

interface OpenFileMessage {
  type: "open-file";
  worktree_id: string;
  path: string;
}

/** Subscribes to /ws/open-file (see internal/api/openfile.go) for the
 * lifetime of the calling component — meant to be mounted once, high up
 * (RepoProvider), not per-worktree, mirroring useAttentionStream.ts.
 * Unlike attention there's no persistent state to return: "open this file"
 * is a one-shot instruction, not something to snapshot on connect, so this
 * hook has no return value — just an onOpenFile callback for every event. */
export function useOpenFileStream(
  onOpenFile: (worktreeId: string, path: string) => void
): void {
  const onOpenFileRef = useRef(onOpenFile);
  onOpenFileRef.current = onOpenFile;

  useEffect(() => {
    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let disposed = false;

    function connect() {
      if (disposed) return;
      ws = new WebSocket(openFileWsUrl());

      ws.onmessage = (event) => {
        let msg: OpenFileMessage;
        try {
          msg = JSON.parse(event.data);
        } catch {
          return;
        }
        if (msg.type === "open-file") {
          onOpenFileRef.current(msg.worktree_id, msg.path);
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
}
