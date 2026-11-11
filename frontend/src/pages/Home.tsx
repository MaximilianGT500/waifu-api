import { Link } from 'react-router';
import { Book, Shuffle, ImageOff, LayoutGrid, Loader2 } from 'lucide-react';
import { CSSProperties, ReactNode, useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import api from '../services/api';
import { ImageDto, PaginatedList, NsfwMode, Orientation, AnimatedMode, ImageOrderBy } from '../types';
import { getEnv } from '../utils/env';
import Skeleton from '../components/Skeleton';
import { MetaTags } from '../hooks/useMetaTags';
import { imgCard, imgCardSrcSet } from '../utils/cfImage';
import ExternalLink from '../components/ExternalLink';
import TagChip from '../components/TagChip';
import { safeColor } from '../utils/color';
import { useHolo } from '../hooks/useHolo';
import { effectsEnabled } from '../utils/motion';
import { sfx } from '../utils/sfx';
import PopularTags from '../components/PopularTags';
import RecentlyViewed from '../components/RecentlyViewed';
import { ColorRow, NewSinceVisit, TrendingRow } from '../components/HomeDiscovery';

interface PublicStats {
    totalRequests: number;
    totalImages: number;
    totalTags: number;
    totalArtists: number;
}

interface HeroResponse {
    image: ImageDto | null;
    status: number;
    ms: number;
}

const HERO_QUERY: Record<string, string> = {
    isNsfw: NsfwMode.False,
    orientation: Orientation.Portrait,
    isAnimated: AnimatedMode.False,
    width: '>=1000',
    orderBy: ImageOrderBy.Random,
    pageSize: '1',
};

const HERO_SIZES = '(min-width: 1280px) 20rem, (min-width: 640px) 19rem, 76vw';
const FLIP_DWELL_MS = 650;

function formatCompactNumber(n: number): string {
    if (n >= 1_000_000_000) return `${+(n / 1_000_000_000).toFixed(1)}B`;
    if (n >= 1_000_000) return `${+(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${+(n / 1_000).toFixed(1)}K`;
    return n.toString();
}

function displayBase(): string {
    const base = getEnv('VITE_API_URL') || '/api';
    try {
        const url = new URL(base, window.location.origin);
        const path = url.pathname.replace(/\/$/, '');
        return url.origin === window.location.origin ? path : `${url.host}${path}`;
    } catch {
        return base;
    }
}

async function fetchHero(): Promise<HeroResponse> {
    const started = performance.now();
    const res = await api.get<PaginatedList<ImageDto>>('/images', { params: HERO_QUERY, skipGlobalErrorHandler: true });
    return { image: res.data.items[0] ?? null, status: res.status, ms: Math.round(performance.now() - started) };
}

const useHero = () => useQuery({ queryKey: ['home-hero'], queryFn: fetchHero, staleTime: Infinity, gcTime: 5 * 60_000 });

const Line = ({ index, indent = 1, children }: { index: number; indent?: number; children: ReactNode }) => (
    <div className="response-line truncate" style={{ '--line': index, paddingLeft: `${indent * 1.25}rem` } as CSSProperties}>
        {children}
    </div>
);

const Key = ({ children }: { children: string }) => <span className="text-foreground">"{children}"</span>;
const Str = ({ children }: { children: string }) => <span className="text-info">"{children}"</span>;
const Num = ({ children }: { children: number }) => <span className="text-brand-ink">{children}</span>;
const Punct = ({ children }: { children: string }) => <span className="text-muted-foreground">{children}</span>;

const ResponseBody = ({ image }: { image: ImageDto }) => {
    const artists = (image.artists ?? []).map((a) => a.name);
    const tagCount = image.tags?.length ?? 0;
    const color = safeColor(image.dominantColor);
    let line = 0;
    return (
        <div className="font-mono text-[0.8125rem] leading-6 text-muted-foreground">
            <Line index={line++} indent={0}><Punct>{'{'}</Punct></Line>
            <Line index={line++}><Key>id</Key><Punct>: </Punct><Num>{image.id}</Num><Punct>,</Punct></Line>
            <Line index={line++}><Key>url</Key><Punct>: </Punct><Str>{image.url}</Str><Punct>,</Punct></Line>
            <Line index={line++}><Key>width</Key><Punct>: </Punct><Num>{image.width}</Num><Punct>, </Punct><Key>height</Key><Punct>: </Punct><Num>{image.height}</Num><Punct>,</Punct></Line>
            {color && (
                <Line index={line++}>
                    <Key>dominantColor</Key><Punct>: </Punct>
                    <span className="mr-1 inline-block size-2.5 rounded-full align-middle ring-1 ring-border" style={{ backgroundColor: color }} aria-hidden="true" />
                    <Str>{color}</Str><Punct>,</Punct>
                </Line>
            )}
            {artists.length > 0 && (
                <Line index={line++}><Key>artists</Key><Punct>: [</Punct>{artists.slice(0, 2).map((name, i) => (
                    <span key={name}>{i > 0 && <Punct>, </Punct>}<Punct>{'{ '}</Punct><Key>name</Key><Punct>: </Punct><Str>{name}</Str><Punct>{' }'}</Punct></span>
                ))}{artists.length > 2 && <Punct>, …</Punct>}<Punct>],</Punct></Line>
            )}
            <Line index={line++}>
                <Key>tags</Key><Punct>: [</Punct>
                <Punct>{`${Array.from({ length: Math.min(tagCount, 3) }, () => '{…}').join(', ')}${tagCount > 3 ? ', …' : ''}`}</Punct>
                <Punct>]</Punct>
            </Line>
            <Line index={line++} indent={0}><Punct>{'}'}</Punct></Line>
        </div>
    );
};

interface HeroCardProps {
    image: ImageDto | null;
    flipped: boolean;
    broken: boolean;
    onLoaded: (id: number) => void;
    onBroken: (id: number) => void;
}

const HeroCard = ({ image, flipped, broken, onLoaded, onBroken }: HeroCardProps) => {
    const holo = useHolo<HTMLDivElement>(12);
    const color = image ? safeColor(image.dominantColor) : null;
    const artists = image ? (image.artists ?? []).map((a) => a.name).join(', ') : '';

    return (
        <div className="relative w-[min(76vw,19rem)] xl:w-80">
            <div className="card-back absolute inset-0 translate-x-5 translate-y-1 rotate-[7deg] rounded-[1.5rem] shadow-card" aria-hidden="true" />
            <div
                onPointerMove={holo.onPointerMove}
                onPointerLeave={holo.onPointerLeave}
                className="holo-card relative -rotate-2 [transform-style:preserve-3d]"
            >
                <div className="flip-card relative aspect-[5/7]" data-flipped={flipped ? '' : undefined}>
                    <div
                        className="flip-face absolute inset-0 flex flex-col rounded-[1.5rem] bg-card p-2.5 shadow-lift dark:ring-1 dark:ring-white/8"
                        inert={flipped || !image}
                    >
                        {image ? (
                            <>
                                <Link
                                    to={`/images/${image.id}`}
                                    className="relative block min-h-0 flex-1 overflow-hidden rounded-[1.05rem] outline-offset-4"
                                    style={{ backgroundColor: color ?? undefined }}
                                >
                                    {broken ? (
                                        <span className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-sleeve text-sm font-bold text-muted-foreground">
                                            <ImageOff size={22} aria-hidden="true" /> Image unavailable
                                        </span>
                                    ) : (
                                        <img
                                            key={image.id}
                                            src={imgCard(image.url)}
                                            srcSet={imgCardSrcSet(image.url, image.width)}
                                            sizes={HERO_SIZES}
                                            alt={artists ? `Image #${image.id} by ${artists}` : `Image #${image.id}`}
                                            width={image.width}
                                            height={image.height}
                                            decoding="async"
                                            fetchPriority="high"
                                            onLoad={() => onLoaded(image.id)}
                                            onError={() => onBroken(image.id)}
                                            className="size-full object-cover object-[50%_28%]"
                                        />
                                    )}
                                    <span className="holo-foil" aria-hidden="true" />
                                </Link>
                                <div className="flex items-baseline gap-3 px-1.5 pt-2.5">
                                    <span className="text-lg font-black tabular">#{image.id}</span>
                                    <span className="ml-auto truncate text-[0.8125rem] font-bold text-muted-foreground">{artists || 'Unknown artist'}</span>
                                </div>
                                <p className="px-1.5 pb-0.5 text-xs text-muted-foreground tabular">{image.width} × {image.height}</p>
                            </>
                        ) : null}
                    </div>
                    <div className="flip-face flip-face-back card-back absolute inset-0 grid place-items-center rounded-[1.5rem] shadow-lift" aria-hidden="true">
                        <span className="grid size-28 place-items-center rounded-full bg-white/8 ring-2 ring-brand/40">
                            <img src="/logo-144.png?v=20261024-logo-v2" alt="" width={72} height={72} className="size-[4.5rem] rounded-[1.25rem] bg-zinc-50 object-cover p-1" />
                        </span>
                    </div>
                </div>
            </div>
        </div>
    );
};

const StatusSticker = ({ state }: { state: 'pending' | 'online' | 'offline' }) => (
    <p className="sticker min-h-8 text-muted-foreground">
        {state === 'pending' ? (
            <>
                <Skeleton className="size-2 rounded-full" />
                <Skeleton className="h-3 w-16 rounded-sm" />
            </>
        ) : state === 'online' ? (
            <>
                <span className="relative flex size-2" aria-hidden="true">
                    <span className="absolute inline-flex size-full rounded-full bg-success opacity-60 motion-safe:animate-ping" />
                    <span className="relative inline-flex size-2 rounded-full bg-success" />
                </span>
                <span>API Online</span>
            </>
        ) : (
            <>
                <span className="size-2 rounded-full bg-destructive" aria-hidden="true" />
                <span>API Offline</span>
            </>
        )}
    </p>
);

const Home = () => {
    const hero = useHero();
    const stats = useQuery({
        queryKey: ['public-stats'],
        queryFn: async () => (await api.get<PublicStats>('/stats/public', { skipGlobalErrorHandler: true })).data,
        staleTime: 60_000,
    });
    const [loadedId, setLoadedId] = useState<number | null>(null);
    const [brokenId, setBrokenId] = useState<number | null>(null);
    const [dwelling, setDwelling] = useState(false);
    const [draws, setDraws] = useState(0);
    const dwellTimer = useRef<number | undefined>(undefined);

    useEffect(() => () => window.clearTimeout(dwellTimer.current), []);

    const image = hero.data?.image ?? null;
    const settled = !!image && (loadedId === image.id || brokenId === image.id);
    const flipped = hero.isFetching || dwelling || !settled;
    const drawing = draws > 0 && flipped && (hero.isFetching || dwelling || (!!image && !settled));
    const docsUrl = getEnv('VITE_DOCS_URL');
    const requestLine = `${displayBase()}/images?${Object.entries(HERO_QUERY).map(([k, v]) => `${k}=${v}`).join('&')}`;

    const draw = () => {
        if (drawing) return;
        sfx('BAM!', document.querySelector('[data-sfx="draw"]'));
        setDraws((n) => n + 1);
        window.clearTimeout(dwellTimer.current);
        if (effectsEnabled()) {
            setDwelling(true);
            dwellTimer.current = window.setTimeout(() => setDwelling(false), FLIP_DWELL_MS);
        }
        void hero.refetch();
    };

    const statItems = [
        { label: 'Images', value: stats.data?.totalImages },
        { label: 'Tags', value: stats.data?.totalTags },
        { label: 'Artists', value: stats.data?.totalArtists },
        { label: 'API requests served', value: stats.data?.totalRequests },
    ];

    return (
        <div className="relative isolate overflow-x-clip">
            <MetaTags />
            <section className="page">
                <div className="grid items-center gap-x-12 gap-y-14 lg:grid-cols-[minmax(0,1fr)_auto] xl:gap-x-20">
                    <div className="max-w-2xl">
                        <StatusSticker state={hero.isPending ? 'pending' : hero.isSuccess ? 'online' : 'offline'} />
                        <h1 className="mt-7 text-5xl font-black leading-[1.04] tracking-[-0.02em] sm:text-6xl xl:text-[4.5rem]">
                            The <span className="text-gradient-api">API</span> for your <span className="text-gradient-waifu">Waifu</span> content
                        </h1>
                        <p className="mt-6 max-w-xl text-lg text-muted-foreground">
                            Thousands of categorized anime illustrations. Query them over a REST API, or browse the collection right here.
                        </p>
                        <div className="mt-9 flex flex-wrap gap-3">
                            <Link to="/gallery" className="btn btn-primary btn-lg">
                                <LayoutGrid size={18} aria-hidden="true" /> Browse the gallery
                            </Link>
                            <ExternalLink href={docsUrl} trusted className="btn btn-secondary btn-lg">
                                <Book size={18} aria-hidden="true" /> API docs
                            </ExternalLink>
                        </div>
                    </div>

                    <div className="flex flex-col items-center gap-8 pb-2 lg:pr-4">
                        <HeroCard
                            image={image}
                            flipped={flipped}
                            broken={!!image && brokenId === image.id}
                            onLoaded={setLoadedId}
                            onBroken={setBrokenId}
                        />
                        <button type="button" data-sfx="draw" onClick={draw} disabled={drawing || hero.isPending} aria-busy={drawing} className="btn btn-accent btn-lg min-w-48">
                            {drawing ? <Loader2 size={18} className="animate-spin" aria-hidden="true" /> : <Shuffle size={18} aria-hidden="true" />}
                            Draw a card
                        </button>
                        <p className="sr-only" aria-live="polite">{draws > 0 && !flipped && image ? `Drew image #${image.id}` : ''}</p>
                    </div>
                </div>

                <figure className="panel mt-12 overflow-hidden lg:mt-10">
                    <figcaption className="flex items-center gap-3 border-b border-dashed border-border px-4 py-3 sm:px-5">
                        <span className="rounded-full bg-success/12 px-2 py-0.5 font-mono text-[0.6875rem] font-bold text-success">GET</span>
                        <code className="min-w-0 flex-1 truncate font-mono text-[0.8125rem] text-foreground" title={requestLine}>{requestLine}</code>
                        {hero.isSuccess && !hero.isFetching && (
                            <span className="hidden shrink-0 font-mono text-xs text-muted-foreground sm:block">
                                <span className="font-bold text-success">{hero.data.status} OK</span>
                                <span className="tabular"> · {hero.data.ms} ms</span>
                            </span>
                        )}
                    </figcaption>
                    <div className="grid gap-6 px-4 py-5 sm:px-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,20rem)]">
                        {hero.isPending ? (
                            <div className="space-y-2.5" aria-hidden="true">
                                <Skeleton className="h-3.5 w-24" />
                                <Skeleton className="h-3.5 w-4/5" />
                                <Skeleton className="h-3.5 w-3/5" />
                                <Skeleton className="h-3.5 w-2/5" />
                            </div>
                        ) : hero.isError ? (
                            <div>
                                <p className="font-bold">The API did not answer</p>
                                <p className="mt-1 text-sm text-muted-foreground">Check your connection, then draw a card to try again.</p>
                            </div>
                        ) : !image ? (
                            <div>
                                <p className="font-bold">No matching image yet</p>
                                <p className="mt-1 text-sm text-muted-foreground">Once images are uploaded, a random one appears here.</p>
                            </div>
                        ) : (
                            <>
                                <div key={`${image.id}-${draws}`} className="min-w-0">
                                    <ResponseBody image={image} />
                                </div>
                                {(image.tags ?? []).length > 0 && (
                                    <div className="flex flex-wrap content-start gap-2 lg:justify-end">
                                        {image.tags.slice(0, 8).map((tag) => (
                                            <TagChip key={tag.id} label={tag.name} href={`/gallery?includedTags=${encodeURIComponent(tag.slug)}`} />
                                        ))}
                                    </div>
                                )}
                            </>
                        )}
                    </div>
                </figure>

                {!stats.isError && (
                    <dl className="mt-12 grid grid-cols-2 gap-x-8 gap-y-6 sm:grid-cols-4">
                        {statItems.map(({ label, value }) => (
                            <div key={label} className="flex flex-col-reverse gap-0.5 border-t-2 border-dashed border-border pt-4">
                                <dt className="text-sm font-bold text-muted-foreground">{label}</dt>
                                <dd className="text-3xl font-black tabular">
                                    {stats.isPending ? <Skeleton className="mb-1 h-8 w-16" /> : formatCompactNumber(value ?? 0)}
                                </dd>
                            </div>
                        ))}
                    </dl>
                )}

                <NewSinceVisit />
                <TrendingRow />
                <RecentlyViewed />
                <PopularTags />
                <ColorRow />
            </section>
        </div>
    );
};

export default Home;
