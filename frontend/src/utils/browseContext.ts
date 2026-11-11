import api from '../services/api';
import { ImageDto, PaginatedList } from '../types';
import { safeRedirectPath } from './safeUrl';

const KEY = 'browse-context';
const MAX_IDS = 2000;
const SAFE_PATH = /^\/[A-Za-z0-9/_-]{1,200}$/;

export interface BrowseSource {
    path: string;
    params: string;
    firstPage: number;
    lastPage: number;
    totalPages: number;
    pageSize: number;
    totalCount?: number;
}

export interface BrowseContext {
    ids: number[];
    from: string;
    source?: BrowseSource;
}

export interface Neighbours {
    index: number;
    position: number;
    total: number;
    prev?: number;
    next?: number;
    morePrev: boolean;
    moreNext: boolean;
}

let cache: BrowseContext | null | undefined;
const listeners = new Set<() => void>();
const inflight = new Map<string, Promise<void>>();

const positiveInt = (value: unknown) => (typeof value === 'number' && Number.isSafeInteger(value) && value > 0 ? value : undefined);

function readSource(value: unknown): BrowseSource | undefined {
    if (!value || typeof value !== 'object') return undefined;
    const s = value as Record<string, unknown>;
    const firstPage = positiveInt(s.firstPage);
    const lastPage = positiveInt(s.lastPage);
    const totalPages = positiveInt(s.totalPages);
    const pageSize = positiveInt(s.pageSize);
    if (typeof s.path !== 'string' || !SAFE_PATH.test(s.path) || s.path.includes('..') || typeof s.params !== 'string') return undefined;
    if (!firstPage || !lastPage || !totalPages || !pageSize || firstPage > lastPage) return undefined;
    const totalCount = typeof s.totalCount === 'number' && Number.isSafeInteger(s.totalCount) && s.totalCount >= 0 ? s.totalCount : undefined;
    return { path: s.path, params: s.params, firstPage, lastPage, totalPages, pageSize, totalCount };
}

function load(): BrowseContext | null {
    try {
        const raw = sessionStorage.getItem(KEY);
        if (!raw) return null;
        const value = JSON.parse(raw) as { ids?: unknown; from?: unknown; source?: unknown };
        if (!Array.isArray(value.ids) || typeof value.from !== 'string') return null;
        const ids = value.ids.filter((id): id is number => Number.isSafeInteger(id) && id > 0).slice(0, MAX_IDS);
        return { ids, from: safeRedirectPath(value.from, '/gallery'), source: readSource(value.source) };
    } catch {
        return null;
    }
}

export function getBrowseContext(): BrowseContext | null {
    if (cache === undefined) cache = load();
    return cache;
}

export function subscribeBrowseContext(listener: () => void): () => void {
    listeners.add(listener);
    return () => {
        listeners.delete(listener);
    };
}

export function saveBrowseContext(context: BrowseContext): void {
    const params = context.source ? new URLSearchParams(context.source.params) : null;
    params?.delete('page');
    cache = {
        ids: context.ids.slice(0, MAX_IDS),
        from: context.from,
        source: context.source && params ? { ...context.source, params: params.toString() } : undefined,
    };
    try {
        sessionStorage.setItem(KEY, JSON.stringify(cache));
    } catch { }
    listeners.forEach((listener) => listener());
}

export function neighboursOf(context: BrowseContext | null, id: number): Neighbours | null {
    if (!context) return null;
    const index = context.ids.indexOf(id);
    if (index === -1) return null;
    const source = context.source;
    const morePrev = !!source && source.firstPage > 1;
    const moreNext = !!source && source.lastPage < source.totalPages && context.ids.length < MAX_IDS;
    if (context.ids.length < 2 && !morePrev && !moreNext) return null;
    const offset = source ? (source.firstPage - 1) * source.pageSize : 0;
    return {
        index,
        position: offset + index + 1,
        total: Math.max(source?.totalCount ?? context.ids.length, offset + context.ids.length),
        prev: context.ids[index - 1],
        next: context.ids[index + 1],
        morePrev,
        moreNext,
    };
}

export function extendBrowseContext(direction: 'next' | 'prev'): Promise<void> {
    const context = getBrowseContext();
    const source = context?.source;
    if (!context || !source) return Promise.resolve();
    const page = direction === 'next' ? source.lastPage + 1 : source.firstPage - 1;
    if (page < 1 || page > source.totalPages || (direction === 'next' && context.ids.length >= MAX_IDS)) return Promise.resolve();
    const key = `${source.path}?${source.params}&page=${page}`;
    const running = inflight.get(key);
    if (running) return running;

    const params = new URLSearchParams(source.params);
    params.set('page', String(page));
    const request = api.get<PaginatedList<ImageDto>>(source.path, { params, skipGlobalErrorHandler: true })
        .then(({ data }) => {
            const current = getBrowseContext();
            if (!current?.source || current.source.path !== source.path || current.source.params !== source.params) return;
            const known = new Set(current.ids);
            const fresh = data.items.map((item) => item.id).filter((id) => !known.has(id));
            saveBrowseContext({
                ...current,
                ids: direction === 'next' ? [...current.ids, ...fresh] : [...fresh, ...current.ids],
                source: {
                    ...current.source,
                    firstPage: direction === 'prev' ? page : current.source.firstPage,
                    lastPage: direction === 'next' ? page : current.source.lastPage,
                    totalPages: Math.max(data.totalPages, 1),
                    totalCount: data.totalCount,
                },
            });
        })
        .catch(() => undefined)
        .finally(() => inflight.delete(key));
    inflight.set(key, request);
    return request;
}
