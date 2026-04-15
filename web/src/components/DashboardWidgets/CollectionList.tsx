import { Link } from 'react-router-dom';
import './DashboardWidgets.css';

// A generic widget that displays a list of rows and a "View all" link.
// The parent feeds in already-resolved rows (collections-backed queries
// land in P4; today the Dashboard's Unclassified Endpoints widget
// sources its rows from listAssets with a predicate). Keeping this
// widget predicate-agnostic lets future P4 changes swap the data
// source without touching the visual shell.
interface Row {
  id: string;
  primary: string;
  secondary?: string;
  badge?: string;
}

interface Props {
  title: string;
  rows: Row[];
  viewAllHref: string;
  isLoading?: boolean;
  error?: unknown;
  emptyMessage?: string;
}

export function CollectionList({
  title,
  rows,
  viewAllHref,
  isLoading,
  error,
  emptyMessage = 'Nothing here yet.',
}: Props) {
  return (
    <div className="dash-card">
      <h3>
        {title}
        {rows.length > 0 && (
          <span style={{ marginLeft: 8, color: 'var(--muted,#6b7280)' }}>
            ({rows.length})
          </span>
        )}
      </h3>
      {isLoading && <p>Loading…</p>}
      {error && <p className="error">Failed to load.</p>}
      {!isLoading && !error && rows.length === 0 && (
        <p style={{ color: 'var(--muted,#6b7280)', fontSize: 13 }}>{emptyMessage}</p>
      )}
      {rows.map((r) => (
        <div key={r.id} className="collection-list-row">
          <div>
            <div>{r.primary}</div>
            {r.secondary && <div className="cl-meta">{r.secondary}</div>}
          </div>
          {r.badge && <div className="cl-meta">{r.badge}</div>}
        </div>
      ))}
      {rows.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <Link to={viewAllHref}>View all →</Link>
        </div>
      )}
    </div>
  );
}
