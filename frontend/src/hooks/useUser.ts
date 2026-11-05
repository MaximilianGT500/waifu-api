import { useQuery } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import api from '../services/api';
import { User } from '../types';

export const useUser = (enabled: boolean) => {
  return useQuery({
    queryKey: ['user', 'me'],
    queryFn: async ({ signal }) => {
      const { data } = await api.get<User>('/users/me', { signal, skipGlobalErrorHandler: true });
      return data;
    },
    enabled,
    retry: (failureCount, error) => {
      const status = isAxiosError(error) ? error.response?.status : undefined;
      if (status !== undefined && status < 500) return false;
      return failureCount < 2;
    },
    refetchOnMount: true,
    refetchOnWindowFocus: true,
    staleTime: 5 * 60_000,
  });
};
