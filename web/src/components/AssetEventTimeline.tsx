import type { AssetEvent, AssetEventType } from '../api/types';

const ICON: Record<AssetEventType, string> = {
  new_asset: '+',
  asset_gone: '×',
  asset_reappeared: '↻',
  new_cve: '!',
  cve_resolved: '✓',
  version_changed: '~',
  port_opened: '+',
  port_closed: '×',
  compliance_pass: '✓',
  compliance_fail: '!',
};

interface Props {
  events: AssetEvent[];
}

function summarize(e: AssetEvent): string {
  const p = (e.payload ?? {}) as Record<string, unknown>;
  switch (e.event_type) {
    case 'new_asset':
      return `discovered ${p.service ?? 'unknown service'}`;
    case 'version_changed':
      return `${p.from_version ?? '?'} → ${p.to_version ?? '?'}`;
    case 'new_cve':
    case 'cve_resolved':
      return String(p.cve_id ?? '');
    case 'asset_gone':
      return 'asset no longer reachable';
    case 'asset_reappeared':
      return 'asset reappeared after being gone';
    default:
      return e.event_type;
  }
}

export default function AssetEventTimeline({ events }: Props) {
  if (!events || events.length === 0) {
    return <p className="muted">No recorded events yet.</p>;
  }
  return (
    <ul className="event-timeline">
      {events.map((e) => (
        <li key={e.id} className={`event event-${e.event_type}`}>
          <span className="event-icon">{ICON[e.event_type] ?? '·'}</span>
          <span className="event-time">{new Date(e.occurred_at).toLocaleString()}</span>
          <span className="event-type">{e.event_type}</span>
          <span className="event-summary">{summarize(e)}</span>
        </li>
      ))}
    </ul>
  );
}
