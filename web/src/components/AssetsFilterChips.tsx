export type ChipId =
  | 'with_cves'
  | 'compliance_candidates'
  | 'failing'
  | 'recently_changed'
  | 'new_this_week'
  | 'manual'
  | 'discovered';

const CHIP_LABELS: Record<ChipId, string> = {
  with_cves: 'With CVEs',
  compliance_candidates: 'Compliance candidates',
  failing: 'Failing compliance',
  recently_changed: 'Recently changed',
  new_this_week: 'New this week',
  manual: 'Manual',
  discovered: 'Discovered',
};

const CHIP_ORDER: ChipId[] = [
  'with_cves',
  'compliance_candidates',
  'failing',
  'recently_changed',
  'new_this_week',
  'manual',
  'discovered',
];

interface Props {
  active: Set<ChipId>;
  total: number;
  onToggle: (id: ChipId) => void;
  onClear: () => void;
  scanRunning?: boolean;
}

export default function AssetsFilterChips({ active, total, onToggle, onClear, scanRunning }: Props) {
  return (
    <div className="filter-chips">
      <div className="chip-row">
        {CHIP_ORDER.map((id) => (
          <button
            key={id}
            type="button"
            className={`chip ${active.has(id) ? 'chip-active' : ''}`}
            onClick={() => onToggle(id)}
          >
            {CHIP_LABELS[id]}
          </button>
        ))}
        {active.size > 0 && (
          <button type="button" className="chip chip-clear" onClick={onClear}>
            Clear
          </button>
        )}
      </div>
      <div className="chip-meta">
        <span>{total} {total === 1 ? 'asset' : 'assets'}</span>
        {scanRunning && <span className="muted" style={{ marginLeft: 12 }}>scan running…</span>}
      </div>
    </div>
  );
}
