import { useMemo, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listAssets,
  listCollections,
  createCollection,
  listScans,
  type AssetFilterParams,
} from '../api/client';
import type {
  CVE,
  Collection,
  DiscoveredAsset,
  Scan,
} from '../api/types';
import AssetsFilterChips, { type ChipId } from '../components/AssetsFilterChips';
import AssetDetailDrawer from '../components/AssetDetailDrawer';
import AssetsBulkActions from '../components/AssetsBulkActions';
import PredicateBuilder, { type Predicate } from '../components/PredicateBuilder';

// Three tabs over one filtered population per docs/plans/ui-shape.md §Assets:
//   · Assets     — one row per asset (host-level)
//   · Endpoints  — one row per asset_endpoint (port-level)
//   · Findings   — one row per finding, scoped to the filtered population
// Multi-select persists across tabs and drives the Bulk Actions bar.

type TabId = 'assets' | 'endpoints' | 'findings';

function chipsToParams(chips: Set<ChipId>): AssetFilterParams {
  const p: AssetFilterParams = {};
  if (chips.has('with_cves')) p.cve_count_gte = 1;
  if (chips.has('compliance_candidates')) {
    p.service_in = ['postgresql', 'mysql', 'mssql', 'mongodb'];
  }
  if (chips.has('failing')) p.compliance_status = 'fail';
  if (chips.has('recently_changed')) p.changed_since = '7d';
  if (chips.has('new_this_week')) p.new_since = '7d';
  if (chips.has('manual')) p.source = 'manual';
  if (chips.has('discovered')) p.source = 'discovered';
  return p;
}

function topSeverity(asset: DiscoveredAsset): string | null {
  if (asset.risk?.max_severity) return asset.risk.max_severity;
  if (!Array.isArray(asset.cves) || asset.cves.length === 0) return null;
  const order = ['critical', 'high', 'medium', 'low', 'info'];
  for (const sev of order) {
    if ((asset.cves as CVE[]).some((c) => c.severity === sev)) return sev;
  }
  return null;
}

function cveCount(asset: DiscoveredAsset): number {
  return Array.isArray(asset.cves) ? asset.cves.length : 0;
}

function Coverage({ scan, creds }: { scan: boolean; creds: boolean }) {
  return (
    <span title={`Scan ${scan ? 'configured' : 'missing'} · Creds ${creds ? 'mapped' : 'missing'}`}>
      <span style={{ color: scan ? '#2a9d8f' : '#e63946' }}>{scan ? '✔' : '❌'}</span>
      <span style={{ margin: '0 4px' }}>/</span>
      <span style={{ color: creds ? '#2a9d8f' : '#e63946' }}>{creds ? '✔' : '❌'}</span>
    </span>
  );
}

export default function Assets() {
  const navigate = useNavigate();
  const { id: selectedAssetId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = ((searchParams.get('tab') as TabId) || 'assets') as TabId;
  const [chips, setChips] = useState<Set<ChipId>>(new Set());
  const [search, setSearch] = useState('');
  const [collectionId, setCollectionId] = useState<string>('');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [saveOpen, setSaveOpen] = useState(false);

  const filters: AssetFilterParams = useMemo(
    () => ({
      ...chipsToParams(chips),
      q: search || undefined,
      page: 1,
      page_size: 200,
    }),
    [chips, search],
  );

  const qc = useQueryClient();
  const { data: assets, isLoading, error } = useQuery({
    queryKey: ['assets', filters, collectionId],
    queryFn: () => listAssets(filters),
    refetchInterval: () => {
      const scans = qc.getQueryData<Scan[]>(['scans']);
      const running = scans?.some(
        (s) => s.scan_type === 'discovery' && (s.status === 'running' || s.status === 'pending'),
      );
      return running ? 5000 : false;
    },
    refetchIntervalInBackground: false,
  });

  useQuery({
    queryKey: ['scans'],
    queryFn: () => listScans(),
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
  });

  const { data: collections } = useQuery({
    queryKey: ['collections'],
    queryFn: () => listCollections(),
  });

  function toggleChip(id: ChipId) {
    setChips((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      if (id === 'manual') next.delete('discovered');
      if (id === 'discovered') next.delete('manual');
      return next;
    });
  }

  function selectTab(t: TabId) {
    const next = new URLSearchParams(searchParams);
    if (t === 'assets') next.delete('tab');
    else next.set('tab', t);
    setSearchParams(next, { replace: true });
  }

  function selectAsset(id: string) {
    navigate(`/assets/${id}${tab !== 'assets' ? `?tab=${tab}` : ''}`);
  }

  function closeDrawer() {
    navigate(`/assets${tab !== 'assets' ? `?tab=${tab}` : ''}`);
  }

  function toggleRow(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  const items = assets?.items ?? [];
  const total = assets?.total ?? 0;
  const scanRunning = !!qc
    .getQueryData<Scan[]>(['scans'])
    ?.some((s) => s.scan_type === 'discovery' && (s.status === 'running' || s.status === 'pending'));

  return (
    <div>
      <div className="page-header">
        <h1>Assets</h1>
      </div>

      {/* Primary filter row: search + collection + Save-as-Collection */}
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 12 }}>
        <input
          type="search"
          placeholder="Search hosts, IPs, services…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ flex: 1, minWidth: 240 }}
        />
        <select
          value={collectionId}
          onChange={(e) => setCollectionId(e.target.value)}
          aria-label="Collection"
        >
          <option value="">All</option>
          {collections?.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name} ({c.scope})
            </option>
          ))}
        </select>
        <button className="btn btn-sm" onClick={() => setSaveOpen(true)}>
          Save as Collection
        </button>
      </div>

      <AssetsFilterChips
        active={chips}
        total={total}
        onToggle={toggleChip}
        onClear={() => setChips(new Set())}
        scanRunning={scanRunning}
      />

      <div className="tab-bar" role="tablist" style={{ marginBottom: 12 }}>
        <TabBtn id="assets" cur={tab} onClick={selectTab}>Assets</TabBtn>
        <TabBtn id="endpoints" cur={tab} onClick={selectTab}>Endpoints</TabBtn>
        <TabBtn id="findings" cur={tab} onClick={selectTab}>Findings</TabBtn>
      </div>

      {error && <p className="error">{(error as Error).message}</p>}
      {isLoading && <p>Loading…</p>}

      {!isLoading && !error && tab === 'assets' && (
        <AssetsView
          items={items}
          selected={selected}
          onToggle={toggleRow}
          onSelect={selectAsset}
        />
      )}
      {!isLoading && !error && tab === 'endpoints' && (
        <EndpointsView
          items={items}
          selected={selected}
          onToggle={toggleRow}
          onSelect={selectAsset}
        />
      )}
      {!isLoading && !error && tab === 'findings' && <FindingsView items={items} />}

      <AssetsBulkActions
        selectionCount={selected.size}
        resolveEndpointIds={() => Array.from(selected)}
        onClear={() => setSelected(new Set())}
      />

      {saveOpen && (
        <SaveAsCollectionModal
          scope={tab === 'findings' ? 'finding' : tab === 'endpoints' ? 'endpoint' : 'asset'}
          seedPredicate={filtersToPredicate(filters)}
          onClose={() => setSaveOpen(false)}
        />
      )}

      {selectedAssetId && (
        <AssetDetailDrawer assetId={selectedAssetId} onClose={closeDrawer} />
      )}
    </div>
  );
}

function TabBtn({
  id,
  cur,
  onClick,
  children,
}: {
  id: TabId;
  cur: TabId;
  onClick: (t: TabId) => void;
  children: React.ReactNode;
}) {
  return (
    <button
      role="tab"
      aria-selected={cur === id}
      className={`btn btn-sm ${cur === id ? 'btn-primary' : ''}`}
      style={{ marginRight: 8 }}
      onClick={() => onClick(id)}
    >
      {children}
    </button>
  );
}

// ── Assets tab ───────────────────────────────────────────────────────────────
function AssetsView({
  items,
  selected,
  onToggle,
  onSelect,
}: {
  items: DiscoveredAsset[];
  selected: Set<string>;
  onToggle: (id: string) => void;
  onSelect: (id: string) => void;
}) {
  if (items.length === 0)
    return <p className="muted">No assets. Create a target or trigger a discovery scan.</p>;
  return (
    <table className="table">
      <thead>
        <tr>
          <th style={{ width: 32 }}></th>
          <th>Host</th>
          <th>IP</th>
          <th>Type</th>
          <th>Env</th>
          <th>#Endpoints</th>
          <th>Max severity</th>
          <th>Coverage</th>
          <th>Last seen</th>
        </tr>
      </thead>
      <tbody>
        {items.map((a) => {
          const sev = topSeverity(a);
          const cov = a.coverage ?? { scan_configured: false, creds_mapped: false };
          return (
            <tr
              key={a.id}
              className="clickable-row"
              onClick={() => onSelect(a.id)}
            >
              <td onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={selected.has(a.id)}
                  onChange={() => onToggle(a.id)}
                  aria-label={`Select ${a.hostname || a.ip}`}
                />
              </td>
              <td>{a.hostname || '-'}</td>
              <td>{a.ip}</td>
              <td>{a.resource_type || a.service || '-'}</td>
              <td>{a.environment || '-'}</td>
              <td>{a.endpoints_count ?? '—'}</td>
              <td>
                {sev ? <span className={`badge badge-cve-${sev}`}>{sev}</span> : '—'}
              </td>
              <td>
                <Coverage scan={cov.scan_configured} creds={cov.creds_mapped} />
              </td>
              <td>{new Date(a.last_seen).toLocaleString()}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// ── Endpoints tab ────────────────────────────────────────────────────────────
function EndpointsView({
  items,
  selected,
  onToggle,
  onSelect,
}: {
  items: DiscoveredAsset[];
  selected: Set<string>;
  onToggle: (id: string) => void;
  onSelect: (id: string) => void;
}) {
  // Until P4-backend exposes a dedicated /endpoints endpoint, the asset
  // list doubles as the endpoint list (current schema is port-scoped per
  // DiscoveredAsset row). This keeps the UI usable during the migration.
  if (items.length === 0) return <p className="muted">No endpoints.</p>;
  return (
    <table className="table">
      <thead>
        <tr>
          <th style={{ width: 32 }}></th>
          <th>Host</th>
          <th>IP:Port</th>
          <th>Service</th>
          <th>Tech</th>
          <th>Findings</th>
          <th>Coverage</th>
        </tr>
      </thead>
      <tbody>
        {items.map((a) => {
          const cov = a.coverage ?? { scan_configured: false, creds_mapped: false };
          const techs = Array.isArray(a.technologies) ? (a.technologies as string[]).join(', ') : '';
          return (
            <tr
              key={a.id}
              className="clickable-row"
              onClick={() => onSelect(a.id)}
            >
              <td onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={selected.has(a.id)}
                  onChange={() => onToggle(a.id)}
                  aria-label={`Select ${a.ip}:${a.port}`}
                />
              </td>
              <td>{a.hostname || '-'}</td>
              <td>{a.ip}:{a.port}</td>
              <td>{a.service || '-'}</td>
              <td>{techs || '-'}</td>
              <td>{cveCount(a) || '-'}</td>
              <td>
                <Coverage scan={cov.scan_configured} creds={cov.creds_mapped} />
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// ── Findings tab ─────────────────────────────────────────────────────────────
function FindingsView({ items }: { items: DiscoveredAsset[] }) {
  // ADR 007 splits findings into its own table; until P5 ships a Findings
  // API we derive a minimal view from the CVE arrays already on the asset
  // rows. This keeps the tab functional and honest about its data source.
  type Row = {
    key: string;
    assetId: string;
    severity: string;
    title: string;
    source: string;
    asset: string;
    lastSeen: string;
  };
  const rows: Row[] = [];
  for (const a of items) {
    if (!Array.isArray(a.cves)) continue;
    for (const c of a.cves as CVE[]) {
      rows.push({
        key: `${a.id}:${c.id}`,
        assetId: a.id,
        severity: c.severity || 'info',
        title: c.id,
        source: c.template || 'nuclei',
        asset: `${a.ip}:${a.port}`,
        lastSeen: a.last_seen,
      });
    }
  }
  if (rows.length === 0)
    return <p className="muted">No findings in the current filtered population.</p>;
  return (
    <table className="table">
      <thead>
        <tr>
          <th>Severity</th>
          <th>Title</th>
          <th>Source</th>
          <th>Asset</th>
          <th>Last seen</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.key}>
            <td><span className={`badge badge-cve-${r.severity}`}>{r.severity}</span></td>
            <td>{r.title}</td>
            <td>{r.source}</td>
            <td>{r.asset}</td>
            <td>{new Date(r.lastSeen).toLocaleString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// Derive a starting predicate from the current Assets filter state so the
// "Save as Collection" flow seeds with a useful object instead of `{}`.
function filtersToPredicate(f: AssetFilterParams): Predicate {
  const clauses: Predicate[] = [];
  if (f.service_in?.length) clauses.push({ service: { $in: f.service_in } });
  if (f.service) clauses.push({ service: f.service });
  if (f.source) clauses.push({ source: f.source });
  if (f.compliance_status) clauses.push({ compliance_status: f.compliance_status });
  if (f.cve_count_gte != null) clauses.push({ cve_count: { $gte: f.cve_count_gte } });
  if (f.q) clauses.push({ q: f.q });
  if (clauses.length === 0) return {};
  if (clauses.length === 1) return clauses[0];
  return { $and: clauses };
}

function SaveAsCollectionModal({
  scope,
  seedPredicate,
  onClose,
}: {
  scope: 'asset' | 'endpoint' | 'finding';
  seedPredicate: Predicate;
  onClose: () => void;
}) {
  const [predicate, setPredicate] = useState<Predicate>(seedPredicate);
  const qc = useQueryClient();
  const mut = useMutation({
    mutationFn: (name: string) =>
      createCollection({ name, scope, predicate }),
    onSuccess: (c: Collection) => {
      qc.invalidateQueries({ queryKey: ['collections'] });
      onClose();
      void c;
    },
  });
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header className="modal-header">
          <h3>Save current filter as a Collection</h3>
          <button className="btn btn-sm" onClick={onClose}>×</button>
        </header>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            const fd = new FormData(e.currentTarget);
            const name = (fd.get('name') as string).trim();
            if (name) mut.mutate(name);
          }}
        >
          <div className="modal-body">
            <div className="form-group">
              <label htmlFor="name">Name</label>
              <input id="name" name="name" required autoFocus />
            </div>
            <div className="form-group">
              <label>Scope</label>
              <div className="muted">{scope}</div>
            </div>
            <div className="form-group">
              <label>Predicate</label>
              <PredicateBuilder value={predicate} onChange={setPredicate} />
            </div>
            {mut.error && <p className="error">{(mut.error as Error).message}</p>}
          </div>
          <footer className="modal-footer">
            <button type="button" className="btn" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={mut.isPending}>
              {mut.isPending ? 'Saving…' : 'Save'}
            </button>
          </footer>
        </form>
      </div>
    </div>
  );
}
