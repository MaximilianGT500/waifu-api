import { ReactNode } from 'react';

export type StatusTone = 'pending' | 'success' | 'danger' | 'neutral' | 'info';

const TONES: Record<StatusTone, string> = {
    pending: 'bg-warning/12 text-warning',
    success: 'bg-success/12 text-success',
    danger: 'bg-destructive/12 text-destructive',
    neutral: 'bg-secondary text-muted-foreground',
    info: 'bg-info/12 text-info',
};

export const StatusBadge = ({ label, tone, icon }: { label: string; tone: StatusTone; icon?: ReactNode }) => (
    <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${TONES[tone]}`}>
        {icon}{label}
    </span>
);

export default StatusBadge;
