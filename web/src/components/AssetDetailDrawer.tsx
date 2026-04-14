import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { getAsset, promoteAsset } from '../api/client';
import type { AssetSuggestion, CVE, DiscoveredAsset } from '../api/types';
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
  const suggestions = asset.metadata?.suggested ?? [];
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
          <dd><AllowlistBadge status={asset.allowlist_status} /></dd>
        </dl>
      </section>
      {suggestions.length > 0 && (
        <SuggestionsSection
          assetId={asset.id}
          suggestions={suggestions}
          outOfPolicy={asset.allowlist_status === 'out_of_policy'}
        />
      )}
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
  const queryClient = useQueryClient();
  const promote = useMutation({
    mutationFn: (bundleId: string) => promoteAsset(assetId, bundleId),
    onSuccess: (resp) => {
      queryClient.invalidateQueries({ queryKey: ['asset', assetId] });
      queryClient.invalidateQueries({ queryKey: ['assets'] });
      navigate(`/targets`);
      void resp;
    },
  });
  const blockedTitle = outOfPolicy
    ? "This asset is outside the agent's scan allowlist. Update /etc/silkstrand/scan-allowlist.yaml on the agent before promoting."
    : undefined;
  return (
    <section className="suggestion-section">
      <h3>Suggestions</h3>
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
    </section>
  );
}
