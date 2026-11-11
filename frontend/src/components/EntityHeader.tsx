import { ReactNode } from 'react';
import { Link } from 'react-router';
import { ChevronLeft, Clock } from 'lucide-react';
import Skeleton from './Skeleton';

interface EntityHeaderProps {
    kind: string;
    backTo: string;
    backLabel: string;
    title?: string;
    pending?: boolean;
    children?: ReactNode;
}

const EntityHeader = ({ kind, backTo, backLabel, title, pending = false, children }: EntityHeaderProps) => (
    <div className="flex min-w-0 max-w-full items-start gap-3">
        <Link to={backTo} aria-label={backLabel} title={backLabel} className="mt-6 grid size-10 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground">
            <ChevronLeft size={22} aria-hidden="true" />
        </Link>
        <div className="min-w-0 flex-1">
            <p className="text-sm font-bold text-muted-foreground">{kind}</p>
            {title ? (
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    <h1 className="page-title min-w-0 wrap-break-word">{title}</h1>
                    {pending && (
                        <span className="inline-flex items-center gap-1 rounded-full bg-warning/12 px-2.5 py-0.5 text-xs font-bold text-warning">
                            <Clock size={12} aria-hidden="true" /> Pending review
                        </span>
                    )}
                </div>
            ) : (
                <Skeleton className="mt-1 h-10 w-56 rounded-full" />
            )}
            {children}
        </div>
    </div>
);

export default EntityHeader;
