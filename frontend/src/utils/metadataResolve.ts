import api from '../services/api';
import { PaginatedList, Tag, Artist } from '../types';

const BATCH_SIZE = 50;

function chunk<T>(values: T[], size: number): T[][] {
    const out: T[][] = [];
    for (let i = 0; i < values.length; i += size) out.push(values.slice(i, i + size));
    return out;
}

async function resolveInBatches<T, V extends string | number>(
    endpoint: string,
    param: string,
    values: V[]
): Promise<T[]> {
    const unique = [...new Set(values)];
    if (unique.length === 0) return [];
    const parts = await Promise.all(
        chunk(unique, BATCH_SIZE).map(async (part) => {
            const params = new URLSearchParams({ pageSize: String(part.length) });
            part.forEach((v) => params.append(param, String(v)));
            try {
                const { data } = await api.get<PaginatedList<T>>(endpoint, { params, skipGlobalErrorHandler: true });
                return data.items;
            } catch {
                return [];
            }
        })
    );
    return parts.flat();
}

export function resolveTagsBySlugs(slugs: string[]): Promise<Tag[]> {
    return resolveInBatches<Tag, string>('/tags', 'IncludedSlugs', slugs);
}

export function resolveArtistsByIds(ids: number[]): Promise<Artist[]> {
    return resolveInBatches<Artist, number>('/artists', 'IncludedIds', ids);
}
