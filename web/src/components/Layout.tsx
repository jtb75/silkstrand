import { NavLink, Outlet } from 'react-router-dom';
import { hasToken, clearToken } from '../api/client';
import './Layout.css';

export default function Layout() {
  const authenticated = hasToken();

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
        </nav>
      </aside>
      <div className="main-area">
        <header className="topbar">
          <span>CIS Compliance Scanner</span>
          <span className="topbar-token-status">
            {authenticated ? (
              <>
                Token set
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
        </header>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
