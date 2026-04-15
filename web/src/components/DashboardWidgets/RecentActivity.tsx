import type { RecentActivityItem } from '../../api/client';
import './DashboardWidgets.css';

interface Props {
  items: RecentActivityItem[];
  isLoading?: boolean;
  error?: unknown;
}

function icon(eventType: string): string {
  switch (eventType) {
    case 'new_asset': return '+';
    case 'version_changed': return '~';
    case 'new_cve': return '!';
    case 'cve_resolved': return '✓';
    case 'port_opened': return '↑';
    case 'scan_completed': return '✓';
    case 'scan_failed': return '✗';
    default: return '·';
  }
}

function label(e: RecentActivityItem): string {
  const host = e.hostname || e.primary_ip || e.asset_endpoint_id.slice(0, 8);
  const port = e.port ? `:${e.port}` : '';
  const svc = e.service ? ` ${e.service}` : '';
  return `${e.event_type} — ${host}${port}${svc}`;
}

function relTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return 'just now';
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

export function RecentActivity({ items, isLoading, error }: Props) {
  return (
    <div className="dash-card">
      <h3>Recent Activity</h3>
      {isLoading && <p>Loading…</p>}
      {error && <p className="error">Failed to load activity.</p>}
      {!isLoading && !error && items.length === 0 && (
        <p style={{ color: 'var(--muted,#6b7280)', fontSize: 13 }}>
          No recent activity.
        </p>
      )}
      {items.map((e) => (
        <div key={e.id} className="activity-row">
          <span className="a-icon">{icon(e.event_type)}</span>
          <span>{label(e)}</span>
          <span className="a-when">{relTime(e.occurred_at)}</span>
        </div>
      ))}
    </div>
  );
}
