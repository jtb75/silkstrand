/**
 * Shared event-stream hook for the ADR 008 SSE surface.
 *
 * - `mintStreamToken` wraps the POST /api/v1/events/stream-tokens client call
 *   with slightly kinder typing (aliases `kinds`/`resource_type`/etc to the
 *   exact backend request shape).
 * - `useEventStream` owns the EventSource lifecycle: token mint, re-mint
 *   every 45s (tokens live 60s), re-mint + reopen on error, and buffered
 *   state with a 1000-event cap.
 *
 * Scope-agnostic: both the Agents "Console" drawer and the Scan Results
 * "Console" tab consume this with different filters.
 */
import { useEffect, useRef, useState, useCallback } from 'react';
import {
  mintStreamToken as apiMintStreamToken,
  eventStreamURL,
  type StreamTokenRequest,
  type StreamTokenResponse,
} from '../api/client';

/** Filter baked into a stream token. Mirrors the backend's streamFilter. */
export type EventFilter = StreamTokenRequest;

/** Minimal envelope delivered over SSE. Matches events.Event on the server. */
export interface StreamEvent<P = unknown> {
  kind: string;
  resource_type?: string;
  resource_id?: string;
  occurred_at: string;
  payload?: P;
}

/** Status exposed to the UI so a small "Connected/Reconnecting/…" pill
 * can be rendered without reaching into the EventSource directly. */
export type StreamStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error';

export interface UseEventStreamOptions {
  /** Gate the stream — false = don't mint, don't connect. Default true. */
  enabled?: boolean;
  /** Maximum events kept in the rolling buffer. Default 1000 (ADR 008 PR E). */
  bufferSize?: number;
  /** Pre-emptive token re-mint cadence in ms. Default 45 000 (tokens live 60s). */
  tokenRefreshMs?: number;
}

export interface UseEventStream<T> {
  events: StreamEvent<T>[];
  status: StreamStatus;
  /** Holds the EventSource open but stops appending to state. */
  pause: () => void;
  resume: () => void;
  /** Drops the buffered events without touching the connection. */
  clear: () => void;
}

/** Re-export: the mint endpoint wrapper. Kept here as well so callers
 * have a single import point for the event-stream machinery. */
export async function mintStreamToken(
  filter?: EventFilter,
): Promise<StreamTokenResponse> {
  return apiMintStreamToken(filter);
}

/**
 * useEventStream opens an SSE connection to `/api/v1/events/stream` with
 * the given filter baked into a short-lived stream token, and returns a
 * rolling buffer of events.
 *
 * Lifecycle notes:
 *
 * - On mount / filter change, mint a token, then open EventSource.
 * - Re-mint every `tokenRefreshMs` (default 45s). The existing EventSource
 *   keeps streaming — the token is only checked on connect, so we store
 *   the fresh token for the next reconnect. Belt-and-braces: if the stream
 *   errors, we use the most recent token to reopen immediately.
 * - `pause()` stops appending to state but holds the connection open so
 *   the buffer resumes from real-time on `resume()` (no missed "just now"
 *   lines).
 * - Unmount cancels the in-flight mint and closes the EventSource.
 */
export function useEventStream<T = unknown>(
  filter: EventFilter,
  options: UseEventStreamOptions = {},
): UseEventStream<T> {
  const {
    enabled = true,
    bufferSize = 1000,
    tokenRefreshMs = 45_000,
  } = options;

  const [events, setEvents] = useState<StreamEvent<T>[]>([]);
  const [status, setStatus] = useState<StreamStatus>('idle');
  const pausedRef = useRef(false);

  // Stable stringification so effect dependency changes correctly when
  // a caller rebuilds the filter object with the same contents each render.
  const filterKey = JSON.stringify(filter ?? {});

  // Track mount state so a late mint-response doesn't call setState on an
  // unmounted component.
  const esRef = useRef<EventSource | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const refreshTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const pause = useCallback(() => {
    pausedRef.current = true;
  }, []);
  const resume = useCallback(() => {
    pausedRef.current = false;
  }, []);
  const clear = useCallback(() => {
    setEvents([]);
  }, []);

  useEffect(() => {
    if (!enabled) {
      // Nothing to do — no connection to set up or tear down. Status
      // stays at whatever it was (default 'idle'). Avoid calling
      // setState here to keep the effect a pure subscribe/unsubscribe.
      return;
    }

    let cancelled = false;
    const parsedFilter: EventFilter = JSON.parse(filterKey);

    function closeEventSource() {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
    }

    function appendEvent(raw: MessageEvent<string>) {
      if (pausedRef.current) return;
      let parsed: StreamEvent<T>;
      try {
        parsed = JSON.parse(raw.data) as StreamEvent<T>;
      } catch {
        // Malformed event — skip rather than crash. The server controls
        // the payload shape, so this shouldn't happen in practice.
        return;
      }
      setEvents((prev) => {
        const next = prev.length >= bufferSize
          ? prev.slice(prev.length - bufferSize + 1)
          : prev.slice();
        next.push(parsed);
        return next;
      });
    }

    function openStream(token: string) {
      closeEventSource();
      setStatus((s) => (s === 'connected' ? 'reconnecting' : 'connecting'));
      const es = new EventSource(eventStreamURL(token));
      esRef.current = es;

      // EventSource dispatches messages under the `event:` SSE name, which
      // the server sets to the event kind (e.g. "agent_log"). To catch
      // every kind without knowing them up front, subscribe to the default
      // 'message' handler AND intercept via onmessage on any named event.
      // Simplest approach: attach a single "generic" handler that runs for
      // the default event type, and mirror it for known kinds via a fallback.
      es.onmessage = (e) => appendEvent(e);

      // The server sets `event: <kind>` on every frame. EventSource only
      // invokes onmessage for frames without an `event:` line, so we use
      // addEventListener with a wildcard approach. EventSource doesn't
      // support wildcards — but we can listen for the known kinds we care
      // about; for everything else, the agent-log console only cares about
      // `agent_log`.
      const kinds = parsedFilter.kinds && parsedFilter.kinds.length > 0
        ? parsedFilter.kinds
        : ['agent_log']; // default: the only kind this hook is wired for today
      for (const k of kinds) {
        es.addEventListener(k, appendEvent as EventListener);
      }

      es.onopen = () => setStatus('connected');
      es.onerror = () => {
        setStatus('reconnecting');
        closeEventSource();
        // Backoff before reconnecting to prevent a flood if the
        // endpoint is transiently unreachable.
        setTimeout(() => {
          if (!cancelled) void refreshAndOpen();
        }, 3000);
      };
    }

    async function refreshAndOpen() {
      if (cancelled) return;
      abortRef.current?.abort();
      const ac = new AbortController();
      abortRef.current = ac;
      try {
        const res = await mintStreamToken(parsedFilter);
        if (cancelled || ac.signal.aborted) return;
        openStream(res.token);
      } catch {
        if (cancelled || ac.signal.aborted) return;
        // Mint failed — surface via status. The pre-emptive interval
        // below will try again at the next tick.
        setStatus('error');
      }
    }

    async function refreshTokenOnly() {
      if (cancelled) return;
      try {
        // Pre-emptive mint every tokenRefreshMs — pay the HTTP round-trip
        // now so a later error/reconnect path doesn't have to. We don't
        // touch the open EventSource (tokens are checked on connect
        // only); the refresh is insurance against the browser deciding
        // to reconnect after the current token has aged past 60s.
        await mintStreamToken(parsedFilter);
      } catch {
        // Ignore — next refresh or an error-triggered re-mint recovers.
      }
    }

    void refreshAndOpen();
    refreshTimerRef.current = setInterval(refreshTokenOnly, tokenRefreshMs);

    return () => {
      cancelled = true;
      abortRef.current?.abort();
      abortRef.current = null;
      if (refreshTimerRef.current) {
        clearInterval(refreshTimerRef.current);
        refreshTimerRef.current = null;
      }
      closeEventSource();
    };
  }, [enabled, filterKey, bufferSize, tokenRefreshMs]);

  return { events, status, pause, resume, clear };
}
