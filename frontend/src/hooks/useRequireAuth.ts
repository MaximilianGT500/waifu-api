import { useEffect, useCallback } from 'react';
import { useNavigate, useLocation } from 'react-router';
import { useAuth } from '../context/AuthContext';
import { useNotification } from '../context/NotificationContext';

export const useRequireAuth = (redirectUrl = '/login', message = 'You must be logged in to access this page.') => {
  const { user, token, isLoading, sessionEndedAt } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const { showNotification } = useNotification();

  useEffect(() => {
    if (!isLoading && !user && !token) {
      const justEnded = sessionEndedAt !== null && Date.now() - sessionEndedAt < 10_000;
      if (!justEnded) showNotification('warning', message);
      navigate(redirectUrl, { state: { from: location }, replace: true });
    }
  }, [user, token, isLoading, sessionEndedAt, navigate, location, showNotification, redirectUrl, message]);

  return user;
};

export const useAuthGuard = () => {
  const { user, token } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const { showNotification } = useNotification();

  const checkAuth = useCallback((message = 'You must be logged in to perform this action.', redirectBack?: string) => {
    if (!user && token) {
      showNotification('warning', 'Your account is not available right now. Please try again in a moment.');
      return false;
    }
    if (!user) {
      showNotification('warning', message);
      const from = redirectBack ? { pathname: redirectBack } : location;
      navigate('/login', { state: { from } });
      return false;
    }
    return true;
  }, [user, token, navigate, location, showNotification]);

  return checkAuth;
};
