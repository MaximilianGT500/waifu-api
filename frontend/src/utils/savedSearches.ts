import { safeRedirectPath } from './safeUrl';

const KEY = 'saved-searches';
const MAX = 30;
const MAX_NAME = 80;

const QUERY_PARAMS = [
    'isNsfw', 'orientation', 'isAnimated', 'width', 'height', 'byteSize',
    'includedTags', 'excludedTags', 'includedArtists', 'excludedArtists',
    'includedIds', 'excludedIds', 'uploaderId', 'reviewStatus', 'orderBy',
    'color', 'colorTolerance', 'since', 'uploadedAfter', 'uploadedBefore',
];

export interface SavedSearch {
    id: string;
    name: string;
    path: string;
    query: string;
}

let cache: SavedSearch[] | null = null;
const listeners = new Set<() => void>();

const notify = () => listeners.forEach((listener) => listener());

function load(): SavedSearch[] {
    try {
        const parsed = JSON.parse(localStorage.getItem(KEY) || '[]') as unknown;
        if (!Array.isArray(parsed)) return [];
        return (parsed as Partial<SavedSearch>[])
            .filter((entry) => typeof entry?.id === 'string' && typeof entry.name === 'string' && typeof entry.query === 'string')
            .map((entry) => ({
                id: entry.id as string,
                name: (entry.name as string).slice(0, MAX_NAME),
                path: safeRedirectPath(typeof entry.path === 'string' ? entry.path : '/gallery', '/gallery').split('?')[0],
                query: new URLSearchParams(entry.query as string).toString(),
            }))
            .slice(0, MAX);
    } catch {
        return [];
    }
}

function save(next: SavedSearch[]) {
    cache = next;
    try {
        if (next.length === 0) localStorage.removeItem(KEY);
        else localStorage.setItem(KEY, JSON.stringify(next));
    } catch { }
    notify();
}

export function getSavedSearches(): SavedSearch[] {
    if (cache === null) cache = load();
    return cache;
}

export function subscribeSavedSearches(listener: () => void): () => void {
    listeners.add(listener);
    const onStorage = (event: StorageEvent) => {
        if (event.key !== KEY && event.key !== null) return;
        cache = null;
        listener();
    };
    window.addEventListener('storage', onStorage);
    return () => {
        listeners.delete(listener);
        window.removeEventListener('storage', onStorage);
    };
}

export function searchQueryOf(searchParams: URLSearchParams): string {
    const out = new URLSearchParams();
    for (const key of QUERY_PARAMS) {
        for (const value of searchParams.getAll(key)) if (value !== '') out.append(key, value);
    }
    return out.toString();
}

export const savedSearchHref = (search: SavedSearch) => (search.query ? `${search.path}?${search.query}` : search.path);

export function addSavedSearch(name: string, path: string, query: string): SavedSearch {
    const entry: SavedSearch = {
        id: `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`,
        name: name.trim().slice(0, MAX_NAME) || 'Saved search',
        path: safeRedirectPath(path, '/gallery').split('?')[0],
        query,
    };
    const rest = getSavedSearches().filter((item) => !(item.path === entry.path && item.query === entry.query));
    save([entry, ...rest].slice(0, MAX));
    return entry;
}

export function renameSavedSearch(id: string, name: string): void {
    const trimmed = name.trim().slice(0, MAX_NAME);
    if (!trimmed) return;
    save(getSavedSearches().map((item) => (item.id === id ? { ...item, name: trimmed } : item)));
}

export function removeSavedSearch(id: string): void {
    save(getSavedSearches().filter((item) => item.id !== id));
}

export function clearSavedSearches(): void {
    save([]);
}
