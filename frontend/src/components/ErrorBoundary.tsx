import { Component, ErrorInfo, ReactNode } from 'react';
import { TriangleAlert, RefreshCw } from 'lucide-react';
import { isChunkLoadError, reloadOnceForNewBuild } from '../utils/chunkReload';

interface ErrorBoundaryProps {
    children: ReactNode;
    resetKey?: string;
}

interface ErrorBoundaryState {
    error: Error | null;
    resetKey?: string;
}

class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
    state: ErrorBoundaryState = { error: null, resetKey: this.props.resetKey };

    static getDerivedStateFromError(error: Error): Partial<ErrorBoundaryState> {
        return { error };
    }

    static getDerivedStateFromProps(props: ErrorBoundaryProps, state: ErrorBoundaryState): Partial<ErrorBoundaryState> | null {
        if (props.resetKey !== state.resetKey) {
            return { error: null, resetKey: props.resetKey };
        }
        return null;
    }

    componentDidCatch(error: Error, info: ErrorInfo) {
        if (isChunkLoadError(error) && reloadOnceForNewBuild()) return;
        if (import.meta.env.DEV) {
            console.error('Unhandled UI error', error, info.componentStack);
        }
    }

    render() {
        const { error } = this.state;
        if (!error) return this.props.children;

        const outdated = isChunkLoadError(error);
        return (
            <div role="alert" className="flex min-h-[50vh] flex-col items-center justify-center gap-4 p-8 text-center">
                <span className="grid size-14 place-items-center rounded-full bg-destructive/12 text-destructive" aria-hidden="true">
                    <TriangleAlert size={26} strokeWidth={1.8} />
                </span>
                <h1 className="text-2xl font-black">
                    {outdated ? 'A new version is available' : 'Something went wrong'}
                </h1>
                <p className="max-w-md text-muted-foreground">
                    {outdated
                        ? 'This page could not be loaded because the application was updated. Reload to get the latest version.'
                        : 'This page ran into an unexpected problem. Reloading usually fixes it.'}
                </p>
                <div className="flex flex-wrap justify-center gap-3">
                    <button type="button" onClick={() => window.location.reload()} className="btn btn-primary">
                        <RefreshCw size={16} aria-hidden="true" /> Reload
                    </button>
                    <a href="/" className="btn btn-secondary">
                        Go to the home page
                    </a>
                </div>
            </div>
        );
    }
}

export default ErrorBoundary;
