import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../auth/useAuth';
import TenantSwitcher from './TenantSwitcher';
import './Layout.css';

export default function Layout() {
  const { user, active, logout } = useAuth();
  const isAdmin = active?.role === 'admin';

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="sidebar-brand">SilkStrand</div>
        <nav className="sidebar-nav">
          <NavLink to="/" end>Dashboard</NavLink>
          <NavLink to="/assets">Assets</NavLink>
          <NavLink to="/targets">Targets</NavLink>
          <NavLink to="/agents">Agents</NavLink>
          <NavLink to="/scans">Scans</NavLink>
          {isAdmin && <NavLink to="/team">Team</NavLink>}
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
