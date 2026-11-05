import { QueryClient } from '@tanstack/react-query';
import { ImageDto, PaginatedList } from '../types';

export function patchCachedImage(queryClient: QueryClient, id: number, patch: Partial<ImageDto>) {
    queryClient.setQueriesData<PaginatedList<ImageDto>>({ queryKey: ['images'] }, (old) => {
        if (!old || !Array.isArray(old.items) || !old.items.some((item) => item.id === id)) return old;
        return { ...old, items: old.items.map((item) => (item.id === id ? { ...item, ...patch } : item)) };
    });
}
