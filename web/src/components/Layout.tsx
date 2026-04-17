import { useEffect } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/useAuth';
import { useEventStream } from '../lib/events';
import TenantSwitcher from './TenantSwitcher';
import './Layout.css';

export default function Layout() {
  const { user, active, logout } = useAuth();
  const isAdmin = active?.role === 'admin';
  const queryClient = useQueryClient();

  // SSE: subscribe to scan_status + agent_status events for real-time cache
  // invalidation. When any scan or agent changes state, invalidate the
  // relevant queries so list views update without manual refresh.
  const { events: scanEvents } = useEventStream<{ status: string }>(
    { kinds: ['scan_status'] },
    { enabled: !!active },
  );
  const { events: agentEvents } = useEventStream<{ status: string }>(
    { kinds: ['agent_status'] },
    { enabled: !!active },
  );

  useEffect(() => {
    if (scanEvents.length > 0) {
      queryClient.invalidateQueries({ queryKey: ['scans'] });
      queryClient.invalidateQueries({ queryKey: ['scan-definitions'] });
    }
  }, [scanEvents.length, queryClient]);

  useEffect(() => {
    if (agentEvents.length > 0) {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    }
  }, [agentEvents.length, queryClient]);

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="sidebar-brand">SilkStrand</div>
        <nav className="sidebar-nav">
          <NavLink to="/" end>Dashboard</NavLink>
          <NavLink to="/assets">Assets</NavLink>
          <NavLink to="/findings">Findings</NavLink>
          <NavLink to="/agents">Agents</NavLink>
          <NavLink to="/scans">Scans</NavLink>
          {isAdmin && <NavLink to="/collections">Collections</NavLink>}
          {isAdmin && <NavLink to="/rules">Rules</NavLink>}
          {/* One-shot scans are manual-only scan_definitions post-refactor;
              they live under Scans → Definitions. Route kept in App.tsx so
              old deep-links don't 404, but no top-level nav entry. */}
          <NavLink to="/settings">Settings</NavLink>
        </nav>
      </aside>
      <div className="main-area">
        <header className="topbar">
          <span>CIS Compliance Scanner</span>
          <span className="topbar-right">
            <TenantSwitcher />
            {user && (
              <>
                <span className="muted" style={{ marginLeft: 12 }}>
                  {user.display_name ? `${user.display_name} (${user.email})` : user.email}
                </span>
                <button className="btn btn-sm" style={{ marginLeft: 8 }} onClick={logout}>
                  Log out
                </button>
              </>
            )}
          </span>
        </header>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
