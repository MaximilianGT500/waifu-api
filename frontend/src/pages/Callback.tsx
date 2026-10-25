import { useEffect, useRef } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import api from '../services/api';

const Callback = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { login } = useAuth();

  // Guards the code exchange against double execution (React StrictMode /
  // effect re-runs would otherwise send the single-use code twice and one
  // attempt would fail with an invalid code error).
  const exchanged = useRef(false);

  useEffect(() => {
    if (exchanged.current) return;

    const code = searchParams.get('code');
    if (!code) {
      navigate('/login');
      return;
    }

    // OAuth state validation (login-CSRF protection). Only enforced when a
    // state was issued by the login page - older links without state still work.
    const expectedState = sessionStorage.getItem('oauth_state');
    if (expectedState && searchParams.get('state') !== expectedState) {
      console.error('OAuth state mismatch - possible CSRF attempt.');
      sessionStorage.removeItem('oauth_state');
      navigate('/login');
      return;
    }
    sessionStorage.removeItem('oauth_state');

    exchanged.current = true;
    api.post('/auth/discord', { code })
      .then((response) => {
        login(response.data.token);

        // Redirect to original location or home
        const redirectPath = localStorage.getItem('auth_redirect') || '/';
        localStorage.removeItem('auth_redirect');
        navigate(redirectPath);
      })
      .catch((error) => {
        console.error('Login failed', error);
        exchanged.current = false;
        navigate('/login');
      });
  }, [searchParams, login, navigate]);

  return (
    <div className="flex justify-center items-center h-screen">
      <p className="text-xl">Logging in...</p>
    </div>
  );
};

export default Callback;
