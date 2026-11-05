import { ReactNode } from 'react';
import { Link } from 'react-router';
import { Clock, Check, X, Ban } from 'lucide-react';
import { Report, ReportStatus } from '../types';
import { StatusBadge, StatusTone } from './StatusBadge';
import { imgCard } from '../utils/cfImage';

const REPORT_STATUS: Record<ReportStatus, { tone: StatusTone; icon: ReactNode }> = {
    [ReportStatus.Pending]: { tone: 'pending', icon: <Clock size={12} /> },
    [ReportStatus.Resolved]: { tone: 'success', icon: <Check size={12} /> },
    [ReportStatus.Rejected]: { tone: 'danger', icon: <X size={12} /> },
    [ReportStatus.Cancelled]: { tone: 'neutral', icon: <Ban size={12} /> },
};

interface ReportCardProps {
    report: Report;
    showReporter?: boolean;
    footer?: ReactNode;
}

export const ReportCard = ({ report, showReporter, footer }: ReportCardProps) => {
    const s = REPORT_STATUS[report.status];
    return (
        <article className="panel flex flex-col overflow-hidden">
            {report.image ? (
                <Link to={`/images/${report.imageId}`} className="group block bg-sleeve">
                    <div className="aspect-16/10 w-full overflow-hidden">
                        <img
                            src={imgCard(report.image.url)}
                            alt={`Reported upload #${report.imageId}`}
                            className={`size-full object-cover ${report.image.isNsfw ? 'blur-xl transition-[filter] duration-300 transform-gpu group-hover:blur-none group-focus-visible:blur-none' : ''}`}
                            loading="lazy"
                        />
                    </div>
                </Link>
            ) : (
                <div className="flex aspect-16/10 w-full items-center justify-center bg-sleeve text-sm font-bold text-muted-foreground">Image removed</div>
            )}

            <div className="flex flex-1 flex-col p-4">
                <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge label={report.status} tone={s.tone} icon={s.icon} />
                    <span className="text-xs font-semibold text-muted-foreground tabular">Report #{report.id}</span>
                </div>
                <p className="mt-3 text-sm wrap-break-word">{report.description || 'No description provided.'}</p>
                {report.reviewerNote && (
                    <p className="mt-3 rounded-2xl bg-secondary/70 px-3 py-2 text-xs text-muted-foreground wrap-break-word">
                        <span className="font-semibold text-foreground">Moderator note: </span>{report.reviewerNote}
                    </p>
                )}
                <p className="mt-auto pt-4 text-xs text-muted-foreground">
                    {showReporter && (
                        <>
                            {report.user ? (
                                <Link to={`/users?includedIds=${report.user.id}`} className="font-semibold text-foreground hover:underline">{report.user.name}</Link>
                            ) : 'Unknown user'}
                            {', '}
                        </>
                    )}
                    {new Date(report.createdAt).toLocaleDateString()}
                </p>
            </div>

            {footer}
        </article>
    );
};

export default ReportCard;
