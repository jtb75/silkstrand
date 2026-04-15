import { Link } from 'react-router-dom';
import type { SuggestedAction } from '../../api/client';
import './DashboardWidgets.css';

interface Props {
  items: SuggestedAction[];
  isLoading?: boolean;
  error?: unknown;
}

// Action kind → Assets page filter URL. Until collections-backed
// predicates land in P4 these are inline querystrings the Assets page
// already knows how to parse.
function actionLink(a: SuggestedAction): string {
  switch (a.kind) {
    case 'endpoints_missing_credentials':
      return `/assets?${a.collection_id_or_inline_predicate}`;
    case 'assets_without_scans':
      return `/assets?${a.collection_id_or_inline_predicate}`;
    case 'recent_scan_failures':
      return `/scans?${a.collection_id_or_inline_predicate}`;
  }
}

function ctaLabel(cta: string): string {
  switch (cta) {
    case 'map-credentials': return 'Map Credentials';
    case 'create-scan': return 'Create Scan';
    case 'review-failures': return 'Review Failures';
    case 'retry': return 'Retry';
    case 'view': return 'View';
    default: return cta;
  }
}

function ctaTarget(cta: string, a: SuggestedAction): string {
  if (cta === 'create-scan') return '/scans/definitions/new';
  if (cta === 'map-credentials') return '/settings/credentials';
  if (cta === 'review-failures' || cta === 'retry') return actionLink(a);
  return actionLink(a);
}

export function SuggestedActions({ items, isLoading, error }: Props) {
  return (
    <div className="dash-card">
      <h3>Suggested Actions</h3>
      {isLoading && <p>Loading…</p>}
      {!!error && <p className="error">Failed to load suggestions.</p>}
      {!isLoading && !error && items.length === 0 && (
        <p style={{ color: 'var(--muted,#6b7280)', fontSize: 13 }}>
          Nothing to do — coverage is clean.
        </p>
      )}
      {items.map((a) => (
        <div key={a.kind} className="suggested-action">
          <div className="sa-title">
            <Link to={actionLink(a)}>{a.title}</Link>
          </div>
          <div className="sa-ctas">
            <Link className="btn btn-sm" to={ctaTarget(a.primary_cta, a)}>
              {ctaLabel(a.primary_cta)}
            </Link>
            <Link className="btn btn-sm btn-secondary" to={ctaTarget(a.secondary_cta, a)}>
              {ctaLabel(a.secondary_cta)}
            </Link>
          </div>
        </div>
      ))}
    </div>
  );
}
