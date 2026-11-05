import { useState, useEffect, useEffectEvent } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import api from '../services/api';
import { Report, PaginatedList, ReportStatus } from '../types';
import { useNotification } from '../context/NotificationContext';
import { Check, X, Trash2 } from 'lucide-react';
import ConfirmModal from '../components/modals/ConfirmModal';
import ReasonModal from '../components/modals/ReasonModal';
import Pagination from '../components/Pagination';
import ReportCard from '../components/ReportCard';
import StatusTabs from '../components/StatusTabs';
import { useRequireAuth } from '../hooks/useRequireAuth';
import { invalidateNavBadges } from '../hooks/useNavBadges';

type StatusFilter = 'All' | ReportStatus;

const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
    { value: 'All', label: 'All' },
    { value: ReportStatus.Pending, label: 'Pending' },
    { value: ReportStatus.Resolved, label: 'Resolved' },
    { value: ReportStatus.Rejected, label: 'Rejected' },
    { value: ReportStatus.Cancelled, label: 'Cancelled' },
];

const Reports = () => {
    const user = useRequireAuth();
    const { showNotification } = useNotification();
    const queryClient = useQueryClient();
    const [reports, setReports] = useState<Report[]>([]);
    const [loading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);
    const [statusFilter, setStatusFilter] = useState<StatusFilter>(ReportStatus.Pending);
    const pageSize = 30;

    const [reportToDelete, setReportToDelete] = useState<Report | null>(null);
    const [actionTarget, setActionTarget] = useState<{ report: Report; action: 'validate' | 'reject' } | null>(null);

    const fetchReports = async () => {
        if (!user) return;
        setLoading(true);
        try {
            const params: Record<string, string | number> = { page, pageSize };
            if (statusFilter !== 'All') params.status = statusFilter;
            const { data } = await api.get<PaginatedList<Report>>('/reports', { params });
            setReports(data.items);
            setTotalPages(data.totalPages);
        } catch {
            return;
        } finally {
            setLoading(false);
        }
    };

    const loadReports = useEffectEvent(() => {
        fetchReports();
    });

    useEffect(() => {
        if (user) loadReports();
    }, [page, user, statusFilter]);

    useEffect(() => { setPage(1); }, [statusFilter]);

    const submitAction = async (reason: string) => {
        if (!actionTarget) return;
        const { report, action } = actionTarget;
        try {
            await api.patch(`/reports/${report.id}/${action}`, { reason: reason || undefined });
            showNotification('success', action === 'validate' ? 'Report validated' : 'Report rejected');
            setActionTarget(null);
            invalidateNavBadges(queryClient);
            fetchReports();
        } catch {
            return;
        }
    };

    const handleDeleteImage = async () => {
        if (!reportToDelete) return;
        try {
            await api.delete(`/images/${reportToDelete.imageId}`);
            showNotification('success', 'Image deleted');
            setReportToDelete(null);
            invalidateNavBadges(queryClient);
            fetchReports();
        } catch {
            return;
        }
    };

    if (!user) return null;

    return (
        <div className="page flex h-full flex-col">
            <div className="page-header">
                <div>
                    <h1 className="page-title">Reports</h1>
                    <p className="page-lede">Images the community flagged. Validate, dismiss or remove them.</p>
                </div>
            </div>

            <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:flex-wrap">
                <StatusTabs value={statusFilter} onChange={setStatusFilter} options={STATUS_OPTIONS} />
            </div>

            {loading ? (
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                    {[...Array(3)].map((_, i) => <div key={i} className="h-80 animate-pulse rounded-[var(--radius-panel)] bg-sleeve" />)}
                </div>
            ) : reports.length === 0 ? (
                <div className="flex h-64 flex-col items-center justify-center rounded-2xl border-2 border-dashed border-border px-6 text-center text-muted-foreground">
                    <Check size={40} className="mb-3 opacity-30" aria-hidden="true" />
                    <p className="font-semibold text-foreground">Nothing to review</p>
                    <p className="mt-1 text-sm">No {statusFilter === 'All' ? '' : statusFilter.toLowerCase() + ' '}reports here.</p>
                </div>
            ) : (
                <div className="flex-1">
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                        {reports.map(report => (
                            <ReportCard
                                key={report.id}
                                report={report}
                                showReporter
                                footer={report.status === ReportStatus.Pending && (
                                    <div className="flex flex-wrap gap-2 border-t border-border p-3">
                                        <button
                                            type="button"
                                            onClick={() => setActionTarget({ report, action: 'validate' })}
                                            className="btn btn-soft btn-sm min-w-[5.5rem] flex-1 hover:bg-success/12 hover:text-success"
                                            title="The report is valid (keep image)"
                                        >
                                            <Check size={15} aria-hidden="true" /> Validate
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => setActionTarget({ report, action: 'reject' })}
                                            className="btn btn-soft btn-sm min-w-[5.5rem] flex-1 hover:bg-warning/12 hover:text-warning"
                                            title="Dismiss the report (keep image)"
                                        >
                                            <X size={15} aria-hidden="true" /> Reject
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => setReportToDelete(report)}
                                            className="btn btn-soft btn-sm min-w-[5.5rem] flex-1 hover:bg-destructive/10 hover:text-destructive"
                                            title="Delete the reported image"
                                        >
                                            <Trash2 size={15} aria-hidden="true" /> Delete image
                                        </button>
                                    </div>
                                )}
                            />
                        ))}
                    </div>
                    <Pagination currentPage={page} totalPages={totalPages} setPage={setPage} />
                </div>
            )}

            <ConfirmModal
                isOpen={!!reportToDelete}
                onClose={() => setReportToDelete(null)}
                onConfirm={handleDeleteImage}
                title="Delete Reported Image"
                message={<>Permanently delete this image? Its reports are kept and marked resolved. This cannot be undone.</>}
                confirmText="Delete Image"
                variant="destructive"
            />

            <ReasonModal
                isOpen={!!actionTarget}
                onClose={() => setActionTarget(null)}
                onSubmit={submitAction}
                title={actionTarget?.action === 'reject' ? 'Reject report' : 'Validate report'}
                description={actionTarget?.action === 'reject'
                    ? 'Optionally tell the reporter why this report was dismissed.'
                    : 'Optionally add a note for the reporter.'}
                placeholder="Reason (optional)..."
                submitText={actionTarget?.action === 'reject' ? 'Reject' : 'Validate'}
                required={false}
                confirmVariant={actionTarget?.action === 'reject' ? 'destructive' : 'primary'}
                maxLength={500}
            />
        </div>
    );
};

export default Reports;
