import { useParams, Link, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getDataCenter, listTenants } from '../api/client';
import type { DataCenter, Tenant } from '../api/types';
import StatusBadge from '../components/StatusBadge';

export default function DataCenterDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const { data: dc, isLoading: dcLoading, error: dcError } = useQuery<DataCenter>({
    queryKey: ['data-centers', id],
    queryFn: () => getDataCenter(id!),
    enabled: !!id,
  });

  const { data: tenants, isLoading: tenantsLoading } = useQuery<Tenant[]>({
    queryKey: ['tenants', { data_center_id: id }],
    queryFn: () => listTenants(id),
    enabled: !!id,
  });

  return (
    <div>
      <Link to="/data-centers" className="back-link">Back to Data Centers</Link>

      {dcLoading && <p>Loading...</p>}
      {dcError && <p className="error">Failed to load data center: {(dcError as Error).message}</p>}

      {dc && (
        <>
          <div className="page-header">
            <h1>{dc.name}</h1>
            <StatusBadge status={dc.status} />
          </div>

          <div className="detail-card">
            <div className="detail-row">
              <span className="detail-label">Region</span>
              <span>{dc.region}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">API URL</span>
              <span>{dc.api_url}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Tenant Count</span>
              <span>{dc.tenant_count}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Created</span>
              <span>{new Date(dc.created_at).toLocaleString()}</span>
            </div>
          </div>

          <h2>Tenants in this Data Center</h2>
          {tenantsLoading && <p>Loading tenants...</p>}
          {!tenantsLoading && tenants && tenants.length === 0 && <p>No tenants in this data center.</p>}
          {tenants && tenants.length > 0 && (
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Provisioning</th>
                  <th>DC Tenant ID</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {tenants.map((t) => (
                  <tr
                    key={t.id}
                    className="clickable-row"
                    onClick={() => navigate(`/tenants/${t.id}`)}
                  >
                    <td>{t.name}</td>
                    <td><StatusBadge status={t.status} /></td>
                    <td><StatusBadge status={t.provisioning_status} /></td>
                    <td className="text-muted">{t.dc_tenant_id || '-'}</td>
                    <td>{new Date(t.created_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  );
}
