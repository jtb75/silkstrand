import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import { clearToken } from '../api/client';
import './Layout.css';

export default function Layout() {
  const navigate = useNavigate();

  function handleLogout() {
    clearToken();
    navigate('/login');
  }

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="sidebar-brand">SilkStrand Backoffice</div>
        <nav className="sidebar-nav">
          <NavLink to="/" end>
            Dashboard
          </NavLink>
          <NavLink to="/data-centers">Data Centers</NavLink>
          <NavLink to="/tenants">Tenants</NavLink>
          <NavLink to="/users">Users</NavLink>
        </nav>
        <div className="sidebar-footer">
          <button className="logout-btn" onClick={handleLogout}>
            Logout
          </button>
        </div>
      </aside>
      <div className="main-area">
        <header className="topbar">
          <span>Backoffice Manager</span>
        </header>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
