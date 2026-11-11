import { QueryClient, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '../services/api';
import { ReviewStatus, UserMinimal } from '../types';
import { resolveArtistsByIds, resolveTagsBySlugs } from '../utils/metadataResolve';

export type LabelKind = 'tag' | 'artist' | 'user';

export type FilterLabel = {
    id: number | string;
    name: string;
    slug?: string;
    reviewStatus?: ReviewStatus;
};

const labelKey = (kind: LabelKind, key: string) => ['label', kind, key] as const;

const configured = new WeakSet<QueryClient>();

const configure = (queryClient: QueryClient) => {
    if (configured.has(queryClient)) return;
    configured.add(queryClient);
    queryClient.setQueryDefaults(['label'], { gcTime: Infinity, staleTime: Infinity });
};

export const rememberLabel = (queryClient: QueryClient, kind: LabelKind, key: string, label: FilterLabel) => {
    configure(queryClient);
    queryClient.setQueryData(labelKey(kind, key), label);
};

const fetchUsers = async (ids: string[]): Promise<[string, FilterLabel][]> => {
    const found = await Promise.all(ids.map(async (id) => {
        try {
            const { data } = await api.get<UserMinimal>(`/users/${id}`, { skipGlobalErrorHandler: true });
            return [id, { id: data.id, name: data.name }] as [string, FilterLabel];
        } catch {
            return null;
        }
    }));
    return found.filter((entry): entry is [string, FilterLabel] => entry !== null);
};

const fetchers: Record<LabelKind, (keys: string[]) => Promise<[string, FilterLabel][]>> = {
    tag: async (slugs) => (await resolveTagsBySlugs(slugs)).map((t) => [t.slug, { id: t.id, name: t.name, slug: t.slug, reviewStatus: t.reviewStatus }]),
    artist: async (ids) => (await resolveArtistsByIds(ids.map(Number))).map((a) => [String(a.id), { id: a.id, name: a.name, reviewStatus: a.reviewStatus }]),
    user: fetchUsers,
};

export function useFilterLabels(kind: LabelKind, keys: string[], enabled = true) {
    const queryClient = useQueryClient();
    configure(queryClient);
    const missing = [...new Set(keys)]
        .filter((key) => key && queryClient.getQueryData(labelKey(kind, key)) === undefined)
        .toSorted();

    const batch = useQuery({
        queryKey: ['labels', kind, missing],
        enabled: enabled && missing.length > 0,
        staleTime: Infinity,
        gcTime: 30 * 60_000,
        queryFn: async () => {
            const found = await fetchers[kind](missing);
            for (const [key, label] of found) {
                queryClient.setQueryData(labelKey(kind, key), label);
            }
            return found.length;
        },
    });

    return {
        get: (key: string) => queryClient.getQueryData<FilterLabel>(labelKey(kind, key)),
        isLoading: batch.isFetching,
    };
}
