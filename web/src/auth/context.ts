import { createContext } from 'react';
import type { User, Membership, ActiveTenant } from '../api/types';

export interface AuthState {
  user: User | null;
  memberships: Membership[];
  active: ActiveTenant | null;
  loading: boolean;
  error: string | null;
}

export interface AuthContextValue extends AuthState {
  login: (email: string, password: string) => Promise<void>;
  acceptInvite: (token: string, password: string) => Promise<void>;
  logout: () => void;
  switchOrg: (tenantId: string) => Promise<void>;
  refresh: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);
