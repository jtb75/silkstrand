import { Fragment, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { getAsset, promoteAsset } from '../api/client';
import type {
  AssetDetailResponse,
  AssetEndpoint,
  AssetSuggestion,
  CVE,
  CoverageRollup,
  DiscoveredAsset,
  RiskRollup,
} from '../api/types';
import AllowlistBadge from './AllowlistBadge';
import AssetEventTimeline from './AssetEventTimeline';

// ui-shape.md §Asset detail view — six sections, in reading order:
//   ❶ Identity  ❷ Lifecycle  ❸ Risk posture  ❹ Endpoints
//   ❺ Coverage roll-up  ❻ Relationships (placeholder)
// Risk + Coverage come from the API response (risk, coverage, endpoints).
// When the backend hasn't populated them we fall back to values derived
// from the DiscoveredAsset row so the drawer still renders usefully.

interface Props {
  assetId: string;
  onClose: () => void;
}

function asCVEs(v: unknown): CVE[] {
  return Array.isArray(v) ? (v as CVE[]) : [];
}

function asTechnologies(v: unknown): string[] {
  return Array.isArray(v) ? (v as string[]) : [];
}

export default function AssetDetailDrawer({ assetId, onClose }: Props) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['asset', assetId],
    queryFn: () => getAsset(assetId),
    enabled: !!assetId,
  });

  return (
    <>
      <div className="drawer-backdrop" onClick={onClose} />
      <aside className="drawer">
        <header className="drawer-header">
          <h2>
            {isLoading
              ? 'Loading…'
              : data
                ? data.asset.hostname || `${data.asset.ip}:${data.asset.port}`
                : 'Asset'}
          </h2>
          <button type="button" className="btn btn-sm" onClick={onClose}>×</button>
        </header>
        {error && <p className="error">{(error as Error).message}</p>}
        {data && <AssetBody detail={data} />}
      </aside>
    </>
  );
}

function AssetBody({ detail }: { detail: AssetDetailResponse }) {
  const { asset, events, risk, coverage, endpoints, provenance } = detail;
  const cves = asCVEs(asset.cves);
  const suggestions = asset.metadata?.suggested ?? [];

  // Derive fallbacks when the backend doesn't yet return roll-ups.
  const riskOrDerived = risk ?? deriveRisk(cves);
  const coverageOrDerived =
    coverage ?? deriveCoverage(asset, endpoints ?? []);

  return (
    <div className="drawer-body">
      {/* ❶ Identity */}
      <Section n="❶" title="Identity">
        <dl className="kv">
          <dt>primary IP</dt><dd>{asset.ip}</dd>
          <dt>hostname</dt><dd>{asset.hostname || '-'}</dd>
          <dt>resource type</dt><dd>{asset.resource_type || 'host'}</dd>
          <dt>environment</dt><dd>{asset.environment || '-'}</dd>
          <dt>source</dt><dd>{asset.source}</dd>
          <dt>scan policy</dt><dd><AllowlistBadge status={asset.allowlist_status} /></dd>
        </dl>
        {asTechnologies(asset.technologies).length > 0 && (
          <p className="muted" style={{ fontSize: 12 }}>
            Technologies: {asTechnologies(asset.technologies).join(', ')}
          </p>
        )}
      </Section>

      {/* ❷ Lifecycle */}
      <Section n="❷" title="Lifecycle">
        <dl className="kv">
          <dt>first seen</dt><dd>{new Date(asset.first_seen).toLocaleString()}</dd>
          <dt>last seen</dt><dd>{new Date(asset.last_seen).toLocaleString()}</dd>
          <dt>missed scans</dt><dd>{asset.missed_scan_count}</dd>
          {provenance?.first_target_id && (
            <>
              <dt>first target</dt><dd>{provenance.first_target_id}</dd>
            </>
          )}
          {provenance?.first_agent_id && (
            <>
              <dt>first agent</dt><dd>{provenance.first_agent_id}</dd>
            </>
          )}
          {provenance?.first_scan_id && (
            <>
              <dt>first scan</dt><dd>{provenance.first_scan_id}</dd>
            </>
          )}
        </dl>
      </Section>

      {/* ❸ Risk posture */}
      <Section n="❸" title="Risk posture">
        <RiskPanel risk={riskOrDerived} />
      </Section>

      {/* Suggestions (pre-existing approve/promote flow) */}
      {suggestions.length > 0 && (
        <SuggestionsSection
          assetId={asset.id}
          suggestions={suggestions}
          outOfPolicy={asset.allowlist_status === 'out_of_policy'}
        />
      )}

      {/* ❹ Endpoints */}
      <Section n="❹" title="Endpoints">
        <EndpointsSection asset={asset} endpoints={endpoints ?? []} />
      </Section>

      {/* ❺ Coverage roll-up */}
      <Section n="❺" title="Coverage roll-up">
        <CoveragePanel coverage={coverageOrDerived} />
      </Section>

      {/* ❻ Relationships (placeholder) */}
      <Section n="❻" title="Relationships">
        <p className="muted">
          depends-on / parent-of / peers will populate once container and cloud
          ingest land. Placeholder for now.
        </p>
      </Section>

      <Section n="" title="Events">
        <AssetEventTimeline events={events} />
      </Section>
    </div>
  );
}

function Section({
  n,
  title,
  children,
}: {
  n: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section style={{ marginBottom: 20 }}>
      <h3 style={{ marginBottom: 8 }}>
        {n && <span style={{ marginRight: 6 }}>{n}</span>}
        {title}
      </h3>
      {children}
    </section>
  );
}

// ── Risk ─────────────────────────────────────────────────────────────────────
function deriveRisk(cves: CVE[]): RiskRollup {
  const counts = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
  for (const c of cves) {
    const sev = (c.severity || 'info') as keyof typeof counts;
    if (sev in counts) counts[sev] += 1;
  }
  const order: (keyof typeof counts)[] = ['critical', 'high', 'medium', 'low', 'info'];
  const max = order.find((k) => counts[k] > 0);
  return {
    ...counts,
    max_severity: max,
    top_findings: cves.slice(0, 3).map((c) => ({
      id: c.id,
      title: c.id,
      severity: c.severity || 'info',
    })),
  };
}

function RiskPanel({ risk }: { risk: RiskRollup }) {
  return (
    <div>
      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
        <Pill label="Critical" count={risk.critical} sev="critical" />
        <Pill label="High" count={risk.high} sev="high" />
        <Pill label="Medium" count={risk.medium} sev="medium" />
        <Pill label="Low" count={risk.low} sev="low" />
      </div>
      {risk.delta_since_last_scan != null && (
        <p className="muted" style={{ marginTop: 8 }}>
          Δ since last scan: {risk.delta_since_last_scan >= 0 ? '+' : ''}
          {risk.delta_since_last_scan}
        </p>
      )}
      {risk.top_findings && risk.top_findings.length > 0 && (
        <ul className="cve-list" style={{ marginTop: 8 }}>
          {risk.top_findings.map((f) => (
            <li key={f.id} className={`cve cve-${f.severity}`}>
              <strong>{f.title}</strong>
              <span className="muted"> · {f.severity}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Pill({ label, count, sev }: { label: string; count: number; sev: string }) {
  return (
    <span className={`badge badge-cve-${sev}`} style={{ padding: '2px 10px' }}>
      {label}: {count}
    </span>
  );
}

// ── Endpoints ────────────────────────────────────────────────────────────────
function EndpointsSection({
  asset,
  endpoints,
}: {
  asset: DiscoveredAsset;
  endpoints: AssetEndpoint[];
}) {
  // Degrade gracefully: if backend doesn't yet return endpoints[], show a
  // single synthetic row derived from the asset itself (asset_id = port).
  const rows: AssetEndpoint[] =
    endpoints.length > 0
      ? endpoints
      : [
          {
            id: asset.id,
            asset_id: asset.id,
            port: asset.port,
            service: asset.service,
            version: asset.version,
            findings_count: Array.isArray(asset.cves) ? asset.cves.length : 0,
            coverage: asset.coverage,
          },
        ];

  const [openId, setOpenId] = useState<string | null>(null);

  return (
    <table className="table" style={{ fontSize: 13 }}>
      <thead>
        <tr>
          <th>Port</th>
          <th>Service</th>
          <th>Version</th>
          <th>Findings</th>
          <th>Coverage</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((ep) => {
          const open = openId === ep.id;
          const cov = ep.coverage ?? { scan_configured: false, creds_mapped: false };
          return (
            <Fragment key={ep.id}>
              <tr
                className="clickable-row"
                onClick={() => setOpenId(open ? null : ep.id)}
              >
                <td>{ep.port}</td>
                <td>{ep.service || '-'}</td>
                <td>{ep.version || '-'}</td>
                <td>{ep.findings_count ?? '-'}</td>
                <td>
                  <span style={{ color: cov.scan_configured ? '#2a9d8f' : '#e63946' }}>
                    {cov.scan_configured ? '✔' : '❌'}
                  </span>{' '}
                  /{' '}
                  <span style={{ color: cov.creds_mapped ? '#2a9d8f' : '#e63946' }}>
                    {cov.creds_mapped ? '✔' : '❌'}
                  </span>
                </td>
              </tr>
              {open && (
                <tr>
                  <td colSpan={5} className="muted">
                    <div style={{ padding: '6px 4px' }}>
                      <div>
                        <strong>Service:</strong> {ep.service || '-'} {ep.version || ''}
                      </div>
                      {ep.fingerprint && (
                        <pre style={{ fontSize: 11, margin: '4px 0' }}>
                          {JSON.stringify(ep.fingerprint, null, 2)}
                        </pre>
                      )}
                      <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
                        <button className="btn btn-sm">Configure scan</button>
                        <button className="btn btn-sm">Map credential</button>
                      </div>
                    </div>
                  </td>
                </tr>
              )}
            </Fragment>
          );
        })}
      </tbody>
    </table>
  );
}

// ── Coverage ─────────────────────────────────────────────────────────────────
function deriveCoverage(asset: DiscoveredAsset, endpoints: AssetEndpoint[]): CoverageRollup {
  const list = endpoints.length > 0
    ? endpoints
    : [
        {
          id: asset.id,
          asset_id: asset.id,
          port: asset.port,
          service: asset.service,
          coverage: asset.coverage,
        } as AssetEndpoint,
      ];
  const total = list.length;
  const withScan = list.filter((e) => e.coverage?.scan_configured).length;
  const withCreds = list.filter((e) => e.coverage?.creds_mapped).length;
  const gaps: CoverageRollup['gaps'] = [];
  for (const ep of list) {
    const c = ep.coverage ?? { scan_configured: false, creds_mapped: false };
    if (!c.scan_configured) {
      gaps.push({
        endpoint_id: ep.id,
        ip: asset.ip,
        port: ep.port,
        service: ep.service,
        reason: 'no_scan',
      });
    } else if (!c.creds_mapped) {
      gaps.push({
        endpoint_id: ep.id,
        ip: asset.ip,
        port: ep.port,
        service: ep.service,
        reason: 'no_creds',
      });
    }
  }
  return {
    endpoints_total: total,
    endpoints_with_scan: withScan,
    endpoints_with_creds: withCreds,
    gaps,
  };
}

function pct(n: number, d: number) {
  if (d === 0) return '—';
  return `${Math.round((100 * n) / d)}%`;
}

function CoveragePanel({ coverage }: { coverage: CoverageRollup }) {
  return (
    <div>
      <dl className="kv">
        <dt>endpoints with scan</dt>
        <dd>
          {coverage.endpoints_with_scan}/{coverage.endpoints_total}{' '}
          ({pct(coverage.endpoints_with_scan, coverage.endpoints_total)})
        </dd>
        <dt>endpoints with creds</dt>
        <dd>
          {coverage.endpoints_with_creds}/{coverage.endpoints_total}{' '}
          ({pct(coverage.endpoints_with_creds, coverage.endpoints_total)})
        </dd>
        {coverage.last_scan_at && (
          <>
            <dt>last scan</dt>
            <dd>{new Date(coverage.last_scan_at).toLocaleString()}</dd>
          </>
        )}
        {coverage.next_scan_at && (
          <>
            <dt>next scan</dt>
            <dd>{new Date(coverage.next_scan_at).toLocaleString()}</dd>
          </>
        )}
      </dl>
      {coverage.gaps.length > 0 ? (
        <>
          <h4 style={{ marginTop: 12 }}>Gaps</h4>
          <ul>
            {coverage.gaps.map((g) => (
              <li key={`${g.endpoint_id}:${g.reason}`}>
                {g.ip}:{g.port} {g.service ? `(${g.service}) ` : ''}— {labelGap(g.reason)}{' '}
                <button className="btn btn-sm" style={{ marginLeft: 4 }}>
                  {g.reason === 'no_creds' ? 'Map credential' : 'Configure scan'}
                </button>
              </li>
            ))}
          </ul>
        </>
      ) : (
        <p className="muted">No coverage gaps.</p>
      )}
      <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
        <button className="btn btn-sm">Map Credential</button>
        <button className="btn btn-sm">Create Scan</button>
      </div>
    </div>
  );
}

function labelGap(r: CoverageRollup['gaps'][number]['reason']): string {
  if (r === 'no_scan') return 'no scan configured';
  if (r === 'no_creds') return 'no credential mapped';
  return 'recent scan failure';
}

// ── Suggestions (preserved from prior drawer) ───────────────────────────────
function SuggestionsSection({
  assetId,
  suggestions,
  outOfPolicy,
}: {
  assetId: string;
  suggestions: AssetSuggestion[];
  outOfPolicy: boolean;
}) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const promote = useMutation({
    mutationFn: (bundleId: string) => promoteAsset(assetId, bundleId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['asset', assetId] });
      qc.invalidateQueries({ queryKey: ['assets'] });
      navigate('/targets');
    },
  });
  const blockedTitle = outOfPolicy
    ? "This asset is outside the agent's scan allowlist. Update /etc/silkstrand/scan-allowlist.yaml on the agent before promoting."
    : undefined;
  return (
    <Section n="" title="Suggestions">
      {outOfPolicy && (
        <p className="muted" style={{ marginTop: 0 }}>
          Promotion is disabled — this asset is out of the agent's scan allowlist.
        </p>
      )}
      <ul className="suggestion-list">
        {suggestions.map((s) => (
          <li key={`${s.rule_name}:${s.bundle_id}`} className="suggestion">
            <div>
              <strong>{s.bundle_id}</strong>
              <span className="muted"> · rule {s.rule_name}</span>
            </div>
            <button
              type="button"
              className="btn btn-sm"
              disabled={promote.isPending || outOfPolicy}
              title={blockedTitle}
              onClick={() => promote.mutate(s.bundle_id)}
            >
              {promote.isPending ? 'Promoting…' : 'Approve'}
            </button>
          </li>
        ))}
      </ul>
      {promote.error && <p className="error">{(promote.error as Error).message}</p>}
    </Section>
  );
}
