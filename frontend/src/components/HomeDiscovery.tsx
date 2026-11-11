import { Link } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight, Flame, Palette, Sparkles } from 'lucide-react';
import api from '../services/api';
import { ImageDto, ImageOrderBy, NsfwMode, PaginatedList } from '../types';
import ThumbGrid from './ThumbGrid';
import { visitSince } from '../utils/lastVisit';
import { COLOR_SWATCHES, formatDay } from '../utils/imageFilters';

const COUNT = 6;

export const NewSinceVisit = () => {
    const since = visitSince();
    const fresh = useQuery({
        queryKey: ['new-since', since],
        enabled: !!since,
        staleTime: 5 * 60_000,
        queryFn: async ({ signal }) => (await api.get<PaginatedList<ImageDto>>('/images', {
            params: { uploadedAfter: since, isNsfw: NsfwMode.False, orderBy: ImageOrderBy.UploadedAt, pageSize: 1 },
            skipGlobalErrorHandler: true,
            signal,
        })).data.totalCount,
    });
    if (!since || !fresh.data) return null;

    const href = `/gallery?${new URLSearchParams({ orderBy: ImageOrderBy.UploadedAt, uploadedAfter: since })}`;
    return (
        <Link
            to={href}
            className="group mt-10 flex items-center gap-3 rounded-[1.25rem] bg-brand-soft p-3 pl-4 text-sm transition-colors hover:bg-brand/15"
        >
            <span className="grid size-9 shrink-0 place-items-center rounded-full bg-card text-brand-ink shadow-card" aria-hidden="true">
                <Sparkles size={17} />
            </span>
            <span className="min-w-0 flex-1">
                <span className="block font-black">{fresh.data.toLocaleString()} new {fresh.data === 1 ? 'image' : 'images'} since your last visit</span>
                <span className="block text-muted-foreground">Uploaded after {formatDay(since)}</span>
            </span>
            <span className="inline-flex items-center gap-1 font-bold text-brand-ink">
                Show <ArrowRight size={15} aria-hidden="true" className="transition-transform group-hover:translate-x-0.5" />
            </span>
        </Link>
    );
};

export const TrendingRow = () => {
    const trending = useQuery({
        queryKey: ['home-trending'],
        staleTime: 5 * 60_000,
        queryFn: async ({ signal }) => (await api.get<PaginatedList<ImageDto>>('/images', {
            params: { orderBy: ImageOrderBy.Trending, isNsfw: NsfwMode.False, pageSize: COUNT },
            skipGlobalErrorHandler: true,
            signal,
        })).data.items,
    });
    if (trending.isError || (trending.isSuccess && trending.data.length === 0)) return null;

    return (
        <section aria-labelledby="trending" className="mt-14">
            <div className="mb-4 flex flex-wrap items-baseline justify-between gap-3">
                <h2 id="trending" className="section-title flex items-center gap-2 text-xl font-black">
                    <Flame size={19} aria-hidden="true" className="text-brand" /> Trending this week
                </h2>
                <Link to={`/gallery?orderBy=${ImageOrderBy.Trending}`} className="inline-flex items-center gap-1.5 text-sm font-bold text-muted-foreground transition-colors hover:text-foreground">
                    See all <ArrowRight size={15} aria-hidden="true" />
                </Link>
            </div>
            <ThumbGrid
                items={trending.data ?? []}
                loading={trending.isPending}
                count={COUNT}
                caption={(item) => (item.favorites > 0 ? `♥ ${item.favorites.toLocaleString()}` : null)}
            />
        </section>
    );
};

export const ColorRow = () => (
    <section aria-labelledby="by-color" className="mt-14">
        <h2 id="by-color" className="section-title mb-4 flex items-center gap-2 text-xl font-black">
            <Palette size={19} aria-hidden="true" className="text-muted-foreground" /> Browse by color
        </h2>
        <ul className="flex flex-wrap gap-2.5">
            {COLOR_SWATCHES.map((swatch) => (
                <li key={swatch.hex}>
                    <Link
                        to={`/gallery?color=${swatch.hex}`}
                        className="color-chip sticker gap-2 py-1.5 pl-1.5 pr-3.5 text-sm transition-[background-color,rotate] duration-200 hover:-rotate-1 hover:bg-secondary motion-reduce:hover:rotate-0"
                    >
                        <span className="size-6 rounded-full ring-1 ring-foreground/15" style={{ backgroundColor: `#${swatch.hex}` }} aria-hidden="true" />
                        {swatch.name}
                    </Link>
                </li>
            ))}
        </ul>
    </section>
);
