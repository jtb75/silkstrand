import { useParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getTenant } from '../api/client';
import type { Tenant } from '../api/types';
import StatusBadge from '../components/StatusBadge';

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>();

  const { data: tenant, isLoading, error } = useQuery<Tenant>({
    queryKey: ['tenants', id],
    queryFn: () => getTenant(id!),
    enabled: !!id,
  });

  return (
    <div>
      <Link to="/tenants" className="back-link">Back to Tenants</Link>

      {isLoading && <p>Loading...</p>}
      {error && <p className="error">Failed to load tenant: {(error as Error).message}</p>}

      {tenant && (
        <>
          <div className="page-header">
            <h1>{tenant.name}</h1>
            <StatusBadge status={tenant.status} />
          </div>

          <div className="detail-card">
            <div className="detail-row">
              <span className="detail-label">Status</span>
              <StatusBadge status={tenant.status} />
            </div>
            <div className="detail-row">
              <span className="detail-label">Provisioning</span>
              <StatusBadge status={tenant.provisioning_status} />
            </div>
            <div className="detail-row">
              <span className="detail-label">DC Tenant ID</span>
              <span>{tenant.dc_tenant_id || '-'}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Data Center ID</span>
              <span className="text-muted">{tenant.data_center_id}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Created</span>
              <span>{new Date(tenant.created_at).toLocaleString()}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Updated</span>
              <span>{new Date(tenant.updated_at).toLocaleString()}</span>
            </div>
          </div>

          {tenant.config && Object.keys(tenant.config).length > 0 && (
            <>
              <h2>Configuration</h2>
              <pre className="config-json">{JSON.stringify(tenant.config, null, 2)}</pre>
            </>
          )}
        </>
      )}
    </div>
  );
}
