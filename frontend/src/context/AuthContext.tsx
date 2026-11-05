import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import { ProblemDetails, Role, User } from '../types';
import { useUser } from '../hooks/useUser';
import { authExpiredBus, globalErrorBus } from '../services/eventBus';
import { showApiError } from '../services/api';
import { clearAuthStorage, decodeJwtPayload, getStoredToken, storeToken } from '../services/tokenStorage';

const ROLE_CLAIM = 'http://schemas.microsoft.com/ws/2008/06/identity/claims/role';

function isBanResponse(status: number | undefined, data: unknown): boolean {
  if (status !== 403 || !data || typeof data !== 'object') return false;
  const problem = data as Partial<ProblemDetails>;
  return problem.status === 403 && typeof problem.title === 'string';
}

function getRoleFromToken(token: string): Role | null {
  const roleClaim = decodeJwtPayload(token)?.[ROLE_CLAIM];
  if (typeof roleClaim === 'string' && Object.values(Role).includes(roleClaim as Role)) {
    return roleClaim as Role;
  }
  return null;
}

interface AuthContextType {
  token: string | null;
  user: User | null;
  login: (token: string) => void;
  logout: () => void;
  isAuthenticated: boolean;
  isLoading: boolean;
  roleOutOfSync: boolean;
  sessionError: boolean;
  sessionEndedAt: number | null;
  sessionRetrying: boolean;
  retrySession: () => void;
}

const AuthContext = createContext<AuthContextType | null>(null);

export const AuthProvider = ({ children }: { children: React.ReactNode }) => {
  const [token, setToken] = useState<string | null>(() => getStoredToken());
  const tokenRef = useRef(token);
  const queryClient = useQueryClient();

  const { data: user, isLoading: userLoading, error: userError, refetch: refetchUser, isFetching: userFetching } = useUser(!!token);

  const isLoading = !!token && userLoading;

  const userErrorStatus = isAxiosError(userError) ? userError.response?.status : undefined;
  const banned = isAxiosError(userError) && isBanResponse(userError.response?.status, userError.response?.data);
  const sessionError = !!token && !user && !!userError && userErrorStatus !== 401 && !banned;
  const [sessionEndedAt, setSessionEndedAt] = useState<number | null>(null);
  const retrySession = useCallback(() => {
    void refetchUser();
  }, [refetchUser]);

  const roleOutOfSync = useMemo(() => {
    if (!token || !user) return false;
    const tokenRole = getRoleFromToken(token);
    return tokenRole !== null && tokenRole !== user.role;
  }, [token, user]);

  const applyToken = useCallback(
    (next: string | null) => {
      if (tokenRef.current === next) return;
      tokenRef.current = next;
      queryClient.clear();
      setToken(next);
    },
    [queryClient]
  );

  const login = useCallback(
    (newToken: string) => {
      storeToken(newToken);
      applyToken(getStoredToken());
    },
    [applyToken]
  );

  const logout = useCallback(() => {
    clearAuthStorage();
    if (tokenRef.current) setSessionEndedAt(Date.now());
    applyToken(null);
  }, [applyToken]);

  const expireSession = useCallback(() => {
    if (!tokenRef.current) return;
    logout();
    globalErrorBus.emit({ type: 'warning', message: 'Your session has expired. Please log in again.' });
  }, [logout]);

  useEffect(() => authExpiredBus.subscribe(() => expireSession()), [expireSession]);

  useEffect(() => {
    if (!userError || !isAxiosError<ProblemDetails>(userError)) return;
    const status = userError.response?.status;
    if (status === 401) expireSession();
    else if (isBanResponse(status, userError.response?.data)) {
      if (tokenRef.current) showApiError(userError);
      logout();
    }
  }, [userError, expireSession, logout]);

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key !== null && event.key !== 'token') return;
      applyToken(getStoredToken());
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, [applyToken]);

  const value = useMemo<AuthContextType>(
    () => ({
      token,
      user: user || null,
      login,
      logout,
      isAuthenticated: !!token && !!user,
      isLoading,
      roleOutOfSync,
      sessionError,
      sessionRetrying: sessionError && userFetching,
      retrySession,
      sessionEndedAt,
    }),
    [token, user, login, logout, isLoading, roleOutOfSync, sessionError, userFetching, retrySession, sessionEndedAt]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
