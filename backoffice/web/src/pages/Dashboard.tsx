import { useQuery } from '@tanstack/react-query';
import { getDashboard } from '../api/client';
import type { DashboardData } from '../api/types';
import DataCenterCard from '../components/DataCenterCard';

export default function Dashboard() {
  const { data, isLoading, error } = useQuery<DashboardData>({
    queryKey: ['dashboard'],
    queryFn: getDashboard,
  });

  return (
    <div>
      <h1>Dashboard</h1>

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load dashboard: {(error as Error).message}</p>}

      {data && (
        <>
          <div className="stats-grid">
            <div className="stat-card">
              <div className="stat-value">{data.total_data_centers}</div>
              <div className="stat-label">Data Centers</div>
            </div>
            <div className="stat-card">
              <div className="stat-value">{data.total_tenants}</div>
              <div className="stat-label">Total Tenants</div>
            </div>
            <div className="stat-card">
              <div className="stat-value">{data.active_tenants}</div>
              <div className="stat-label">Active Tenants</div>
            </div>
            <div className="stat-card">
              <div className="stat-value">{data.suspended_tenants}</div>
              <div className="stat-label">Suspended Tenants</div>
            </div>
          </div>

          <h2>Data Center Health</h2>
          {data.data_centers.length === 0 && <p>No data centers registered.</p>}
          <div className="dc-cards-grid">
            {data.data_centers.map((dc) => (
              <DataCenterCard key={dc.id} dc={dc} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
