import { useQuery, QueryClient } from '@tanstack/react-query';
import api from '../services/api';
import { useAuth } from '../context/AuthContext';
import { canModerate } from '../utils/roles';

export const NAV_BADGE_KEY = ['nav-badge'];

export const invalidateNavBadges = (queryClient: QueryClient) =>
    queryClient.invalidateQueries({ queryKey: NAV_BADGE_KEY });

const countOf = async (url: string, params: Record<string, unknown>) => {
    const { data } = await api.get<{ totalCount: number }>(url, { params, skipGlobalErrorHandler: true });
    return data.totalCount ?? 0;
};

export const useNavBadges = () => {
    const { user } = useAuth();

    const pending = useQuery({
        queryKey: [...NAV_BADGE_KEY, 'review', user?.id],
        enabled: !!user,
        staleTime: 0,
        queryFn: () => countOf('/review/tasks', { status: 'Pending', page: 1, pageSize: 1 }),
    });

    const reports = useQuery({
        queryKey: [...NAV_BADGE_KEY, 'reports', user?.id],
        enabled: !!user && canModerate(user),
        staleTime: 0,
        queryFn: () => countOf('/reports', { status: 'Pending', page: 1, pageSize: 1 }),
    });

    return { pendingCount: pending.data ?? 0, reportCount: reports.data ?? 0 };
};
