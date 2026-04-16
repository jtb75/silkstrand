/**
 * AgentLogConsole — live-tail of `agent_log` events from the SSE stream.
 *
 * Two filter modes per ADR 008 D4:
 *
 *   { agentId }  → all logs from a specific agent (Agents → Console drawer)
 *   { scanId  }  → logs tagged with that scan_id (Scan Results → Console tab)
 *
 * Exactly one of the two should be set. The hook filter is built from
 * whichever is present: agentId becomes {resource_type, resource_id};
 * scanId becomes {scan_id}. Both flavors bake `kind: "agent_log"` into
 * the stream token.
 */
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useEventStream, type StreamEvent, type EventFilter } from '../lib/events';
import { formatAbsolute, formatRelative } from '../lib/time';

export interface AgentLogConsoleProps {
  filter: { agentId?: string; scanId?: string };
}

// Payload shape the agent's slog → tunnel handler emits. See
// `agent/internal/logstream/tunnel_handler.go` — the Handle method builds
// {level, msg, scan_id?, attrs?}.
interface AgentLogPayload {
  level?: string;
  msg?: string;
  scan_id?: string;
  attrs?: Record<string, unknown>;
}

type LogEvent = StreamEvent<AgentLogPayload>;

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

export default function AgentLogConsole({ filter }: AgentLogConsoleProps) {
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

  const { events, status, pause, resume, clear } = useEventStream<AgentLogPayload>(
    streamFilter,
    { enabled: Boolean(filter.agentId || filter.scanId) },
  );

  // Local paused flag mirrors the hook's internal flag so we can render
  // the button label correctly. The hook exposes pause/resume as setters;
  // we track "what we asked for" here.
  const [paused, setPaused] = useState(false);
  const onPause = () => { pause(); setPaused(true); };
  const onResume = () => { resume(); setPaused(false); };

  // Auto-scroll: pin to bottom unless the user has scrolled up. The
  // "sticky" threshold is 24px — anything within that of the bottom is
  // still considered at-bottom.
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
  }, [events]);

  const pill = statusLabel(status);
  const showScanTag = Boolean(filter.agentId); // agent-level console flags scan-scoped lines
  const isThrottled = (e: LogEvent) => e.payload?.msg === 'agent_log.throttled';

  return (
    <div className="log-console">
      <div className="log-toolbar">
        <div className="log-toolbar-left">
          <span className={`log-pill ${pill.klass}`}>{pill.label}</span>
          <span className="log-meta muted">
            {events.length} line{events.length === 1 ? '' : 's'}
            {paused ? ' · paused' : ''}
          </span>
        </div>
        <div className="log-toolbar-right">
          {paused ? (
            <button className="btn btn-sm" onClick={onResume}>Resume</button>
          ) : (
            <button className="btn btn-sm" onClick={onPause}>Pause</button>
          )}
          <button className="btn btn-sm" onClick={clear}>Clear</button>
        </div>
      </div>

      <div className="log-body" ref={bodyRef}>
        {events.length === 0 ? (
          <div className="log-empty muted">Waiting for log lines…</div>
        ) : (
          events.map((e, i) => {
            if (isThrottled(e)) {
              const dropped = (e.payload?.attrs?.['dropped'] ?? '?') as number | string;
              const window = (e.payload?.attrs?.['window'] ?? '') as string;
              return (
                <div key={i} className="log-line log-line-throttled muted">
                  <span
                    className="log-time"
                    title={formatAbsolute(e.occurred_at)}
                  >
                    {formatRelative(e.occurred_at)}
                  </span>{' '}
                  <span aria-hidden="true">🕒</span>{' '}
                  <span>[throttled] dropped {dropped} line{dropped === 1 ? '' : 's'}{window ? ` over ${window}` : ''}</span>
                </div>
              );
            }
            const lvl = (e.payload?.level ?? 'INFO').toUpperCase();
            const scanTag = showScanTag && e.payload?.scan_id ? e.payload.scan_id : null;
            return (
              <div key={i} className="log-line">
                <span
                  className="log-time"
                  title={formatAbsolute(e.occurred_at)}
                >
                  {formatRelative(e.occurred_at)}
                </span>{' '}
                <span className={`log-badge ${levelClass(lvl)}`}>{lvl}</span>{' '}
                <span className="log-msg">{e.payload?.msg ?? ''}</span>
                {scanTag ? (
                  <span className="log-scan-tag" title="scan id">
                    {' '}scan_id={scanTag.slice(0, 8)}…
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
