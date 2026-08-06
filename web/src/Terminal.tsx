import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { terminalWsUrl } from "./api";

interface Props {
  terminalId: string;
}

/**
 * A real terminal, not an exec-and-capture box: xterm.js renders whatever
 * the server's pty->tmux attach sends over the websocket, and every
 * keystroke goes straight back over the wire. See PLAN.md section 3 — the
 * backing tmux session is what makes this survive a worktree-studio server
 * restart, not anything client-side.
 */
export default function Terminal({ terminalId }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 13,
      fontFamily:
        "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(containerRef.current);
    fit.fit();

    const ws = new WebSocket(terminalWsUrl(terminalId));
    ws.binaryType = "arraybuffer";

    const sendResize = () => {
      fit.fit();
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows })
        );
      }
    };

    ws.onopen = sendResize;
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(ev.data));
      } else if (typeof ev.data === "string") {
        term.write(ev.data);
      }
    };
    ws.onerror = () => {
      term.writeln("\r\n[connection error]");
    };
    ws.onclose = () => {
      term.writeln("\r\n[disconnected]");
    };

    const dataDisposable = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });

    const resizeObserver = new ResizeObserver(sendResize);
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
      dataDisposable.dispose();
      ws.close();
      term.dispose();
    };
  }, [terminalId]);

  return (
    <div
      ref={containerRef}
      className="terminal-pane"
      style={{ width: "100%", height: "100%" }}
    />
  );
}
