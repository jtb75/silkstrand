import { useEffect, useState, useCallback, type ReactNode } from 'react';
import { authApi, type MeResponse } from '../api/authClient';
import { getToken, setToken, clearToken } from '../api/client';
import { AuthContext, type AuthState } from './context';

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({
    user: null,
    memberships: [],
    active: null,
    loading: true,
    error: null,
  });

  const applyMe = useCallback((me: MeResponse) => {
    setState({
      user: me.user,
      memberships: me.memberships,
      active: me.active,
      loading: false,
      error: null,
    });
  }, []);

  const refresh = useCallback(async () => {
    const tok = getToken();
    if (!tok) {
      setState((s) => ({ ...s, loading: false }));
      return;
    }
    try {
      const me = await authApi.me(tok);
      applyMe(me);
    } catch (e) {
      clearToken();
      setState({ user: null, memberships: [], active: null, loading: false, error: (e as Error).message });
    }
  }, [applyMe]);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { void refresh(); }, [refresh]);

  const login = useCallback(async (email: string, password: string) => {
    const res = await authApi.login(email, password);
    setToken(res.token);
    await refresh();
  }, [refresh]);

  const acceptInvite = useCallback(async (token: string, password: string) => {
    const res = await authApi.acceptInvite(token, password);
    setToken(res.token);
    await refresh();
  }, [refresh]);

  const logout = useCallback(() => {
    clearToken();
    setState({ user: null, memberships: [], active: null, loading: false, error: null });
  }, []);

  const switchOrg = useCallback(async (tenantId: string) => {
    const tok = getToken();
    if (!tok) throw new Error('not authenticated');
    const res = await authApi.switchOrg(tok, tenantId);
    setToken(res.token);
    await refresh();
  }, [refresh]);

  return (
    <AuthContext.Provider value={{ ...state, login, acceptInvite, logout, switchOrg, refresh }}>
      {children}
    </AuthContext.Provider>
  );
}
