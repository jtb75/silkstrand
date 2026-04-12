import { Navigate } from 'react-router-dom';
import { useAuth } from '../auth/useAuth';

export default function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="auth-card"><p>Loading…</p></div>;
  if (!user) return <Navigate to="/login" replace />;
  return <>{children}</>;
}
