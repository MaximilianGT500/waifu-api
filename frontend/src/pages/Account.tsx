import { ReactNode, useState } from 'react';
import { Link } from 'react-router';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import {
    Activity, ChevronLeft, ChevronRight, Download, Flag, GitPullRequest, Heart, History, Key, Library, Loader2, Bookmark,
    Palette, ShieldCheck, Upload as UploadIcon, LucideIcon,
} from 'lucide-react';
import api from '../services/api';
import { AccountSummary, ImageDto, ImageOrderBy, NsfwMode, PaginatedList, ReviewStatus } from '../types';
import { useRequireAuth } from '../hooks/useRequireAuth';
import { MetaTags } from '../hooks/useMetaTags';
import { useNotification } from '../context/NotificationContext';
import ThumbGrid from '../components/ThumbGrid';
import Skeleton from '../components/Skeleton';
import { useRecentImages } from '../hooks/useRecentImages';
import { clearRecentImages } from '../utils/recentImages';
import { useSavedSearches } from '../hooks/useSavedSearches';
import { clearSavedSearches } from '../utils/savedSearches';
import { readLocal, removeLocal } from '../utils/storage';
import { openWelcome } from '../utils/welcome';
import { DESIGNS } from '../utils/design';
import { useDesign } from '../hooks/useDesign';
import { useTheme } from '../context/ThemeContext';
import { ACCENTS } from '../utils/accent';

const UPLOADS_PAGE_SIZE = 12;

interface Tile {
    label: string;
    value: number;
    note?: string;
    icon: LucideIcon;
    to?: string;
}

const count = (n: number, one: string, many: string) => `${n.toLocaleString()} ${n === 1 ? one : many}`;

const TileCard = ({ tile }: { tile: Tile }) => {
    const Icon = tile.icon;
    const body = (
        <>
            <span className="flex items-center justify-between gap-2 text-sm font-bold text-muted-foreground">
                {tile.label}
                <Icon size={16} aria-hidden="true" />
            </span>
            <span className="mt-2 block text-3xl font-black tabular">{tile.value.toLocaleString()}</span>
            {tile.note && <span className="mt-1 block truncate text-xs text-muted-foreground">{tile.note}</span>}
        </>
    );
    const className = 'block rounded-[1.1rem] bg-card p-4 shadow-card dark:ring-1 dark:ring-white/6';
    return tile.to
        ? <Link to={tile.to} className={`${className} transition-shadow hover:shadow-lift`}>{body}</Link>
        : <div className={className}>{body}</div>;
};

const Row = ({ icon: Icon, title, detail, children }: { icon: LucideIcon; title: string; detail: ReactNode; children: ReactNode }) => (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 py-4">
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-secondary text-muted-foreground" aria-hidden="true">
            <Icon size={17} />
        </span>
        <span className="min-w-0 flex-1">
            <span className="block font-bold">{title}</span>
            <span className="block text-sm text-muted-foreground">{detail}</span>
        </span>
        <span className="flex shrink-0 gap-2">{children}</span>
    </li>
);

const Account = () => {
    const user = useRequireAuth();
    const { showNotification } = useNotification();
    const design = useDesign();
    const { resolvedTheme, theme, accent } = useTheme();
    const recent = useRecentImages();
    const saved = useSavedSearches();
    const [uploadsPage, setUploadsPage] = useState(1);
    const [isExporting, setIsExporting] = useState(false);
    const [consent, setConsent] = useState(() => readLocal('nsfw-consent') === 'true');

    const summary = useQuery({
        queryKey: ['account-summary'],
        enabled: !!user,
        staleTime: 30_000,
        queryFn: async ({ signal }) => (await api.get<AccountSummary>('/users/me/summary', { signal })).data,
    });

    const uploads = useQuery({
        queryKey: ['my-uploads', uploadsPage, consent],
        enabled: !!user,
        placeholderData: keepPreviousData,
        queryFn: async ({ signal }) => (await api.get<PaginatedList<ImageDto>>('/images', {
            params: {
                mine: true,
                includeMyPending: true,
                orderBy: ImageOrderBy.UploadedAt,
                isNsfw: consent ? NsfwMode.All : NsfwMode.False,
                pageSize: UPLOADS_PAGE_SIZE,
                page: uploadsPage,
            },
            signal,
        })).data,
    });

    if (!user) return null;

    const s = summary.data;
    const tiles: Tile[] = s ? [
        {
            label: 'Uploads', value: s.uploads.accepted, icon: UploadIcon, to: '#uploads',
            note: s.uploads.pending > 0 ? `${s.uploads.pending.toLocaleString()} waiting for review` : s.uploads.rejected > 0 ? `${s.uploads.rejected.toLocaleString()} rejected` : 'Nothing waiting for review',
        },
        { label: 'Favorites', value: s.favorites, icon: Heart, to: '/albums' },
        { label: 'Albums', value: s.albums, icon: Library, to: '/albums', note: count(s.albumImages, 'image', 'images') },
        { label: 'API keys', value: s.apiKeys, icon: Key, to: '/api-keys' },
        { label: 'Requests', value: s.requests.total, icon: Activity, note: `${s.requests.apiKey.toLocaleString()} with a key · ${s.requests.jwt.toLocaleString()} signed in` },
        { label: 'Reports', value: s.reports.total, icon: Flag, to: '/my-reports', note: s.reports.pending > 0 ? `${s.reports.pending.toLocaleString()} open` : 'None open' },
        { label: 'Submissions', value: s.submissions.total, icon: GitPullRequest, to: '/review', note: s.submissions.pending > 0 ? `${s.submissions.pending.toLocaleString()} waiting` : 'None waiting' },
    ] : [];

    const exportData = async () => {
        if (isExporting) return;
        setIsExporting(true);
        try {
            const { data } = await api.get<Blob>('/users/me/export', { responseType: 'blob' });
            const href = URL.createObjectURL(data);
            const link = document.createElement('a');
            link.href = href;
            link.download = `account-export-${user.id}.json`;
            document.body.append(link);
            link.click();
            link.remove();
            window.setTimeout(() => URL.revokeObjectURL(href), 10_000);
            showNotification('success', 'Your export is downloading.');
        } catch {
            showNotification('error', 'The export could not be created. Please try again.');
        } finally {
            setIsExporting(false);
        }
    };

    const withdrawConsent = () => {
        removeLocal('nsfw-consent');
        setConsent(false);
        setUploadsPage(1);
        showNotification('success', 'NSFW content is hidden again in this browser.');
    };

    const designLabel = DESIGNS.find((d) => d.id === design)?.label ?? 'Collector';
    const accentLabel = ACCENTS.find((a) => a.id === accent)?.label ?? 'Default';
    const modeLabel = theme === 'system' ? `System (${resolvedTheme})` : theme === 'dark' ? 'Dark' : 'Light';
    const page = uploads.data;

    return (
        <div className="page">
            <MetaTags title="Account" description="Your account at a glance." />
            <div className="page-header">
                <div className="flex items-center gap-4">
                    {user.avatarUrl && <img src={user.avatarUrl} alt="" width={56} height={56} className="size-14 shrink-0 rounded-full shadow-card" />}
                    <div>
                        <h1 className="page-title">{user.name}</h1>
                        <p className="page-lede">Your account at a glance, your uploads and everything this browser remembers.</p>
                    </div>
                </div>
            </div>

            <section aria-label="Overview" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
                {summary.isPending && Array.from({ length: 7 }, (_, i) => <Skeleton key={i} className="h-[7.25rem] rounded-[1.1rem]" />)}
                {summary.isError && <p className="col-span-full text-sm font-bold text-destructive">The overview could not be loaded.</p>}
                {tiles.map((tile) => <TileCard key={tile.label} tile={tile} />)}
            </section>

            <section id="uploads" aria-labelledby="uploads-title" className="mt-14 scroll-mt-24">
                <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                    <div>
                        <h2 id="uploads-title" className="section-title text-xl font-black">Your uploads</h2>
                        <p className="mt-1 text-sm text-muted-foreground">
                            Newest first, including the ones still waiting for review.{!consent && ' NSFW uploads stay hidden until you allow NSFW content.'}
                        </p>
                    </div>
                    <Link to="/upload" className="btn btn-secondary btn-sm"><UploadIcon size={15} aria-hidden="true" /> Upload</Link>
                </div>
                {uploads.isError ? (
                    <p className="text-sm font-bold text-destructive">Your uploads could not be loaded.</p>
                ) : page && page.items.length === 0 && !uploads.isPending ? (
                    <p className="rounded-[1.1rem] bg-sleeve p-6 text-center text-sm text-muted-foreground">Nothing here yet. Your uploads show up here as soon as you send them.</p>
                ) : (
                    <div className={uploads.isPlaceholderData ? 'opacity-60 transition-opacity' : undefined}>
                        <ThumbGrid
                            items={page?.items ?? []}
                            loading={uploads.isPending}
                            count={UPLOADS_PAGE_SIZE}
                            caption={(item) => (item.reviewStatus === ReviewStatus.Pending ? 'Pending' : item.isNsfw ? 'NSFW' : null)}
                        />
                    </div>
                )}
                {page && page.totalPages > 1 && (
                    <div className="mt-4 flex items-center justify-center gap-3">
                        <button type="button" onClick={() => setUploadsPage((p) => Math.max(1, p - 1))} disabled={!page.hasPreviousPage} className="btn btn-secondary btn-sm">
                            <ChevronLeft size={15} aria-hidden="true" /> Newer
                        </button>
                        <span className="text-sm font-bold tabular text-muted-foreground">{page.pageNumber} / {page.totalPages}</span>
                        <button type="button" onClick={() => setUploadsPage((p) => p + 1)} disabled={!page.hasNextPage} className="btn btn-secondary btn-sm">
                            Older <ChevronRight size={15} aria-hidden="true" />
                        </button>
                    </div>
                )}
            </section>

            <div className="mt-14 grid gap-10 lg:grid-cols-2">
                <section aria-labelledby="data-title">
                    <h2 id="data-title" className="section-title text-xl font-black">Your data</h2>
                    <p className="mt-1 text-sm text-muted-foreground">
                        Download everything stored about your account as one JSON file: your profile, albums, API keys without their secrets, uploads, the tags and artists you created, reports and submissions.
                    </p>
                    <button type="button" onClick={exportData} disabled={isExporting} className="btn btn-primary mt-4">
                        {isExporting ? <Loader2 size={17} className="animate-spin" aria-hidden="true" /> : <Download size={17} aria-hidden="true" />}
                        Download my data
                    </button>
                </section>

                <section aria-labelledby="browser-title">
                    <h2 id="browser-title" className="section-title text-xl font-black">This browser</h2>
                    <p className="mt-1 text-sm text-muted-foreground">Kept only on this device and never sent to the server.</p>
                    <ul className="mt-2 divide-y-2 divide-dashed divide-border">
                        <Row icon={Palette} title="Look" detail={`${designLabel} design · ${modeLabel} · ${accentLabel}`}>
                            <button type="button" onClick={openWelcome} className="btn btn-secondary btn-sm">Change</button>
                        </Row>
                        <Row icon={History} title="Recently viewed" detail={count(recent.length, 'image', 'images')}>
                            <button type="button" onClick={clearRecentImages} disabled={recent.length === 0} className="btn btn-ghost btn-sm">Clear</button>
                        </Row>
                        <Row icon={Bookmark} title="Saved searches" detail={count(saved.length, 'search', 'searches')}>
                            <button type="button" onClick={clearSavedSearches} disabled={saved.length === 0} className="btn btn-ghost btn-sm">Clear</button>
                        </Row>
                        <Row icon={ShieldCheck} title="NSFW content" detail={consent ? 'Allowed in this browser' : 'Hidden until you confirm your age'}>
                            {consent && <button type="button" onClick={withdrawConsent} className="btn btn-ghost btn-sm">Hide again</button>}
                        </Row>
                    </ul>
                </section>
            </div>
        </div>
    );
};

export default Account;
