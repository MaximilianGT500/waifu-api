import { UploadLimits } from '../types';

export const DEFAULT_ACCEPTED_TYPES = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'];

const TYPE_LABELS: Record<string, string> = {
    'image/jpeg': 'JPEG',
    'image/png': 'PNG',
    'image/gif': 'GIF',
    'image/webp': 'WebP',
};

export interface ImageFileInfo {
    width: number;
    height: number;
}

export interface ImageFileCheck {
    error: string | null;
    info: ImageFileInfo | null;
}

export function acceptedTypes(limits: UploadLimits | null | undefined): string[] {
    return limits?.allowedContentTypes?.length ? limits.allowedContentTypes : DEFAULT_ACCEPTED_TYPES;
}

export function describeTypes(types: string[]): string {
    const labels = types.map((t) => TYPE_LABELS[t] ?? t.replace(/^image\//, '').toUpperCase());
    if (labels.length <= 1) return labels.join('');
    return `${labels.slice(0, -1).join(', ')} or ${labels[labels.length - 1]}`;
}

export function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function readImageDimensions(file: File): Promise<ImageFileInfo | null> {
    return new Promise((resolve) => {
        const url = URL.createObjectURL(file);
        const img = new Image();
        const done = (info: ImageFileInfo | null) => {
            URL.revokeObjectURL(url);
            resolve(info);
        };
        img.addEventListener('load', () => done(img.naturalWidth > 0 ? { width: img.naturalWidth, height: img.naturalHeight } : null), { once: true });
        img.addEventListener('error', () => done(null), { once: true });
        img.src = url;
    });
}

const fits = (w: number, h: number, minW: number, minH: number, maxW: number, maxH: number) =>
    w >= minW && h >= minH && w <= maxW && h <= maxH;

export async function checkImageFile(file: File, limits: UploadLimits | null | undefined): Promise<ImageFileCheck> {
    const types = acceptedTypes(limits);
    if (!types.includes(file.type)) {
        return { error: `This file type is not supported. Please choose a ${describeTypes(types)} image.`, info: null };
    }
    if (file.size === 0) {
        return { error: 'The selected file is empty.', info: null };
    }
    if (limits?.maxFileSizeMb && file.size > limits.maxFileSizeMb * 1024 * 1024) {
        return { error: `The file is too large (${formatBytes(file.size)}). The maximum size is ${limits.maxFileSizeMb} MB.`, info: null };
    }

    const info = await readImageDimensions(file);
    if (!info || !limits) return { error: null, info };

    const mayBeAnimated = file.type === 'image/gif' || file.type === 'image/webp';
    const orientations: Array<[number, number]> = [[info.width, info.height], [info.height, info.width]];
    const acceptable = orientations.some(([w, h]) =>
        fits(w, h, limits.minWidth, limits.minHeight, limits.maxWidth, limits.maxHeight) ||
        (mayBeAnimated && fits(w, h, limits.animatedMinWidth, limits.animatedMinHeight, limits.animatedMaxWidth, limits.animatedMaxHeight))
    );
    if (!acceptable) {
        const tooSmall = info.width < limits.minWidth || info.height < limits.minHeight;
        return {
            error: tooSmall
                ? `The image resolution is too low (${info.width}×${info.height}). The minimum is ${limits.minWidth}×${limits.minHeight}.`
                : `The image resolution is too high (${info.width}×${info.height}). The maximum is ${limits.maxWidth}×${limits.maxHeight}.`,
            info,
        };
    }
    if (limits.maxPixels > 0 && info.width * info.height > limits.maxPixels) {
        return { error: `The image resolution is too high (${info.width}×${info.height}).`, info };
    }
    return { error: null, info };
}

export function firstImageFile(files: FileList | null | undefined): File | null {
    if (!files) return null;
    for (const file of Array.from(files)) {
        if (file.type.startsWith('image/')) return file;
    }
    return files.length > 0 ? files[0] : null;
}
