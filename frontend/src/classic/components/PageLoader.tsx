import { Loader2 } from 'lucide-react';

const PageLoader = () => (
    <div className="flex min-h-[40vh] items-center justify-center" role="status" aria-live="polite">
        <Loader2 className="size-6 animate-spin text-muted-foreground" aria-hidden="true" />
        <span className="sr-only">Loading…</span>
    </div>
);

export default PageLoader;
