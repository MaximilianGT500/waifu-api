import { QueryClient, useQuery, useQueryClient } from '@tanstack/react-query';
import api from '../services/api';
import { useAuth } from '../context/AuthContext';
import { ImageDto, PaginatedList, ReviewStatusFilter, User } from '../types';
import { canModerate } from '../utils/roles';

const DETAIL_FRESH_MS = 30_000;

export const imageDetailKey = (id: number | string, userId?: number | null) => ['image', String(id), userId ?? null] as const;

const detailParams = (user: User | null): Record<string, string> => {
    if (user && canModerate(user)) return { childrenReviewStatus: ReviewStatusFilter.All };
    if (user) return { includeMyPendingChildren: 'true' };
    return {};
};

export async function fetchImageDetail(id: number | string, user: User | null, signal?: AbortSignal): Promise<ImageDto> {
    const { data } = await api.get<ImageDto>(`/images/${id}`, { params: detailParams(user), skipGlobalErrorHandler: true, signal });
    return data;
}

export function findCachedImage(queryClient: QueryClient, id: number | string): ImageDto | undefined {
    const wanted = Number(id);
    for (const [, data] of queryClient.getQueriesData<PaginatedList<ImageDto>>({ queryKey: ['images'] })) {
        const hit = data?.items?.find((item) => item.id === wanted);
        if (hit) return hit;
    }
    const hero = queryClient.getQueryData<{ image: ImageDto | null }>(['home-hero']);
    return hero?.image?.id === wanted ? hero.image : undefined;
}

export function useImageDetail(id: string) {
    const { user } = useAuth();
    const queryClient = useQueryClient();
    return useQuery({
        queryKey: imageDetailKey(id, user?.id),
        queryFn: ({ signal }) => fetchImageDetail(id, user, signal),
        placeholderData: () => findCachedImage(queryClient, id),
        staleTime: DETAIL_FRESH_MS,
        retry: false,
    });
}

export function prefetchImageDetail(queryClient: QueryClient, id: number, user: User | null) {
    return queryClient.prefetchQuery({
        queryKey: imageDetailKey(id, user?.id),
        queryFn: ({ signal }) => fetchImageDetail(id, user, signal),
        staleTime: DETAIL_FRESH_MS,
    });
}
