import { useEffect, useState } from 'react';
import { Navigate, useLocation } from 'react-router';
import { getEnv } from '../utils/env';
import { safeRedirectPath } from '../utils/safeUrl';
import { createOAuthState, storeRedirect } from '../services/tokenStorage';
import { useAuth } from '../context/AuthContext';
import { MetaTags } from '../hooks/useMetaTags';

type FromLocation = { pathname?: string; search?: string; hash?: string } | undefined;

function redirectTarget(from: FromLocation): string {
    if (!from?.pathname) return '/';
    return safeRedirectPath(`${from.pathname}${from.search ?? ''}${from.hash ?? ''}`);
}

const Login = () => {
    const location = useLocation();
    const { isAuthenticated } = useAuth();
    const [error, setError] = useState<string | null>(null);
    const [redirecting, setRedirecting] = useState(false);
    const from = (location.state as { from?: FromLocation } | null)?.from;

    useEffect(() => {
        const onPageShow = (event: PageTransitionEvent) => {
            if (event.persisted) setRedirecting(false);
        };
        window.addEventListener('pageshow', onPageShow);
        return () => window.removeEventListener('pageshow', onPageShow);
    }, []);

    if (isAuthenticated) {
        return <Navigate to={redirectTarget(from)} replace />;
    }

    const handleLogin = () => {
        const clientId = getEnv('VITE_DISCORD_CLIENT_ID')?.trim();
        const redirectUri = getEnv('VITE_DISCORD_REDIRECT_URI')?.trim();

        if (!clientId || !/^\d{5,25}$/.test(clientId) || !redirectUri) {
            setError('Login is not configured. Please contact the administrator.');
            return;
        }

        const state = createOAuthState();
        if (!state) {
            setError('Your browser blocks site data (local storage) for this site. Logging in requires it – please allow it and try again.');
            return;
        }
        storeRedirect(redirectTarget(from));

        const params = new URLSearchParams({
            client_id: clientId,
            redirect_uri: redirectUri,
            response_type: 'code',
            scope: 'identify',
            state,
        });
        setRedirecting(true);
        window.location.assign(`https://discord.com/api/oauth2/authorize?${params.toString()}`);
    };

    const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';

    return (
        <div className="flex min-h-[calc(100dvh-14rem)] items-center justify-center px-4 py-12">
            <MetaTags title="Login" description="Sign in with Discord to upload images, manage albums and create API keys." />
            <div className="panel w-full max-w-sm p-7 sm:p-8">
                <img
                    src="/logo-96.png?v=20261024-logo-v2"
                    srcSet="/logo-96.png?v=20261024-logo-v2 2x, /logo-144.png?v=20261024-logo-v2 3x"
                    alt=""
                    width={56}
                    height={56}
                    className="mb-6 size-14 -rotate-6 rounded-2xl bg-card object-cover p-0.5 shadow-card"
                />
                <h1 className="text-3xl font-black">Log in to {appTitle}</h1>
                <p className="mt-2 text-muted-foreground">Upload images, keep albums and create API keys. Browsing needs no account.</p>

                {error && (
                    <p role="alert" className="mt-6 rounded-2xl bg-destructive/10 px-4 py-3 text-sm font-bold text-destructive">
                        {error}
                    </p>
                )}

                <button
                    type="button"
                    onClick={handleLogin}
                    disabled={redirecting}
                    aria-busy={redirecting}
                    className="btn btn-lg mt-8 w-full bg-[#5865F2] text-white hover:bg-[#4752C4] disabled:cursor-wait disabled:opacity-70"
                >
                    <svg aria-hidden="true" className="size-5" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                        <path d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.419-2.1568 2.419zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.419-2.1568 2.419z"/>
                    </svg>
                    {redirecting ? 'Redirecting to Discord…' : 'Continue with Discord'}
                </button>
                <p className="mt-4 text-xs leading-relaxed text-muted-foreground">
                    Discord only shares your basic profile with us: your user ID, name and avatar.
                </p>
            </div>
        </div>
    );
};

export default Login;
