import './DashboardWidgets.css';

interface KpiCardProps {
  label: string;
  value: number | string;
  delta?: string;
  deltaTone?: 'positive' | 'negative' | 'neutral';
}

export function KpiCard({ label, value, delta, deltaTone = 'neutral' }: KpiCardProps) {
  return (
    <div className="dash-card kpi-card">
      <h3>{label}</h3>
      <div className="kpi-value">{value}</div>
      {delta && (
        <div className={`kpi-delta ${deltaTone === 'neutral' ? '' : deltaTone}`}>
          {delta}
        </div>
      )}
    </div>
  );
}
