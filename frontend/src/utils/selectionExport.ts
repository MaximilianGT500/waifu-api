import { ImageDto, NsfwMode } from '../types';
import { createZip, saveBlob, ZipEntry } from './zip';
import { safeExternalUrl } from './safeUrl';

export const ZIP_MAX_IMAGES = 60;
export const LINK_MAX_IMAGES = 50;

const extensionOf = (url: string) => {
    const match = /\.(png|jpe?g|gif|webp|avif)(?:$|\?)/i.exec(new URL(url, window.location.href).pathname);
    return match ? `.${match[1].toLowerCase()}` : '';
};

export async function downloadImagesAsZip(images: ImageDto[], onProgress: (done: number, total: number) => void): Promise<{ saved: number; failed: number }> {
    const chosen = images.slice(0, ZIP_MAX_IMAGES);
    const entries: (ZipEntry | null)[] = Array.from({ length: chosen.length }, () => null);
    let done = 0;
    let next = 0;
    const worker = async () => {
        while (next < chosen.length) {
            const index = next++;
            const image = chosen[index];
            const url = safeExternalUrl(image.url);
            try {
                if (!url) throw new Error('unsafe url');
                const res = await fetch(url, { mode: 'cors', credentials: 'omit' });
                if (!res.ok) throw new Error(`status ${res.status}`);
                const data = new Uint8Array(await res.arrayBuffer());
                entries[index] = { name: `${image.id}${extensionOf(url) || '.img'}`, data, modified: new Date(image.uploadedAt) };
            } catch {
                entries[index] = null;
            }
            done += 1;
            onProgress(done, chosen.length);
        }
    };
    await Promise.all(Array.from({ length: Math.min(4, chosen.length) }, worker));
    const files = entries.filter((entry): entry is ZipEntry => entry !== null);
    if (files.length > 0) {
        const stamp = new Date().toISOString().slice(0, 10);
        saveBlob(createZip(files), `images-${stamp}.zip`);
    }
    return { saved: files.length, failed: chosen.length - files.length };
}

export function selectionLink(images: ImageDto[]): string {
    const params = new URLSearchParams();
    for (const image of images.slice(0, LINK_MAX_IMAGES)) params.append('includedIds', String(image.id));
    if (images.some((image) => image.isNsfw)) params.set('isNsfw', NsfwMode.All);
    params.set('orderBy', 'UploadedAt');
    return `${window.location.origin}/gallery?${params}`;
}
