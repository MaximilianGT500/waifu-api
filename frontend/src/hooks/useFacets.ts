import { keepPreviousData, useQuery } from '@tanstack/react-query';
import api from '../services/api';
import { ImageFacets } from '../types';
import { imageListParams } from '../utils/apiSnippets';

export function facetParams(searchParams: URLSearchParams, fixed: Record<string, string> = {}, limit = 10): URLSearchParams {
    const params = imageListParams(searchParams, {}, fixed);
    for (const key of ['page', 'pageSize', 'orderBy', 'mine', 'includeMyPending']) params.delete(key);
    params.set('limit', String(limit));
    return params;
}

export const useFacets = (params: URLSearchParams, enabled = true) => {
    const key = params.toString();
    return useQuery({
        queryKey: ['facets', key],
        enabled,
        staleTime: 60_000,
        placeholderData: keepPreviousData,
        queryFn: async ({ signal }) => (await api.get<ImageFacets>('/images/facets', { params: new URLSearchParams(key), signal, skipGlobalErrorHandler: true })).data,
    });
};
