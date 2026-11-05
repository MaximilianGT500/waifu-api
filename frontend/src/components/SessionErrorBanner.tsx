import { TriangleAlert, Loader2, RefreshCw } from 'lucide-react';
import { useAuth } from '../context/AuthContext';

const SessionErrorBanner = () => {
    const { sessionError, sessionRetrying, retrySession, logout } = useAuth();
    if (!sessionError) return null;

    const handleLogout = () => {
        logout();
        window.location.assign('/');
    };

    return (
        <div
            role="alert"
            className="mx-3 mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-[var(--radius-panel)] bg-warning/12 px-4 py-3 text-sm font-bold sm:mx-5 sm:mt-5 lg:mx-8 lg:mt-7"
        >
            <TriangleAlert size={18} className="shrink-0 text-warning" aria-hidden="true" />
            <p className="min-w-48 flex-1 text-foreground">
                Your account could not be loaded. The server may be temporarily unavailable; you are still logged in.
            </p>
            <div className="flex items-center gap-2">
                <button
                    type="button"
                    onClick={retrySession}
                    disabled={sessionRetrying}
                    className="btn btn-secondary btn-sm"
                >
                    {sessionRetrying ? (
                        <Loader2 size={14} className="animate-spin" aria-hidden="true" />
                    ) : (
                        <RefreshCw size={14} aria-hidden="true" />
                    )}
                    {sessionRetrying ? 'Retrying…' : 'Retry'}
                </button>
                <button type="button" onClick={handleLogout} className="btn btn-ghost btn-sm">
                    Log out
                </button>
            </div>
        </div>
    );
};

export default SessionErrorBanner;
