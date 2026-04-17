/**
 * AgentLogConsole — loads persisted log history on mount, then live-tails
 * via SSE for new events.
 *
 * Two filter modes:
 *   { agentId }  -> all logs from a specific agent (Agents -> Console drawer)
 *   { scanId  }  -> logs tagged with that scan_id (Scan Results -> Console tab)
 *
 * agentId is always required (it's the REST endpoint path param). scanId
 * is optional and additionally scopes the history fetch + SSE filter.
 */
import { useEffect, useLayoutEffect, useMemo, useRef, useState, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useEventStream, type StreamEvent, type EventFilter } from '../lib/events';
import { getAgentLogs, type AgentLogEntry } from '../api/client';
import { formatAbsolute, formatRelative } from '../lib/time';

export interface AgentLogConsoleProps {
  filter: { agentId?: string; scanId?: string };
}

// Payload shape the agent's slog -> tunnel handler emits.
interface AgentLogPayload {
  level?: string;
  msg?: string;
  scan_id?: string;
  attrs?: Record<string, unknown>;
}

// Unified line type used for rendering. History lines come from the REST
// response; live lines come from SSE events. We normalize both into this.
interface LogLine {
  key: string;
  level: string;
  msg: string;
  scan_id?: string;
  attrs?: Record<string, unknown>;
  occurred_at: string;
}

const LEVEL_CLASS: Record<string, string> = {
  INFO: 'log-level-info',
  WARN: 'log-level-warn',
  ERROR: 'log-level-error',
  DEBUG: 'log-level-debug',
};

function levelClass(level?: string) {
  if (!level) return 'log-level-info';
  return LEVEL_CLASS[level.toUpperCase()] ?? 'log-level-info';
}

function statusLabel(status: string) {
  switch (status) {
    case 'connected':   return { label: 'Connected',    klass: 'log-pill-connected' };
    case 'connecting':  return { label: 'Connecting',   klass: 'log-pill-reconnecting' };
    case 'reconnecting':return { label: 'Reconnecting', klass: 'log-pill-reconnecting' };
    case 'error':       return { label: 'Error',        klass: 'log-pill-error' };
    default:            return { label: 'Idle',         klass: 'log-pill-idle' };
  }
}

function historyToLine(e: AgentLogEntry): LogLine {
  return {
    key: e.id,
    level: (e.level ?? 'INFO').toUpperCase(),
    msg: e.msg ?? '',
    scan_id: e.scan_id,
    attrs: e.attrs,
    occurred_at: e.occurred_at,
  };
}

function sseToLine(e: StreamEvent<AgentLogPayload>, idx: number): LogLine {
  return {
    key: `sse-${idx}-${e.occurred_at}`,
    level: (e.payload?.level ?? 'INFO').toUpperCase(),
    msg: e.payload?.msg ?? '',
    scan_id: e.payload?.scan_id,
    attrs: e.payload?.attrs,
    occurred_at: e.occurred_at,
  };
}

export default function AgentLogConsole({ filter }: AgentLogConsoleProps) {
  // Load history via react-query.
  const { data: historyData, isLoading: historyLoading } = useQuery({
    queryKey: ['agent-logs', filter.agentId, filter.scanId],
    queryFn: () =>
      getAgentLogs(filter.agentId!, {
        limit: 200,
        order: 'asc',
        ...(filter.scanId ? { scan_id: filter.scanId } : {}),
      }),
    enabled: Boolean(filter.agentId),
    staleTime: Infinity, // history is a snapshot; SSE handles new lines
  });

  const historyLines = useMemo(
    () => (historyData?.items ?? []).map(historyToLine),
    [historyData],
  );

  const [sseCounter, setSseCounter] = useState(0);
  const [historyCleared, setHistoryCleared] = useState(false);

  // SSE live tail.
  const streamFilter: EventFilter = useMemo(() => {
    const f: EventFilter = { kinds: ['agent_log'] };
    if (filter.agentId) {
      f.resource_type = 'agent';
      f.resource_id = filter.agentId;
    }
    if (filter.scanId) {
      f.scan_id = filter.scanId;
    }
    return f;
  }, [filter.agentId, filter.scanId]);

  const { events: sseEvents, status, pause, resume, clear: clearSse } = useEventStream<AgentLogPayload>(
    streamFilter,
    { enabled: Boolean(filter.agentId || filter.scanId) },
  );

  // Convert SSE events to LogLines.
  const liveLines = useMemo(() => {
    return sseEvents.map((e, i) => sseToLine(e, sseCounter + i));
  }, [sseEvents, sseCounter]);

  // Merged: history + live.
  const allLines = useMemo(() => {
    const base = historyCleared ? [] : historyLines;
    return [...base, ...liveLines];
  }, [historyCleared, historyLines, liveLines]);

  // Pause / resume state.
  const [paused, setPaused] = useState(false);
  const onPause = () => { pause(); setPaused(true); };
  const onResume = () => { resume(); setPaused(false); };
  const onClear = useCallback(() => {
    setHistoryCleared(true);
    clearSse();
    setSseCounter((c) => c + sseEvents.length);
  }, [clearSse, sseEvents.length]);

  // Auto-scroll.
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const stickyRef = useRef(true);

  useEffect(() => {
    const el = bodyRef.current;
    if (!el) return;
    const onScroll = () => {
      const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
      stickyRef.current = dist < 24;
    };
    el.addEventListener('scroll', onScroll, { passive: true });
    return () => el.removeEventListener('scroll', onScroll);
  }, []);

  useLayoutEffect(() => {
    const el = bodyRef.current;
    if (!el) return;
    if (stickyRef.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [allLines]);

  const pill = statusLabel(status);
  const showScanTag = Boolean(filter.agentId);
  const isThrottled = (line: LogLine) => line.msg === 'agent_log.throttled';

  return (
    <div className="log-console">
      <div className="log-toolbar">
        <div className="log-toolbar-left">
          <span className={`log-pill ${pill.klass}`}>{pill.label}</span>
          <span className="log-meta muted">
            {allLines.length} line{allLines.length === 1 ? '' : 's'}
            {paused ? ' · paused' : ''}
          </span>
        </div>
        <div className="log-toolbar-right">
          {paused ? (
            <button className="btn btn-sm" onClick={onResume}>Resume</button>
          ) : (
            <button className="btn btn-sm" onClick={onPause}>Pause</button>
          )}
          <button className="btn btn-sm" onClick={onClear}>Clear</button>
        </div>
      </div>

      <div className="log-body" ref={bodyRef}>
        {historyLoading && allLines.length === 0 ? (
          <div className="log-empty muted">Loading log history...</div>
        ) : allLines.length === 0 ? (
          <div className="log-empty muted">No log lines yet.</div>
        ) : (
          allLines.map((line) => {
            if (isThrottled(line)) {
              const dropped = (line.attrs?.['dropped'] ?? '?') as number | string;
              const window = (line.attrs?.['window'] ?? '') as string;
              return (
                <div key={line.key} className="log-line log-line-throttled muted">
                  <span
                    className="log-time"
                    title={formatAbsolute(line.occurred_at)}
                  >
                    {formatRelative(line.occurred_at)}
                  </span>{' '}
                  <span>[throttled] dropped {dropped} line{dropped === 1 ? '' : 's'}{window ? ` over ${window}` : ''}</span>
                </div>
              );
            }
            const scanTag = showScanTag && line.scan_id ? line.scan_id : null;
            return (
              <div key={line.key} className="log-line">
                <span
                  className="log-time"
                  title={formatAbsolute(line.occurred_at)}
                >
                  {formatRelative(line.occurred_at)}
                </span>{' '}
                <span className={`log-badge ${levelClass(line.level)}`}>{line.level}</span>{' '}
                <span className="log-msg">{line.msg}</span>
                {scanTag ? (
                  <span className="log-scan-tag" title="scan id">
                    {' '}scan_id={scanTag.slice(0, 8)}...
                  </span>
                ) : null}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
