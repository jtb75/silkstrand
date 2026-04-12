import { useAuth } from '../auth/useAuth';

/**
 * A compact dropdown in the topbar that lets a user switch between the
 * tenants they belong to. Does nothing (renders nothing) if the user is
 * in exactly one tenant.
 */
export default function TenantSwitcher() {
  const { memberships, active, switchOrg } = useAuth();
  if (!active || memberships.length <= 1) return null;

  return (
    <select
      value={active.tenant_id}
      onChange={(e) => { void switchOrg(e.target.value); }}
      style={{ marginLeft: 12 }}
      aria-label="Active tenant"
    >
      {memberships.map((m) => (
        <option key={m.tenant_id} value={m.tenant_id}>
          {m.tenant_name} ({m.role})
        </option>
      ))}
    </select>
  );
}
