import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import api from '../services/api';
import { ImageDto, PaginatedList, NsfwMode, AnimatedMode, Orientation, ImageOrderBy, ReviewStatusFilter, User } from '../types';
import { useAuth } from '../context/AuthContext';
import { canModerate } from '../utils/roles';
import { buildQueryParams } from '../utils/queryParams';

export interface ImageFilters {
  isNsfw?: NsfwMode;
  includedTags?: string[];
  excludedTags?: string[];
  includedArtists?: string[];
  excludedArtists?: string[];
  includedIds?: string[];
  excludedIds?: string[];
  isAnimated?: AnimatedMode;
  orientation?: Orientation;
  orderBy?: ImageOrderBy;
  page?: number;
  pageSize?: number;
  width?: string;
  height?: string;
  byteSize?: string;
  albumId?: string;
  albumUserId?: string;
  enabled?: boolean;
  uploaderId?: string;
  reviewStatus?: ReviewStatusFilter;
  color?: string;
  colorTolerance?: string;
  uploadedAfter?: string;
  uploadedBefore?: string;
  mine?: boolean;
  includeMyPending?: boolean;
}

const PREFETCH_FRESH_MS = 30_000;

const imagesKey = (filters: ImageFilters, user: User | null) => ['images', filters, user?.id];

async function fetchImages(filters: ImageFilters, user: User | null, signal?: AbortSignal) {
  const includeMyPendingChildren = user && !canModerate(user) ? true : undefined;
  const params = buildQueryParams({
    params: {
      isNsfw: filters.isNsfw,
      isAnimated: filters.isAnimated,
      orderBy: filters.orderBy,
      orientation: filters.orientation,
      width: filters.width,
      height: filters.height,
      byteSize: filters.byteSize,
      page: filters.page,
      pageSize: filters.pageSize,
      uploaderId: filters.uploaderId,
      reviewStatus: filters.reviewStatus,
      color: filters.color,
      colorTolerance: filters.colorTolerance,
      uploadedAfter: filters.uploadedAfter,
      uploadedBefore: filters.uploadedBefore,
      mine: filters.mine || undefined,
      includeMyPending: filters.includeMyPending || undefined,
      includeMyPendingChildren,
    },
    arrays: {
      includedTags: filters.includedTags,
      excludedTags: filters.excludedTags,
      includedArtists: filters.includedArtists,
      excludedArtists: filters.excludedArtists,
      includedIds: filters.includedIds,
      excludedIds: filters.excludedIds,
    },
  });

  const albumUser = filters.albumUserId || 'me';
  const endpoint = filters.albumId
      ? `/users/${albumUser}/albums/${filters.albumId}/images`
      : '/images';

  const { data } = await api.get<PaginatedList<ImageDto>>(endpoint, { params, signal });
  return data;
}

export const useImages = (filters: ImageFilters) => {
  const { user } = useAuth();
  const queryClient = useQueryClient();
  const isAlbumRequest = !!filters.albumId;
  const isEnabled = filters.enabled !== undefined ? filters.enabled : true;

  const query = useQuery({
    queryKey: imagesKey(filters, user),
    queryFn: ({ signal }) => fetchImages(filters, user, signal),
    enabled: isEnabled && (!isAlbumRequest || (isAlbumRequest && !!user)),
    staleTime: 0,
    refetchOnWindowFocus: false,
  });

  const hasNextPage = !!query.data?.hasNextPage;
  const deterministic = filters.orderBy !== undefined && filters.orderBy !== ImageOrderBy.Random;
  const filtersKey = JSON.stringify(filters);
  useEffect(() => {
    if (!hasNextPage || !deterministic || !isEnabled) return;
    const current = JSON.parse(filtersKey) as ImageFilters;
    const next = { ...current, page: (current.page ?? 1) + 1 };
    const handle = window.setTimeout(() => {
      void queryClient.prefetchQuery({
        queryKey: imagesKey(next, user),
        queryFn: ({ signal }) => fetchImages(next, user, signal),
        staleTime: PREFETCH_FRESH_MS,
      });
    }, 350);
    return () => window.clearTimeout(handle);
  }, [hasNextPage, deterministic, isEnabled, filtersKey, user, queryClient]);

  return query;
};
