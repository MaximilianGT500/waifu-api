import { ReactNode } from 'react';
import { Link } from 'react-router';
import { Plus, Minus, Clock } from 'lucide-react';

export type TagChipTone = 'neutral' | 'pending' | 'add' | 'remove';

const TONES: Record<TagChipTone, { cls: string; icon?: ReactNode }> = {
    neutral: { cls: '' },
    pending: { cls: 'text-warning outline-1 outline-dashed -outline-offset-1 outline-warning/60', icon: <Clock size={11} className="shrink-0" /> },
    add: { cls: 'bg-[color-mix(in_oklab,var(--success)_14%,var(--card))] text-success', icon: <Plus size={12} className="shrink-0" /> },
    remove: { cls: 'bg-[color-mix(in_oklab,var(--destructive)_12%,var(--card))] text-destructive line-through decoration-destructive/50', icon: <Minus size={12} className="shrink-0" /> },
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
    const base = `sticker gap-1 px-2.5 py-0.5 text-xs ${t.cls}`;
    const inner = <>{lead}<span className="truncate">{label}</span>{trailingIcon}</>;
    return href
        ? <Link to={href} title={title} className={`${base} transition-[rotate,background-color] duration-200 hover:-rotate-2 hover:bg-secondary motion-reduce:hover:rotate-0`}>{inner}</Link>
        : <span title={title} className={base}>{inner}</span>;
};

export default TagChip;
