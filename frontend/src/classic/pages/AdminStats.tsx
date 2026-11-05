import { useMemo, useRef, useState, KeyboardEvent, PointerEvent, ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { Key, Globe } from 'lucide-react';
import api from '../../services/api';
import { useRequireAuth } from '../../hooks/useRequireAuth';
import { Role } from '../../types';

interface DailyStat {
    date: string;
    count: number;
}

interface UserStat {
    id: number;
    name: string;
    requestCount: number;
    apiKeyRequestCount: number;
    jwtRequestCount: number;
}

interface UploaderStat {
    id: number;
    name: string;
    uploadedImageCount: number;
}

interface AlbumUserStat {
    id: number;
    name: string;
    albumImageCount: number;
}

interface AdminStatsDto {
    requestsToday: number;
    history: DailyStat[];
    topUsers: UserStat[];
    topApiKeyUsers: UserStat[];
    topJwtUsers: UserStat[];
    topUploaders: UploaderStat[];
    topAlbumUsers: AlbumUserStat[];
    totalRequests: number;
    totalAuthenticatedRequests: number;
    totalUnauthenticatedRequests: number;
    totalApiKeyRequests: number;
    totalJwtRequests: number;
    totalImages: number;
    totalTags: number;
    totalArtists: number;
}

const DAY_MS = 86_400_000;
const HISTORY_DAYS = 366;

const utcDate = (iso: string, options: Intl.DateTimeFormatOptions = {}) =>
    new Date(`${iso}T00:00:00Z`).toLocaleDateString(undefined, { timeZone: 'UTC', ...options });

const compact = (n: number) => new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(n);

function fillHistory(history: DailyStat[]): DailyStat[] {
    const counts = new Map(history.map((d) => [d.date.slice(0, 10), d.count]));
    const today = new Date();
    const end = Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate());
    const days: DailyStat[] = [];
    for (let i = HISTORY_DAYS - 1; i >= 0; i--) {
        const date = new Date(end - i * DAY_MS).toISOString().slice(0, 10);
        days.push({ date, count: counts.get(date) ?? 0 });
    }
    return days;
}

function niceMax(value: number): number {
    if (value <= 4) return 4;
    const magnitude = Math.pow(10, Math.floor(Math.log10(value)));
    const steps = [1, 2, 2.5, 5, 10];
    const normalized = value / magnitude;
    const step = steps.find((s) => normalized <= s) ?? 10;
    return step * magnitude;
}

const RequestChart = ({ days }: { days: DailyStat[] }) => {
    const [active, setActive] = useState<number | null>(null);
    const plotRef = useRef<HTMLDivElement>(null);
    const max = niceMax(Math.max(...days.map((d) => d.count), 0));
    const ticks = [max, max * 0.75, max * 0.5, max * 0.25, 0];
    const x = (i: number) => (days.length <= 1 ? 50 : (i / (days.length - 1)) * 100);
    const y = (count: number) => 100 - (count / max) * 100;
    const line = days.map((d, i) => `${x(i).toFixed(3)},${y(d.count).toFixed(3)}`).join(' ');
    const area = `0,100 ${line} 100,100`;
    const months = days
        .map((d, i) => ({ d, i }))
        .filter(({ d, i }) => d.date.endsWith('-01') && i > 6 && i < days.length - 6);

    const pick = (e: PointerEvent<HTMLDivElement>) => {
        const rect = plotRef.current?.getBoundingClientRect();
        if (!rect || rect.width === 0) return;
        const ratio = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
        setActive(Math.round(ratio * (days.length - 1)));
    };

    const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
        const last = days.length - 1;
        const current = active ?? last;
        const next = e.key === 'ArrowLeft' ? current - 1
            : e.key === 'ArrowRight' ? current + 1
                : e.key === 'Home' ? 0
                    : e.key === 'End' ? last
                        : e.key === 'PageUp' ? current - 30
                            : e.key === 'PageDown' ? current + 30 : null;
        if (next === null) return;
        e.preventDefault();
        setActive(Math.min(last, Math.max(0, next)));
    };

    const point = active !== null ? days[active] : null;

    return (
        <div className="relative">
            <div className="flex">
                <div className="relative h-72 w-10 shrink-0 select-none text-right text-[0.6875rem] text-muted-foreground tabular sm:h-80" aria-hidden="true">
                    {ticks.map((t, i) => (
                        <span key={i} className="absolute right-2 -translate-y-1/2" style={{ top: `${(i / (ticks.length - 1)) * 100}%` }}>{compact(t)}</span>
                    ))}
                </div>
                <div
                    ref={plotRef}
                    className="relative h-72 min-w-0 flex-1 cursor-crosshair rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-4 focus-visible:ring-offset-card sm:h-80"
                    tabIndex={0}
                    role="slider"
                    aria-label="Requests per day"
                    aria-valuemin={0}
                    aria-valuemax={days.length - 1}
                    aria-valuenow={active ?? days.length - 1}
                    aria-valuetext={`${utcDate((point ?? days[days.length - 1]).date)}: ${(point ?? days[days.length - 1]).count.toLocaleString()} requests`}
                    onPointerMove={pick}
                    onPointerDown={pick}
                    onPointerLeave={() => setActive(null)}
                    onKeyDown={onKeyDown}
                    onBlur={() => setActive(null)}
                >
                    {ticks.map((_, i) => (
                        <div key={i} className="pointer-events-none absolute inset-x-0 h-px bg-border" style={{ top: `${(i / (ticks.length - 1)) * 100}%` }} aria-hidden="true" />
                    ))}
                    <svg className="absolute inset-0 size-full overflow-visible text-chart-line" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
                        <polygon points={area} fill="currentColor" fillOpacity="0.1" />
                        <polyline points={line} fill="none" stroke="currentColor" strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    {point && active !== null && (
                        <>
                            <div className="pointer-events-none absolute inset-y-0 w-px bg-foreground/30" style={{ left: `${x(active)}%` }} aria-hidden="true" />
                            <div
                                className="pointer-events-none absolute size-3 -translate-x-1/2 -translate-y-1/2 rounded-full bg-chart-line ring-2 ring-card"
                                style={{ left: `${x(active)}%`, top: `${y(point.count)}%` }}
                                aria-hidden="true"
                            />
                            <div
                                className={`pointer-events-none absolute top-2 z-10 whitespace-nowrap rounded-xl border border-border bg-popover px-3 py-2 text-popover-foreground shadow-float ${x(active) > 70 ? '-translate-x-[calc(100%+0.75rem)]' : 'translate-x-3'}`}
                                style={{ left: `${x(active)}%` }}
                            >
                                <p className="text-base font-bold tabular">{point.count.toLocaleString()} <span className="text-xs font-medium text-muted-foreground">requests</span></p>
                                <p className="text-xs text-muted-foreground">{utcDate(point.date, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' })}</p>
                            </div>
                        </>
                    )}
                </div>
            </div>
            <div className="relative ml-10 mt-2 h-4 select-none text-[0.6875rem] text-muted-foreground" aria-hidden="true">
                {months.map(({ d, i }) => (
                    <span key={d.date} className="absolute -translate-x-1/2" style={{ left: `${x(i)}%` }}>{utcDate(d.date, { month: 'short' })}</span>
                ))}
            </div>
        </div>
    );
};

const Leaderboard = ({ title, rows, empty = 'No data yet.' }: {
    title: string;
    rows: { id: number; name: string; value: number; detail?: ReactNode }[];
    empty?: string;
}) => (
    <section className="panel p-5">
        <h2 className="section-title">{title}</h2>
        {rows.length === 0 ? (
            <p className="mt-4 text-sm text-muted-foreground">{empty}</p>
        ) : (
            <ol className="mt-3 divide-y divide-border">
                {rows.map((row, i) => (
                    <li key={row.id} className="flex items-center gap-3 py-2.5">
                        <span className="w-5 shrink-0 text-right text-sm text-muted-foreground tabular">{i + 1}</span>
                        <div className="min-w-0 flex-1">
                            <Link to={`/users?includedIds=${row.id}`} className="block truncate font-medium hover:underline" title={row.name}>{row.name}</Link>
                            {row.detail}
                        </div>
                        <span className="shrink-0 text-sm font-semibold tabular">{row.value.toLocaleString()}</span>
                    </li>
                ))}
            </ol>
        )}
    </section>
);

const AdminStats = () => {
    const user = useRequireAuth();
    const isAdmin = user?.role === Role.Admin;
    const statsQuery = useQuery({
        queryKey: ['stats', 'admin'],
        enabled: !!isAdmin,
        queryFn: async () => (await api.get<AdminStatsDto>('/stats/admin')).data,
    });
    const stats = statsQuery.data;
    const days = useMemo(() => fillHistory(stats?.history ?? []), [stats?.history]);

    if (!user) return null;
    if (!isAdmin) return <div className="page text-center text-muted-foreground">Only administrators can see the statistics.</div>;
    if (statsQuery.isPending) return <div className="page text-center text-muted-foreground" role="status">Loading statistics…</div>;
    if (statsQuery.isError || !stats) {
        return (
            <div className="page flex flex-col items-center text-center" role="alert">
                <p className="font-semibold">The statistics could not be loaded</p>
                <button type="button" onClick={() => statsQuery.refetch()} className="btn btn-secondary mt-4">Try again</button>
            </div>
        );
    }

    const yearTotal = days.reduce((acc, d) => acc + d.count, 0);
    const authTotal = stats.totalAuthenticatedRequests;
    const anonTotal = stats.totalUnauthenticatedRequests;
    const trafficTotal = Math.max(1, authTotal + anonTotal);
    const monthly = Object.entries(days.reduce<Record<string, number>>((acc, d) => {
        const month = d.date.slice(0, 7);
        acc[month] = (acc[month] ?? 0) + d.count;
        return acc;
    }, {})).toReversed();

    const tiles = [
        { label: 'Requests today', value: stats.requestsToday },
        { label: 'Images', value: stats.totalImages },
        { label: 'Tags', value: stats.totalTags },
        { label: 'Artists', value: stats.totalArtists },
    ];

    const traffic = [
        { label: 'Authenticated', value: authTotal, children: [
            { label: 'API key', value: stats.totalApiKeyRequests, icon: Key },
            { label: 'Web session', value: stats.totalJwtRequests, icon: Globe },
        ] },
        { label: 'Anonymous', value: anonTotal, children: [] },
    ];

    return (
        <div className="page">
            <div className="page-header">
                <div>
                    <h1 className="page-title">Statistics</h1>
                    <p className="page-lede">Traffic and activity across the whole API. Days are counted in UTC.</p>
                </div>
            </div>

            <dl className="mb-6 grid grid-cols-2 gap-4 xl:grid-cols-4">
                {tiles.map((tile) => (
                    <div key={tile.label} className="panel flex flex-col-reverse p-5">
                        <dt className="mt-1 text-sm text-muted-foreground">{tile.label}</dt>
                        <dd className="truncate text-3xl font-bold tracking-[-0.02em]" title={tile.value.toLocaleString()}>{tile.value.toLocaleString()}</dd>
                    </div>
                ))}
            </dl>

            <section className="panel mb-6 p-5 sm:p-6">
                <div className="mb-6 flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
                    <div>
                        <h2 className="section-title">Requests per day</h2>
                        <p className="text-sm text-muted-foreground">Last 12 months</p>
                    </div>
                    <dl className="flex gap-6 text-sm">
                        <div>
                            <dt className="text-muted-foreground">Last 12 months</dt>
                            <dd className="font-bold">{yearTotal.toLocaleString()}</dd>
                        </div>
                        <div>
                            <dt className="text-muted-foreground">All time</dt>
                            <dd className="font-bold">{stats.totalRequests.toLocaleString()}</dd>
                        </div>
                    </dl>
                </div>
                <RequestChart days={days} />
                <details className="mt-5 border-t border-border pt-4">
                    <summary className="cursor-pointer text-sm font-semibold text-muted-foreground hover:text-foreground">Monthly totals as a table</summary>
                    <table className="mt-3 w-full max-w-md text-sm">
                        <thead>
                            <tr className="border-b border-border text-left text-xs text-muted-foreground">
                                <th scope="col" className="py-2 font-semibold">Month</th>
                                <th scope="col" className="py-2 text-right font-semibold">Requests</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                            {monthly.map(([month, count]) => (
                                <tr key={month}>
                                    <td className="py-1.5">{utcDate(`${month}-01`, { year: 'numeric', month: 'long' })}</td>
                                    <td className="py-1.5 text-right tabular">{count.toLocaleString()}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </details>
            </section>

            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                <section className="panel p-5">
                    <h2 className="section-title">Traffic source</h2>
                    <div className="mt-4 flex h-2 overflow-hidden rounded-full bg-secondary" aria-hidden="true">
                        <div className="h-full bg-chart-line" style={{ width: `${(authTotal / trafficTotal) * 100}%` }} />
                    </div>
                    <dl className="mt-4 space-y-3 text-sm">
                        {traffic.map((row) => (
                            <div key={row.label}>
                                <div className="flex items-center justify-between gap-3">
                                    <dt className="flex items-center gap-2 font-medium">
                                        <span className={`size-2.5 rounded-full ${row.label === 'Authenticated' ? 'bg-chart-line' : 'bg-secondary ring-1 ring-inset ring-border'}`} aria-hidden="true" />
                                        {row.label}
                                    </dt>
                                    <dd className="font-semibold tabular">
                                        {row.value.toLocaleString()} <span className="font-normal text-muted-foreground">({Math.round((row.value / trafficTotal) * 100)}%)</span>
                                    </dd>
                                </div>
                                {row.children.length > 0 && (
                                    <dl className="ml-4 mt-2 space-y-1.5 border-l border-border pl-4">
                                        {row.children.map(({ label, value, icon: Icon }) => (
                                            <div key={label} className="flex items-center justify-between gap-3 text-muted-foreground">
                                                <dt className="flex items-center gap-2"><Icon size={13} aria-hidden="true" />{label}</dt>
                                                <dd className="tabular">{value.toLocaleString()}</dd>
                                            </div>
                                        ))}
                                    </dl>
                                )}
                            </div>
                        ))}
                    </dl>
                </section>

                <Leaderboard
                    title="Top users"
                    empty="No user data yet."
                    rows={stats.topUsers.map((u) => ({
                        id: u.id,
                        name: u.name,
                        value: u.requestCount,
                        detail: (
                            <span className="flex gap-3 text-xs text-muted-foreground tabular">
                                <span className="flex items-center gap-1" title="API key requests"><Key size={11} aria-hidden="true" />{u.apiKeyRequestCount.toLocaleString()}</span>
                                <span className="flex items-center gap-1" title="Web requests"><Globe size={11} aria-hidden="true" />{u.jwtRequestCount.toLocaleString()}</span>
                            </span>
                        ),
                    }))}
                />
                <Leaderboard title="Top API key users" rows={stats.topApiKeyUsers.map((u) => ({ id: u.id, name: u.name, value: u.apiKeyRequestCount }))} />
                <Leaderboard title="Top web users" rows={stats.topJwtUsers.map((u) => ({ id: u.id, name: u.name, value: u.jwtRequestCount }))} />
                <Leaderboard title="Top uploaders" rows={stats.topUploaders.map((u) => ({ id: u.id, name: u.name, value: u.uploadedImageCount }))} />
                <Leaderboard title="Most images in albums" rows={stats.topAlbumUsers.map((u) => ({ id: u.id, name: u.name, value: u.albumImageCount }))} />
            </div>
        </div>
    );
};

export default AdminStats;
