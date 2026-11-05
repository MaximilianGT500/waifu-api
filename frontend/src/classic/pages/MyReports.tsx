import { useState, useEffect, useEffectEvent } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import api from '../../services/api';
import { Report, PaginatedList, ReportStatus } from '../../types';
import { useNotification } from '../../context/NotificationContext';
import { Edit2, Ban, Inbox } from 'lucide-react';
import Pagination from '../components/Pagination';
import ReportCard from '../components/ReportCard';
import StatusTabs from '../components/StatusTabs';
import ReasonModal from '../components/modals/ReasonModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import { useRequireAuth } from '../../hooks/useRequireAuth';
import { invalidateNavBadges } from '../../hooks/useNavBadges';
import { MetaTags } from '../../hooks/useMetaTags';

type StatusFilter = 'All' | ReportStatus;

const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
    { value: 'All', label: 'All' },
    { value: ReportStatus.Pending, label: 'Pending' },
    { value: ReportStatus.Resolved, label: 'Resolved' },
    { value: ReportStatus.Rejected, label: 'Rejected' },
    { value: ReportStatus.Cancelled, label: 'Cancelled' },
];

const MyReports = () => {
    const user = useRequireAuth();
    const { showNotification } = useNotification();
    const queryClient = useQueryClient();
    const [reports, setReports] = useState<Report[]>([]);
    const [loading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);
    const [statusFilter, setStatusFilter] = useState<StatusFilter>(ReportStatus.Pending);
    const [editing, setEditing] = useState<Report | null>(null);
    const [cancelTarget, setCancelTarget] = useState<Report | null>(null);
    const pageSize = 30;

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

    const handleEdit = async (description: string) => {
        if (!editing) return;
        try {
            const { data } = await api.patch<Report>(`/reports/${editing.id}`, { description });
            setReports(prev => prev.map(r => (r.id === data.id ? data : r)));
            showNotification('success', 'Report updated');
            setEditing(null);
        } catch {
            return;
        }
    };

    const handleCancel = async () => {
        if (!cancelTarget) return;
        try {
            await api.patch(`/reports/${cancelTarget.id}/cancel`);
            showNotification('success', 'Report withdrawn');
            setCancelTarget(null);
            invalidateNavBadges(queryClient);
            fetchReports();
        } catch {
            return;
        }
    };

    if (!user) return null;

    return (
        <div className="page flex h-full flex-col">
            <MetaTags title="My Reports" description="Images you have reported." />

            <div className="page-header">
                <div>
                    <h1 className="page-title">My Reports</h1>
                    <p className="page-lede">Images you reported and what the moderators decided.</p>
                </div>
            </div>

            <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:flex-wrap">
                <StatusTabs value={statusFilter} onChange={setStatusFilter} options={STATUS_OPTIONS} />
            </div>

            {loading ? (
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                    {[...Array(3)].map((_, i) => <div key={i} className="h-80 animate-pulse rounded-2xl bg-secondary" />)}
                </div>
            ) : reports.length === 0 ? (
                <div className="flex h-64 flex-col items-center justify-center rounded-2xl border-2 border-dashed border-border px-6 text-center text-muted-foreground">
                    <Inbox size={40} className="mb-3 opacity-30" aria-hidden="true" />
                    <p className="font-semibold text-foreground">No reports here</p>
                    <p className="mt-1 text-sm">When you report an image, it shows up here with its status.</p>
                </div>
            ) : (
                <div className="flex-1">
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                        {reports.map(report => (
                            <ReportCard
                                key={report.id}
                                report={report}
                                footer={report.status === ReportStatus.Pending && (
                                    <div className="flex gap-2 border-t border-border p-3">
                                        <button type="button" onClick={() => setEditing(report)} className="btn btn-soft btn-sm flex-1">
                                            <Edit2 size={15} aria-hidden="true" /> Edit
                                        </button>
                                        <button type="button" onClick={() => setCancelTarget(report)} className="btn btn-soft btn-sm flex-1 hover:bg-destructive/10 hover:text-destructive">
                                            <Ban size={15} aria-hidden="true" /> Withdraw
                                        </button>
                                    </div>
                                )}
                            />
                        ))}
                    </div>
                    <Pagination currentPage={page} totalPages={totalPages} setPage={setPage} />
                </div>
            )}

            <ReasonModal
                isOpen={!!editing}
                onClose={() => setEditing(null)}
                onSubmit={handleEdit}
                title="Edit Report"
                description="Update the description for this report."
                placeholder="Describe the issue with this image..."
                submitText="Save"
                confirmVariant="primary"
                maxLength={500}
                initialValue={editing?.description ?? ''}
            />

            <ConfirmModal
                isOpen={!!cancelTarget}
                onClose={() => setCancelTarget(null)}
                onConfirm={handleCancel}
                title="Withdraw Report"
                message="Withdraw this report? It will no longer be reviewed by moderators."
                confirmText="Withdraw"
                variant="destructive"
            />
        </div>
    );
};

export default MyReports;
