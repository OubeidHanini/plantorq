import { useEffect, useRef, useState } from "react";
import type { LiveEvent } from "@/shared/api/types";

export type ConnectionState = "connecting" | "open" | "closed";

/**
 * Subscribes to the API WebSocket and calls `onEvent` for every message.
 * Reconnects with exponential backoff (1s → 30s), so a restart of the API
 * or a network blip heals on its own.
 */
export function useLiveEvents(token: string | null, onEvent: (e: LiveEvent) => void): ConnectionState {
  const [state, setState] = useState<ConnectionState>("connecting");
  const handler = useRef(onEvent);
  handler.current = onEvent;

  useEffect(() => {
    if (!token) return;
    let socket: WebSocket | null = null;
    let retry = 0;
    let timer: number | undefined;
    let disposed = false;

    const connect = () => {
      const scheme = location.protocol === "https:" ? "wss" : "ws";
      socket = new WebSocket(`${scheme}://${location.host}/api/v1/ws?access_token=${encodeURIComponent(token)}`);
      setState("connecting");
      socket.onopen = () => {
        retry = 0;
        setState("open");
      };
      socket.onmessage = (msg) => {
        try {
          handler.current(JSON.parse(msg.data as string) as LiveEvent);
        } catch {
          /* ignore malformed frames */
        }
      };
      socket.onclose = () => {
        setState("closed");
        if (disposed) return;
        const delay = Math.min(30_000, 1000 * 2 ** retry++);
        timer = window.setTimeout(connect, delay);
      };
    };

    connect();
    return () => {
      disposed = true;
      window.clearTimeout(timer);
      socket?.close();
    };
  }, [token]);

  return state;
}
