import { ImageDto } from '../types';
import { safeColor } from './color';
import { safeExternalUrl } from './safeUrl';

const KEY = 'recently-viewed';
const MAX = 24;

export interface RecentImage {
    id: number;
    url: string;
    dominantColor: string | null;
    width: number;
    height: number;
    isNsfw: boolean;
    isAnimated: boolean;
}

let cache: RecentImage[] | null = null;
const listeners = new Set<() => void>();

const notify = () => listeners.forEach((listener) => listener());

function load(): RecentImage[] {
    try {
        const parsed = JSON.parse(localStorage.getItem(KEY) || '[]') as unknown;
        if (!Array.isArray(parsed)) return [];
        const out: RecentImage[] = [];
        for (const entry of parsed as Partial<RecentImage>[]) {
            const url = safeExternalUrl(entry?.url);
            if (!url || !Number.isSafeInteger(entry.id) || (entry.id as number) <= 0) continue;
            out.push({
                id: entry.id as number,
                url,
                dominantColor: safeColor(entry.dominantColor),
                width: Number(entry.width) || 0,
                height: Number(entry.height) || 0,
                isNsfw: entry.isNsfw === true,
                isAnimated: entry.isAnimated === true,
            });
        }
        return out.slice(0, MAX);
    } catch {
        return [];
    }
}

function save(next: RecentImage[]) {
    cache = next;
    try {
        if (next.length === 0) localStorage.removeItem(KEY);
        else localStorage.setItem(KEY, JSON.stringify(next));
    } catch {
    }
    notify();
}

export function getRecentImages(): RecentImage[] {
    if (cache === null) cache = load();
    return cache;
}

export function subscribeRecentImages(listener: () => void): () => void {
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

export function addRecentImage(image: ImageDto): void {
    const url = safeExternalUrl(image.url);
    if (!url) return;
    const current = getRecentImages();
    if (current[0]?.id === image.id && current[0].url === url) return;
    const entry: RecentImage = {
        id: image.id,
        url,
        dominantColor: safeColor(image.dominantColor),
        width: image.width,
        height: image.height,
        isNsfw: image.isNsfw,
        isAnimated: image.isAnimated,
    };
    save([entry, ...current.filter((item) => item.id !== image.id)].slice(0, MAX));
}

export function removeRecentImage(id: number): void {
    save(getRecentImages().filter((item) => item.id !== id));
}

export function clearRecentImages(): void {
    save([]);
}
