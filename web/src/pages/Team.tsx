import { OrganizationProfile, useOrganization } from '@clerk/clerk-react';

/**
 * Team management page.
 * Uses Clerk's drop-in <OrganizationProfile /> for member management.
 * Only visible to tenant admins (hide logic in Layout nav).
 */
export default function Team() {
  const { organization, isLoaded } = useOrganization();

  if (!isLoaded) {
    return <div>Loading…</div>;
  }

  if (!organization) {
    return (
      <div>
        <h1>Team</h1>
        <p>
          Your account isn't linked to an organization yet. Contact your SilkStrand
          administrator.
        </p>
      </div>
    );
  }

  return (
    <div>
      <h1>Team Settings</h1>
      <p className="muted">Invite users, manage roles, and configure your organization.</p>
      <div style={{ marginTop: '1.5rem' }}>
        <OrganizationProfile
          appearance={{
            elements: {
              rootBox: { width: '100%' },
              card: { width: '100%', boxShadow: 'none', border: '1px solid #e5e7eb' },
            },
          }}
        />
      </div>
    </div>
  );
}
