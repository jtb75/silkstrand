import { useMemo } from 'react';
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import type { DiscoveredAsset } from '../api/types';

export type Grouping = 'cidr24' | 'env' | 'service';

interface Props {
  assets: DiscoveredAsset[];
  grouping: Grouping;
  onSelect: (id: string) => void;
  maxNodes?: number;
}

// Maximum nodes drawn before we collapse the largest group into a count
// badge. ADR plan §3 calls for a hard cap so the canvas stays usable
// at large inventories.
const DEFAULT_MAX_NODES = 300;

function groupKey(a: DiscoveredAsset, grouping: Grouping): string {
  if (grouping === 'env') return a.environment || '(no env)';
  if (grouping === 'service') return a.service || '(unknown)';
  // cidr24 — take the /24 of the IP.
  const m = a.ip.match(/^(\d+\.\d+\.\d+)\./);
  return m ? `${m[1]}.0/24` : a.ip;
}

export default function AssetsTopology({ assets, grouping, onSelect, maxNodes = DEFAULT_MAX_NODES }: Props) {
  const { nodes, edges } = useMemo(() => buildGraph(assets, grouping, maxNodes), [assets, grouping, maxNodes]);

  return (
    <div style={{ height: '70vh', border: '1px solid #ddd', borderRadius: 4 }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        fitView
        nodesDraggable={false}
        onNodeClick={(_, n) => {
          const id = (n.data as { assetId?: string }).assetId;
          if (id) onSelect(id);
        }}
      >
        <Background />
        <Controls />
        <MiniMap pannable zoomable />
      </ReactFlow>
    </div>
  );
}

function buildGraph(assets: DiscoveredAsset[], grouping: Grouping, maxNodes: number): { nodes: Node[]; edges: Edge[] } {
  const groups = new Map<string, DiscoveredAsset[]>();
  for (const a of assets) {
    const k = groupKey(a, grouping);
    const list = groups.get(k);
    if (list) list.push(a);
    else groups.set(k, [a]);
  }

  // Decide which groups expand inline vs. collapse to a badge.
  // Largest groups collapse first.
  const sorted = [...groups.entries()].sort((a, b) => b[1].length - a[1].length);
  let drawn = 0;
  const expanded = new Set<string>();
  for (const [k, list] of sorted) {
    // Reserve room for the group node itself + its assets.
    if (drawn + 1 + list.length > maxNodes) continue;
    expanded.add(k);
    drawn += 1 + list.length;
  }

  const nodes: Node[] = [];
  const edges: Edge[] = [];
  let groupY = 0;
  for (const [k, list] of sorted) {
    const groupID = `g-${k}`;
    nodes.push({
      id: groupID,
      type: 'default',
      data: { label: `${k}\n(${list.length})` },
      position: { x: 0, y: groupY },
      draggable: false,
      style: { background: '#1f2a4a', color: '#fff', borderRadius: 4 },
    });
    if (expanded.has(k)) {
      let assetX = 200;
      for (const a of list) {
        const id = `a-${a.id}`;
        const cveCount = Array.isArray(a.cves) ? a.cves.length : 0;
        const label = `${a.service ?? a.port}${cveCount ? ` !${cveCount}` : ''}`;
        nodes.push({
          id,
          type: 'default',
          data: { label, assetId: a.id },
          position: { x: assetX, y: groupY },
          style: { background: cveCount > 0 ? '#fde0e0' : '#fff', borderRadius: 4 },
        });
        edges.push({ id: `${groupID}-${id}`, source: groupID, target: id });
        assetX += 160;
      }
    } else {
      // Collapsed: indicate the group hides N assets.
      nodes.push({
        id: `${groupID}-hidden`,
        type: 'default',
        data: { label: `${list.length} hidden\n(zoom or filter)` },
        position: { x: 200, y: groupY },
        style: { background: '#f5f5f5', color: '#666', borderRadius: 4 },
      });
      edges.push({ id: `${groupID}-h`, source: groupID, target: `${groupID}-hidden` });
    }
    groupY += 120;
  }
  return { nodes, edges };
}
