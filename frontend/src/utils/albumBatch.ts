import { QueryClient } from '@tanstack/react-query';
import api from '../services/api';
import { AlbumBatchResult, ImageDto } from '../types';
import { patchCachedImage } from './imageCache';
import { nowIso } from './time';

export const ALBUM_BATCH_MAX = 100;

export async function batchAlbumImages(albumId: number | 'favorites', change: { add?: number[]; remove?: number[] }): Promise<AlbumBatchResult> {
    const result: AlbumBatchResult = { added: 0, removed: 0, skipped: [], imageCount: 0 };
    const add = change.add ?? [];
    const remove = change.remove ?? [];
    for (let offset = 0; offset < Math.max(add.length, remove.length); offset += ALBUM_BATCH_MAX) {
        const { data } = await api.patch<AlbumBatchResult>(`/users/me/albums/${albumId}/images`, {
            add: add.slice(offset, offset + ALBUM_BATCH_MAX),
            remove: remove.slice(offset, offset + ALBUM_BATCH_MAX),
        });
        result.added += data.added;
        result.removed += data.removed;
        result.skipped.push(...data.skipped);
        result.imageCount = data.imageCount;
    }
    return result;
}

export function applyFavoriteChange(queryClient: QueryClient, images: ImageDto[], liked: boolean) {
    const now = nowIso();
    for (const image of images) {
        if (!!image.likedAt === liked) continue;
        patchCachedImage(queryClient, image.id, {
            likedAt: liked ? now : undefined,
            favorites: Math.max(0, image.favorites + (liked ? 1 : -1)),
            albums: liked ? image.albums : (image.albums ?? []).filter((a) => !a.isDefault),
        });
    }
}

export const pluralImages = (n: number) => `${n.toLocaleString()} ${n === 1 ? 'image' : 'images'}`;
