import type { DiscoveredAsset, CVE } from '../api/types';

interface Props {
  assets: DiscoveredAsset[];
  onSelect: (id: string) => void;
}

function cveCount(asset: DiscoveredAsset): number {
  if (Array.isArray(asset.cves)) return asset.cves.length;
  return 0;
}

function topSeverity(asset: DiscoveredAsset): string {
  if (!Array.isArray(asset.cves) || asset.cves.length === 0) return '';
  const order = ['critical', 'high', 'medium', 'low', 'info'];
  for (const sev of order) {
    if ((asset.cves as CVE[]).some((c) => c.severity === sev)) return sev;
  }
  return '';
}

export default function AssetsTable({ assets, onSelect }: Props) {
  return (
    <table className="table">
      <thead>
        <tr>
          <th>IP:Port</th>
          <th>Hostname</th>
          <th>Service</th>
          <th>Version</th>
          <th>CVE</th>
          <th>Compliance</th>
          <th>Source</th>
          <th>Last seen</th>
        </tr>
      </thead>
      <tbody>
        {assets.map((a) => {
          const count = cveCount(a);
          const sev = topSeverity(a);
          return (
            <tr key={a.id} className="clickable-row" onClick={() => onSelect(a.id)}>
              <td>{a.ip}:{a.port}</td>
              <td>{a.hostname || '-'}</td>
              <td>{a.service || '-'}</td>
              <td>{a.version || '-'}</td>
              <td>
                {count > 0 ? (
                  <span className={`badge badge-cve-${sev || 'info'}`}>{count}</span>
                ) : (
                  '-'
                )}
              </td>
              <td>{a.compliance_status || '-'}</td>
              <td>{a.source === 'manual' ? 'M' : 'D'}</td>
              <td>{new Date(a.last_seen).toLocaleString()}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
