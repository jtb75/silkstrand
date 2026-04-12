import { useNavigate } from 'react-router-dom';
import type { DataCenter } from '../api/types';
import StatusBadge from './StatusBadge';

interface DataCenterCardProps {
  dc: DataCenter;
}

export default function DataCenterCard({ dc }: DataCenterCardProps) {
  const navigate = useNavigate();

  return (
    <div className="dc-card" onClick={() => navigate(`/data-centers/${dc.id}`)}>
      <div className="dc-card-header">
        <span className="dc-card-name">
          {dc.name} <span className={`env-badge env-${dc.environment}`}>{dc.environment}</span>
        </span>
        <StatusBadge status={dc.last_health_status || dc.status} />
      </div>
      <div className="dc-card-meta">
        <div>Region: {dc.region}</div>
        <div>Tenants: {dc.tenant_count}</div>
      </div>
    </div>
  );
}
