import { useState } from 'react';
import ScanDefinitions from './ScanDefinitions';
import ScanActivity from './ScanActivity';
import Targets from './Targets';

// P3 UI shape (docs/plans/ui-shape.md § Scans): the Scans top-level nav
// item exposes three tabs — Definitions (authoring surface),
// Activity (feed of runs), and Targets (port of the legacy Targets page,
// which no longer has its own nav entry after this phase).
type Tab = 'definitions' | 'activity' | 'targets';

const TABS: { value: Tab; label: string }[] = [
  { value: 'definitions', label: 'Definitions' },
  { value: 'activity', label: 'Activity' },
  { value: 'targets', label: 'Targets' },
];

export default function Scans() {
  const [tab, setTab] = useState<Tab>('definitions');

  return (
    <div>
      <div className="page-header">
        <h1>Scans</h1>
      </div>
      <div className="tabbar" style={{ display: 'flex', gap: 16, borderBottom: '1px solid #e5e7eb', marginBottom: 16 }}>
        {TABS.map((t) => (
          <button
            key={t.value}
            className={`tab ${tab === t.value ? 'tab-active' : ''}`}
            onClick={() => setTab(t.value)}
            style={{
              background: 'none',
              border: 'none',
              padding: '8px 4px',
              cursor: 'pointer',
              fontWeight: tab === t.value ? 600 : 400,
              borderBottom: tab === t.value ? '2px solid #2563eb' : '2px solid transparent',
              marginBottom: -1,
            }}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'definitions' && <ScanDefinitions />}
      {tab === 'activity' && <ScanActivity />}
      {tab === 'targets' && <Targets />}
    </div>
  );
}
