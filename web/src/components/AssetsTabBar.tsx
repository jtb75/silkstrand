type Tab = 'list' | 'topology';

interface Props {
  tab: Tab;
  onChange: (t: Tab) => void;
}

export default function AssetsTabBar({ tab, onChange }: Props) {
  return (
    <div className="tab-bar">
      <button
        type="button"
        className={`tab ${tab === 'list' ? 'tab-active' : ''}`}
        onClick={() => onChange('list')}
      >
        List
      </button>
      <button
        type="button"
        className={`tab ${tab === 'topology' ? 'tab-active' : ''}`}
        onClick={() => onChange('topology')}
      >
        Topology
      </button>
    </div>
  );
}
