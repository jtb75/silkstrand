import { Suspense, lazy, useMemo, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { listAssets, listScans, type AssetFilterParams } from '../api/client';
import type { Scan } from '../api/types';
import AssetsTabBar from '../components/AssetsTabBar';
import AssetsFilterChips, { type ChipId } from '../components/AssetsFilterChips';
import AssetsTable from '../components/AssetsTable';
import AssetDetailDrawer from '../components/AssetDetailDrawer';

// Topology is the heavy import (xyflow + styles) — lazy-load so the
// list view + every other page in the app pay zero bundle cost.
const AssetsTopology = lazy(() => import('../components/AssetsTopology'));

// Filter chips → server-side query params. Keep one place to map chips
// so AssetsFilterChips stays presentation-only.
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

export default function Assets() {
  const navigate = useNavigate();
  const { id: selectedAssetId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = (searchParams.get('tab') === 'topology' ? 'topology' : 'list') as 'list' | 'topology';
  const [chips, setChips] = useState<Set<ChipId>>(new Set());

  const filters = useMemo(() => ({ ...chipsToParams(chips), page: 1, page_size: 200 }), [chips]);

  const queryClient = useQueryClient();
  const { data: assets, isLoading, error } = useQuery({
    queryKey: ['assets', filters],
    queryFn: () => listAssets(filters),
    refetchInterval: () => {
      const scans = queryClient.getQueryData<Scan[]>(['scans']);
      const running = scans?.some(
        (s) => s.scan_type === 'discovery' && (s.status === 'running' || s.status === 'pending')
      );
      return running ? 5000 : false;
    },
    refetchIntervalInBackground: false,
  });

  // Piggyback poll: keep ['scans'] fresh so the gate above stays accurate.
  useQuery({
    queryKey: ['scans'],
    queryFn: () => listScans(),
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
  });

  function toggleChip(id: ChipId) {
    setChips((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      // Manual & Discovered are mutually exclusive (same backend column).
      if (id === 'manual') next.delete('discovered');
      if (id === 'discovered') next.delete('manual');
      return next;
    });
  }

  function selectTab(t: 'list' | 'topology') {
    const next = new URLSearchParams(searchParams);
    if (t === 'topology') next.set('tab', 'topology');
    else next.delete('tab');
    setSearchParams(next, { replace: true });
  }

  function selectAsset(id: string) {
    navigate(`/assets/${id}${tab === 'topology' ? '?tab=topology' : ''}`);
  }

  function closeDrawer() {
    navigate(`/assets${tab === 'topology' ? '?tab=topology' : ''}`);
  }

  const items = assets?.items ?? [];
  const total = assets?.total ?? 0;
  const scanRunning = !!queryClient
    .getQueryData<Scan[]>(['scans'])
    ?.some((s) => s.scan_type === 'discovery' && (s.status === 'running' || s.status === 'pending'));

  return (
    <div>
      <div className="page-header">
        <h1>Assets</h1>
      </div>
      <AssetsTabBar tab={tab} onChange={selectTab} />
      <AssetsFilterChips
        active={chips}
        total={total}
        onToggle={toggleChip}
        onClear={() => setChips(new Set())}
        scanRunning={scanRunning}
      />

      {error && <p className="error">{(error as Error).message}</p>}
      {isLoading && <p>Loading…</p>}

      {!isLoading && !error && tab === 'list' && (
        items.length === 0
          ? <p className="muted">No assets yet. Create a target or trigger a discovery scan.</p>
          : <AssetsTable assets={items} onSelect={selectAsset} />
      )}

      {!isLoading && !error && tab === 'topology' && (
        <Suspense fallback={<p>Loading topology…</p>}>
          <AssetsTopology assets={items} grouping="cidr24" onSelect={selectAsset} />
        </Suspense>
      )}

      {selectedAssetId && (
        <AssetDetailDrawer assetId={selectedAssetId} onClose={closeDrawer} />
      )}
    </div>
  );
}
