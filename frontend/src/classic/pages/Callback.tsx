import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router';
import { isAxiosError } from 'axios';
import { Loader2 } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import api from '../../services/api';
import { consumeOAuthState, getStoredToken, takeRedirect } from '../../services/tokenStorage';
import { safeRedirectPath } from '../../utils/safeUrl';
import { ProblemDetails } from '../../types';

function loginErrorMessage(error: unknown): string {
  if (isAxiosError<ProblemDetails>(error)) {
    const status = error.response?.status;
    const detail = error.response?.data?.detail;
    if (status === 403) return detail || 'Your account has been blacklisted.';
    if (status === 429) return 'Too many login attempts. Please wait a minute and try again.';
    if (status === 502 || status === 503 || status === 504) {
      return 'Discord could not be reached. Please try again in a moment.';
    }
    if (detail) return detail;
    if (!error.response) return 'The server is unreachable. Please check your connection and try again.';
  }
  return 'The login could not be completed. Please try again.';
}

const Callback = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { login } = useAuth();
  const [error, setError] = useState<string | null>(null);

  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;

    const code = searchParams.get('code');
    const state = searchParams.get('state');
    const oauthError = searchParams.get('error');

    window.history.replaceState(window.history.state, '', window.location.pathname);

    const stateValid = consumeOAuthState(state);

    if (oauthError) {
      setError(oauthError === 'access_denied' ? 'The login was cancelled.' : 'Discord rejected the login request.');
      return;
    }
    if (!code) {
      navigate('/login', { replace: true });
      return;
    }
    if (!stateValid) {
      setError('The login request could not be verified or has expired. Please start the login again.');
      return;
    }

    api
      .post<{ token: string }>('/auth/discord', { code }, { skipGlobalErrorHandler: true, skipAuth: true, timeout: 30_000 })
      .then((response) => {
        const token = response.data?.token;
        if (typeof token !== 'string' || !token) throw new Error('No token received');
        login(token);
        if (getStoredToken() !== token) {
          setError('Your browser blocked storing the login. Please allow site data (local storage) for this site and try again.');
          return;
        }
        navigate(safeRedirectPath(takeRedirect()), { replace: true });
      })
      .catch((err) => {
        setError(loginErrorMessage(err));
      });
  }, [searchParams, login, navigate]);

  if (error) {
    return (
      <div className="flex min-h-[calc(100dvh-4rem)] items-center justify-center px-4 py-12">
        <div className="w-full max-w-sm">
          <h1 className="text-3xl font-extrabold tracking-[-0.02em]">Login failed</h1>
          <p role="alert" className="mt-2 text-muted-foreground wrap-break-word">{error}</p>
          <Link to="/login" replace className="btn btn-primary btn-lg mt-8 w-full">
            Try again
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-[calc(100dvh-4rem)] items-center justify-center gap-3" role="status" aria-live="polite">
      <Loader2 className="size-5 animate-spin text-muted-foreground" aria-hidden="true" />
      <p className="text-lg font-medium">Logging in…</p>
    </div>
  );
};

export default Callback;
