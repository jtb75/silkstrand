import { useQuery } from '@tanstack/react-query';
import { getAsset } from '../api/client';
import type { CVE, DiscoveredAsset } from '../api/types';
import AllowlistBadge from './AllowlistBadge';
import AssetEventTimeline from './AssetEventTimeline';

interface Props {
  assetId: string;
  onClose: () => void;
}

function asCVEs(v: unknown): CVE[] {
  return Array.isArray(v) ? (v as CVE[]) : [];
}

function asTechnologies(v: unknown): string[] {
  if (Array.isArray(v)) return v as string[];
  return [];
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
          <h2>{isLoading ? 'Loading…' : data ? `Asset ${data.asset.ip}:${data.asset.port}` : 'Asset'}</h2>
          <button type="button" className="btn btn-sm" onClick={onClose}>×</button>
        </header>
        {error && <p className="error">{(error as Error).message}</p>}
        {data && <AssetBody asset={data.asset} events={data.events} />}
      </aside>
    </>
  );
}

function AssetBody({ asset, events }: { asset: DiscoveredAsset; events: import('../api/types').AssetEvent[] }) {
  const cves = asCVEs(asset.cves);
  const techs = asTechnologies(asset.technologies);
  return (
    <div className="drawer-body">
      <section>
        {asset.hostname && <p><strong>{asset.hostname}</strong></p>}
        <dl className="kv">
          <dt>service</dt><dd>{asset.service || '-'}</dd>
          <dt>version</dt><dd>{asset.version || '-'}</dd>
          <dt>source</dt><dd>{asset.source}</dd>
          <dt>first seen</dt><dd>{new Date(asset.first_seen).toLocaleString()}</dd>
          <dt>last seen</dt><dd>{new Date(asset.last_seen).toLocaleString()}</dd>
          <dt>scan policy</dt>
          <dd><AllowlistBadge status="unknown" /></dd>
        </dl>
      </section>
      {techs.length > 0 && (
        <section>
          <h3>Technologies</h3>
          <p>{techs.join(', ')}</p>
        </section>
      )}
      {cves.length > 0 && (
        <section>
          <h3>CVEs ({cves.length})</h3>
          <ul className="cve-list">
            {cves.map((c) => (
              <li key={c.id} className={`cve cve-${c.severity || 'info'}`}>
                <strong>{c.id}</strong>
                {c.severity && <span className="muted"> · {c.severity}</span>}
                {c.template && <span className="muted"> · {c.template}</span>}
              </li>
            ))}
          </ul>
        </section>
      )}
      {asset.compliance_status && (
        <section>
          <h3>Compliance</h3>
          <p>{asset.compliance_status}</p>
        </section>
      )}
      <section>
        <h3>Events</h3>
        <AssetEventTimeline events={events} />
      </section>
    </div>
  );
}
