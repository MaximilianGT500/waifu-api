import { Link, useNavigate } from 'react-router';
import { Book, Shuffle, RotateCw, Loader2, ImageOff } from 'lucide-react';
import { CSSProperties, ReactNode, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import api from '../../services/api';
import { ImageDto, PaginatedList, NsfwMode, Orientation, AnimatedMode, ImageOrderBy } from '../../types';
import { getEnv } from '../../utils/env';
import Skeleton from '../components/Skeleton';
import { MetaTags } from '../../hooks/useMetaTags';
import { imgPreview } from '../../utils/cfImage';
import ExternalLink from '../components/ExternalLink';
import TagChip from '../components/TagChip';
import { safeColor } from '../../utils/color';

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
    orientation: Orientation.Landscape,
    isAnimated: AnimatedMode.False,
    width: '>=1000',
    orderBy: ImageOrderBy.Random,
    pageSize: '1',
};

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

const ApiPreview = ({ query, onRunAgain }: { query: ReturnType<typeof useHero>; onRunAgain: () => void }) => {
    const [loadedId, setLoadedId] = useState<number | null>(null);
    const [brokenId, setBrokenId] = useState<number | null>(null);
    const { data, isPending, isError, isFetching } = query;
    const image = data?.image ?? null;
    const requestLine = `${displayBase()}/images?${Object.entries(HERO_QUERY).map(([k, v]) => `${k}=${v}`).join('&')}`;
    const color = image ? safeColor(image.dominantColor) : null;
    const artistNames = image ? (image.artists ?? []).map((a) => a.name).join(', ') : '';

    return (
        <figure className="relative w-full min-w-0 overflow-hidden rounded-[1.5rem] border border-border bg-card shadow-float">
            <figcaption className="flex items-center gap-3 border-b border-border bg-secondary/45 py-2.5 pl-4 pr-2.5">
                <span className="rounded-md bg-success/12 px-1.5 py-0.5 font-mono text-[0.6875rem] font-semibold text-success">GET</span>
                <code className="min-w-0 flex-1 truncate font-mono text-[0.8125rem] text-foreground" title={requestLine}>{requestLine}</code>
                <button
                    type="button"
                    onClick={onRunAgain}
                    disabled={isFetching}
                    className="grid size-8 shrink-0 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-background hover:text-foreground disabled:opacity-60"
                    aria-label="Send the request again"
                    title="Send the request again"
                >
                    <RotateCw size={15} aria-hidden="true" className={isFetching ? 'animate-spin' : ''} />
                </button>
            </figcaption>

            <div className="p-3 sm:p-4">
                {isPending ? (
                    <div aria-hidden="true">
                        <Skeleton className="aspect-[16/10] w-full rounded-2xl" />
                        <div className="mt-4 space-y-2.5 px-1">
                            <Skeleton className="h-3.5 w-24" />
                            <Skeleton className="h-3.5 w-4/5" />
                            <Skeleton className="h-3.5 w-3/5" />
                        </div>
                    </div>
                ) : isError ? (
                    <div className="flex aspect-[16/10] flex-col items-center justify-center gap-3 rounded-2xl bg-secondary/60 px-6 text-center">
                        <p className="font-semibold">The API did not answer</p>
                        <p className="max-w-xs text-sm text-muted-foreground">Check your connection or try again in a moment.</p>
                        <button type="button" onClick={onRunAgain} className="btn btn-secondary btn-sm">Try again</button>
                    </div>
                ) : !image ? (
                    <div className="flex aspect-[16/10] flex-col items-center justify-center gap-2 rounded-2xl bg-secondary/60 px-6 text-center">
                        <p className="font-semibold">No matching image yet</p>
                        <p className="max-w-xs text-sm text-muted-foreground">Once images are uploaded, a random one appears here.</p>
                    </div>
                ) : (
                    <>
                        <Link
                            to={`/images/${image.id}`}
                            className="group relative block overflow-hidden rounded-2xl"
                            style={{ aspectRatio: `${image.width} / ${image.height}`, maxHeight: '22rem', backgroundColor: color ?? undefined }}
                        >
                            {brokenId === image.id ? (
                                <span className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-secondary text-sm text-muted-foreground">
                                    <ImageOff size={22} aria-hidden="true" /> Image unavailable
                                </span>
                            ) : (
                                <img
                                    key={image.id}
                                    src={imgPreview(image.url)}
                                    alt={artistNames ? `Image #${image.id} by ${artistNames}` : `Image #${image.id}`}
                                    width={image.width}
                                    height={image.height}
                                    decoding="async"
                                    fetchPriority="high"
                                    onLoad={() => setLoadedId(image.id)}
                                    onError={() => setBrokenId(image.id)}
                                    className={`size-full object-cover transition-opacity duration-500 ${loadedId === image.id ? 'response-image opacity-100' : 'opacity-0'}`}
                                />
                            )}
                        </Link>
                        <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 px-1 font-mono text-xs text-muted-foreground">
                            <span className="font-semibold text-success">{data.status} OK</span>
                            <span className="tabular">{data.ms} ms</span>
                            <span>items[0]</span>
                        </div>
                        <div key={image.id} className="mt-2 px-1">
                            <ResponseBody image={image} />
                        </div>
                        {(image.tags ?? []).length > 0 && (
                            <div className="mt-3 flex flex-wrap gap-1.5 px-1">
                                {image.tags.slice(0, 6).map((tag) => (
                                    <TagChip key={tag.id} label={tag.name} href={`/gallery?includedTags=${encodeURIComponent(tag.slug)}`} />
                                ))}
                            </div>
                        )}
                    </>
                )}
            </div>
        </figure>
    );
};

const ctaSize = 'px-4 py-3 text-[0.9375rem] sm:rounded-2xl sm:px-6 sm:py-3.5 sm:text-base';

const useHero = () => useQuery({ queryKey: ['home-hero'], queryFn: fetchHero, staleTime: Infinity, gcTime: 5 * 60_000 });

const Home = () => {
    const navigate = useNavigate();
    const [isSurprising, setIsSurprising] = useState(false);
    const hero = useHero();
    const stats = useQuery({
        queryKey: ['public-stats'],
        queryFn: async () => (await api.get<PublicStats>('/stats/public', { skipGlobalErrorHandler: true })).data,
        staleTime: 60_000,
    });

    const docsUrl = getEnv('VITE_DOCS_URL');

    const surpriseMe = async () => {
        if (isSurprising) return;
        setIsSurprising(true);
        try {
            const { data } = await api.get<PaginatedList<ImageDto>>('/images', {
                params: { isNsfw: NsfwMode.False, pageSize: 1, orderBy: ImageOrderBy.Random },
                skipGlobalErrorHandler: true,
            });
            if (data.items.length > 0) navigate(`/images/${data.items[0].id}`);
        } catch {
            return;
        } finally {
            setIsSurprising(false);
        }
    };

    const statItems = [
        { label: 'Images', value: stats.data?.totalImages },
        { label: 'Tags', value: stats.data?.totalTags },
        { label: 'Artists', value: stats.data?.totalArtists },
        { label: 'API requests served', value: stats.data?.totalRequests },
    ];

    return (
        <div className="relative isolate">
            <MetaTags />
            <section className="page grid items-center gap-12 lg:min-h-[calc(100dvh-4rem)] lg:grid-cols-[minmax(0,1.05fr)_minmax(0,1fr)] lg:gap-14 xl:gap-20">
                <div className="max-w-2xl animate-in fade-in slide-in-from-bottom-4 duration-700">
                    <p className="inline-flex min-h-7 items-center gap-2 rounded-full border border-border bg-card py-1 pl-2.5 pr-3 text-xs font-semibold text-muted-foreground">
                        {hero.isPending ? (
                            <>
                                <Skeleton className="size-2 rounded-full" />
                                <Skeleton className="h-3 w-16 rounded-sm" />
                            </>
                        ) : hero.isSuccess ? (
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

                    <h1 className="mt-6 text-5xl font-extrabold tracking-[-0.035em] text-foreground sm:text-6xl">
                        The <span className="text-gradient-api">API</span> for your <span className="text-gradient-waifu">Waifu</span> content
                    </h1>

                    <p className="mt-6 max-w-xl text-lg text-muted-foreground">
                        Thousands of categorized anime illustrations. Query them over a REST API, or browse the collection right here.
                    </p>

                    <div className="mt-9 flex flex-wrap gap-3">
                        <Link to="/gallery" className={`btn btn-primary ${ctaSize}`}>Browse the gallery</Link>
                        <button type="button" onClick={surpriseMe} disabled={isSurprising} className={`btn btn-secondary ${ctaSize}`}>
                            {isSurprising ? <Loader2 size={18} className="animate-spin" aria-hidden="true" /> : <Shuffle size={18} aria-hidden="true" />}
                            Surprise me
                        </button>
                        <ExternalLink href={docsUrl} trusted className={`btn btn-ghost ${ctaSize}`}>
                            <Book size={18} aria-hidden="true" /> API docs
                        </ExternalLink>
                    </div>

                    {!stats.isError && (
                        <dl className="mt-12 grid grid-cols-2 gap-x-8 gap-y-5 border-t border-border pt-8 sm:flex sm:flex-wrap sm:gap-x-10">
                            {statItems.map(({ label, value }) => (
                                <div key={label} className="flex flex-col-reverse">
                                    <dt className="text-sm text-muted-foreground">{label}</dt>
                                    <dd className="text-2xl font-extrabold tracking-[-0.02em]">
                                        {stats.isPending ? <Skeleton className="mb-1 h-7 w-14" /> : formatCompactNumber(value ?? 0)}
                                    </dd>
                                </div>
                            ))}
                        </dl>
                    )}
                </div>

                <ApiPreview query={hero} onRunAgain={() => hero.refetch()} />
            </section>
        </div>
    );
};

export default Home;
