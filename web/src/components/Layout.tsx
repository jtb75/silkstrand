import { NavLink, Outlet } from 'react-router-dom';
import { useOrganization, OrganizationSwitcher } from '@clerk/clerk-react';
import { hasDevToken, clearToken } from '../api/client';
import './Layout.css';

const isClerkMode = !!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;

export default function Layout() {
  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="sidebar-brand">SilkStrand</div>
        <nav className="sidebar-nav">
          <NavLink to="/" end>
            Dashboard
          </NavLink>
          <NavLink to="/targets">Targets</NavLink>
          <NavLink to="/scans">Scans</NavLink>
          {isClerkMode && <AdminOnlyTeamLink />}
        </nav>
      </aside>
      <div className="main-area">
        <header className="topbar">
          <span>CIS Compliance Scanner</span>
          {isClerkMode && (
            <span style={{ marginLeft: 'auto' }}>
              <OrganizationSwitcher
                hidePersonal
                appearance={{
                  elements: {
                    rootBox: { display: 'inline-flex' },
                    organizationSwitcherTrigger: { padding: '4px 8px' },
                  },
                }}
              />
            </span>
          )}
          {!isClerkMode && (
            <span className="topbar-token-status">
              {hasDevToken() ? (
                <>
                  Dev token set
                  <button
                    onClick={() => {
                      clearToken();
                      window.location.reload();
                    }}
                  >
                    Clear
                  </button>
                </>
              ) : (
                'No auth token'
              )}
            </span>
          )}
        </header>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

// AdminOnlyTeamLink renders the Team nav link only for users with the admin
// role in their current organization. Uses Clerk's useOrganization hook.
function AdminOnlyTeamLink() {
  const { membership, isLoaded } = useOrganization();
  if (!isLoaded) return null;
  if (membership?.role !== 'admin' && membership?.role !== 'org:admin') {
    return null;
  }
  return <NavLink to="/team">Team</NavLink>;
}
