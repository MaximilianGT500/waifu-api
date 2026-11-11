import { Bookmark, X } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useFilterLabels } from '../hooks/useFilterLabels';
import { AnimatedMode, NsfwMode, Orientation, ReviewStatusFilter } from '../types';
import { canModerate } from '../utils/roles';
import { EXTRA_FILTER_PARAMS, SINCE_OPTIONS, colorName, formatDay, normalizeHex } from '../utils/imageFilters';

type SetSearchParams = (params: URLSearchParams | ((prev: URLSearchParams) => URLSearchParams)) => void;

interface ActiveFiltersProps {
    searchParams: URLSearchParams;
    setSearchParams: SetSearchParams;
    totalCount?: number;
    isLoading?: boolean;
    onSave?: (summary: string) => void;
}

interface Chip {
    key: string;
    label: string;
    tone: 'neutral' | 'include' | 'exclude';
    remove: (prev: URLSearchParams) => void;
    title: string;
    swatch?: string;
}

const FILTER_PARAMS = [
    'isNsfw', 'orientation', 'isAnimated', 'width', 'height', 'byteSize',
    'includedTags', 'excludedTags', 'includedArtists', 'excludedArtists',
    'includedIds', 'excludedIds', 'uploaderId', 'reviewStatus',
    ...EXTRA_FILTER_PARAMS,
];

const TONE: Record<Chip['tone'], string> = {
    neutral: '',
    include: 'bg-brand-soft text-brand-ink',
    exclude: 'bg-destructive/10 text-destructive',
};

const removeValue = (key: string, value: string) => (prev: URLSearchParams) => {
    const rest = prev.getAll(key).filter((v) => v !== value);
    prev.delete(key);
    rest.forEach((v) => prev.append(key, v));
};

const removeKey = (key: string) => (prev: URLSearchParams) => prev.delete(key);

const describeRange = (value: string) => {
    const match = /^(>=|<=|>|<)?\s*(.+)$/.exec(value.trim());
    if (!match) return value;
    const [, op, rest] = match;
    const symbol = op === '>=' ? '≥' : op === '<=' ? '≤' : op ?? '';
    return `${symbol}${symbol ? ' ' : ''}${rest.replace('..', '–')}`;
};

const ActiveFilters = ({ searchParams, setSearchParams, totalCount, isLoading = false, onSave }: ActiveFiltersProps) => {
    const { user } = useAuth();
    const isModerator = !!user && canModerate(user);
    const tags = useFilterLabels('tag', [...searchParams.getAll('includedTags'), ...searchParams.getAll('excludedTags')]);
    const artists = useFilterLabels('artist', [...searchParams.getAll('includedArtists'), ...searchParams.getAll('excludedArtists')]);
    const uploaderId = searchParams.get('uploaderId') || '';
    const users = useFilterLabels('user', isModerator && uploaderId ? [uploaderId] : [], isModerator);

    const chips: Chip[] = [];
    const isNsfw = searchParams.get('isNsfw');
    if (isNsfw === NsfwMode.True || isNsfw === NsfwMode.All) {
        chips.push({ key: 'nsfw', tone: 'neutral', label: isNsfw === NsfwMode.True ? 'NSFW only' : 'Includes NSFW', title: 'Show safe images only', remove: removeKey('isNsfw') });
    }
    const orientation = searchParams.get('orientation');
    if (orientation && orientation !== Orientation.All) {
        chips.push({ key: 'orientation', tone: 'neutral', label: orientation, title: 'Any orientation', remove: removeKey('orientation') });
    }
    const animated = searchParams.get('isAnimated');
    if (animated === AnimatedMode.True || animated === AnimatedMode.False) {
        chips.push({ key: 'animated', tone: 'neutral', label: animated === AnimatedMode.True ? 'Animated only' : 'Static only', title: 'Any type', remove: removeKey('isAnimated') });
    }
    for (const [key, name, unit] of [['width', 'Width', 'px'], ['height', 'Height', 'px'], ['byteSize', 'File size', 'bytes']] as const) {
        const value = searchParams.get(key);
        if (value) chips.push({ key, tone: 'neutral', label: `${name} ${describeRange(value)} ${unit}`, title: `Any ${name.toLowerCase()}`, remove: removeKey(key) });
    }
    const color = normalizeHex(searchParams.get('color'));
    if (color) {
        chips.push({
            key: 'color', tone: 'neutral', swatch: `#${color}`, label: `Color: ${colorName(color)}`, title: 'Any color',
            remove: (prev) => { prev.delete('color'); prev.delete('colorTolerance'); },
        });
    }
    const since = SINCE_OPTIONS.find((o) => o.id === searchParams.get('since'));
    if (since) {
        chips.push({ key: 'since', tone: 'neutral', label: since.label, title: 'Any upload date', remove: removeKey('since') });
    }
    const uploadedAfter = searchParams.get('uploadedAfter');
    if (uploadedAfter && !since) {
        chips.push({ key: 'after', tone: 'neutral', label: `Uploaded after ${formatDay(uploadedAfter)}`, title: 'Any upload date', remove: removeKey('uploadedAfter') });
    }
    const uploadedBefore = searchParams.get('uploadedBefore');
    if (uploadedBefore) {
        chips.push({ key: 'before', tone: 'neutral', label: `Uploaded before ${formatDay(uploadedBefore)}`, title: 'Any upload date', remove: removeKey('uploadedBefore') });
    }
    for (const slug of searchParams.getAll('includedTags')) {
        const name = tags.get(slug)?.name ?? slug;
        chips.push({ key: `it-${slug}`, tone: 'include', label: name, title: `Remove the tag ${name}`, remove: removeValue('includedTags', slug) });
    }
    for (const slug of searchParams.getAll('excludedTags')) {
        const name = tags.get(slug)?.name ?? slug;
        chips.push({ key: `et-${slug}`, tone: 'exclude', label: `not ${name}`, title: `Stop excluding the tag ${name}`, remove: removeValue('excludedTags', slug) });
    }
    for (const id of searchParams.getAll('includedArtists')) {
        const name = artists.get(id)?.name ?? `Artist #${id}`;
        chips.push({ key: `ia-${id}`, tone: 'include', label: `by ${name}`, title: `Remove the artist ${name}`, remove: removeValue('includedArtists', id) });
    }
    for (const id of searchParams.getAll('excludedArtists')) {
        const name = artists.get(id)?.name ?? `Artist #${id}`;
        chips.push({ key: `ea-${id}`, tone: 'exclude', label: `not by ${name}`, title: `Stop excluding the artist ${name}`, remove: removeValue('excludedArtists', id) });
    }
    for (const id of searchParams.getAll('includedIds')) {
        chips.push({ key: `ii-${id}`, tone: 'include', label: `#${id}`, title: `Remove image #${id}`, remove: removeValue('includedIds', id) });
    }
    for (const id of searchParams.getAll('excludedIds')) {
        chips.push({ key: `ei-${id}`, tone: 'exclude', label: `not #${id}`, title: `Stop excluding image #${id}`, remove: removeValue('excludedIds', id) });
    }
    if (isModerator && uploaderId) {
        const name = users.get(uploaderId)?.name ?? `User #${uploaderId}`;
        chips.push({ key: 'uploader', tone: 'neutral', label: `uploaded by ${name}`, title: 'Any uploader', remove: removeKey('uploaderId') });
    }
    const reviewStatus = searchParams.get('reviewStatus');
    if (isModerator && reviewStatus && reviewStatus !== ReviewStatusFilter.Accepted) {
        chips.push({ key: 'review', tone: 'neutral', label: reviewStatus === ReviewStatusFilter.Pending ? 'Pending review' : 'All review states', title: 'Accepted images only', remove: removeKey('reviewStatus') });
    }

    const showCount = totalCount !== undefined && !isLoading;
    if (chips.length === 0 && !showCount) return null;

    const apply = (change: (prev: URLSearchParams) => void) => setSearchParams((prev) => {
        change(prev);
        prev.delete('page');
        return prev;
    });

    const clearAll = () => apply((prev) => FILTER_PARAMS.forEach((key) => prev.delete(key)));

    return (
        <div className="mb-5 flex min-h-8 flex-wrap items-center gap-2" aria-label="Active filters" role="region">
            {showCount && (
                <p className="mr-1 text-sm font-bold text-muted-foreground tabular" aria-live="polite">
                    {totalCount.toLocaleString()} {totalCount === 1 ? 'image' : 'images'}
                </p>
            )}
            {chips.map((chip) => (
                <span key={chip.key} className={`sticker gap-1 py-0.5 pl-3 pr-1 text-xs ${TONE[chip.tone]}`}>
                    {chip.swatch && <span className="size-3 shrink-0 rounded-full ring-1 ring-foreground/20" style={{ backgroundColor: chip.swatch }} aria-hidden="true" />}
                    <span className="max-w-[16rem] truncate">{chip.label}</span>
                    <button
                        type="button"
                        onClick={() => apply(chip.remove)}
                        title={chip.title}
                        aria-label={chip.title}
                        className="grid size-6 shrink-0 place-items-center rounded-full transition-colors hover:bg-foreground/10"
                    >
                        <X size={13} aria-hidden="true" />
                    </button>
                </span>
            ))}
            {chips.length > 1 && (
                <button type="button" onClick={clearAll} className="btn btn-ghost btn-sm">
                    Clear all
                </button>
            )}
            {onSave && chips.length > 0 && (
                <button type="button" onClick={() => onSave(chips.map((chip) => chip.label).join(', '))} className="btn btn-ghost btn-sm">
                    <Bookmark size={14} aria-hidden="true" /> Save search
                </button>
            )}
        </div>
    );
};

export default ActiveFilters;
