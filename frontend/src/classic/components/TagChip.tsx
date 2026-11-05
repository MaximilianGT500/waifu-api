import { ReactNode } from 'react';
import { Link } from 'react-router';
import { Plus, Minus, Clock } from 'lucide-react';

export type TagChipTone = 'neutral' | 'pending' | 'add' | 'remove';

const TONES: Record<TagChipTone, { cls: string; icon?: ReactNode }> = {
    neutral: { cls: 'bg-secondary text-secondary-foreground' },
    pending: { cls: 'bg-warning/10 text-warning outline-1 outline-dashed -outline-offset-1 outline-warning/50', icon: <Clock size={11} className="shrink-0" /> },
    add: { cls: 'bg-success/12 text-success', icon: <Plus size={12} className="shrink-0" /> },
    remove: { cls: 'bg-destructive/10 text-destructive line-through decoration-destructive/50', icon: <Minus size={12} className="shrink-0" /> },
};

interface TagChipProps {
    label: ReactNode;
    tone?: TagChipTone;
    href?: string;
    icon?: ReactNode;
    trailingIcon?: ReactNode;
    title?: string;
}

export const TagChip = ({ label, tone = 'neutral', href, icon, trailingIcon, title }: TagChipProps) => {
    const t = TONES[tone];
    const lead = icon !== undefined ? icon : t.icon;
    const base = `inline-flex max-w-full items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${t.cls}`;
    const inner = <>{lead}<span className="truncate">{label}</span>{trailingIcon}</>;
    return href
        ? <Link to={href} title={title} className={`${base} transition-[filter,background-color] hover:brightness-95 dark:hover:brightness-125`}>{inner}</Link>
        : <span title={title} className={base}>{inner}</span>;
};

export default TagChip;
