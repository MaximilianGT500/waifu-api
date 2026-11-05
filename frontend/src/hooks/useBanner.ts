import { useQuery, QueryClient } from '@tanstack/react-query';
import api from '../services/api';
import { useAuth } from '../context/AuthContext';
import { Banner, UserPermissions } from '../types';

export const BANNER_KEY = ['banner'];

export const invalidateBanner = (queryClient: QueryClient) =>
    queryClient.invalidateQueries({ queryKey: BANNER_KEY });

export const useActiveBanner = () =>
    useQuery({
        queryKey: BANNER_KEY,
        staleTime: 5 * 60 * 1000,
        queryFn: async () => {
            const { data } = await api.get<Banner | ''>('/banner', { skipGlobalErrorHandler: true });
            return data || null;
        },
    });

export const usePermissions = () => {
    const { user } = useAuth();

    const query = useQuery({
        queryKey: ['user', 'me', 'permissions', user?.id, user?.role],
        enabled: !!user,
        queryFn: async ({ signal }) => {
            const { data } = await api.get<UserPermissions>('/users/me/permissions', { skipGlobalErrorHandler: true, signal });
            return data;
        },
        staleTime: 5 * 60_000,
    });

    return {
        canManageBanner: query.data?.canManageBanner ?? false,
        uploadLimits: query.data?.upload ?? null,
        isLoading: !!user && query.isLoading,
    };
};
