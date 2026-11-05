import { getEnv } from './env';

const DEFAULT_CARD_WIDTH = 640;
const DEFAULT_PREVIEW_WIDTH = 1280;
const DEFAULT_WIDTH_PARAM = 'width';
const CARD_WIDTHS = [240, 360, 480, 720];
const PREVIEW_WIDTHS = [640, 960, 1280, 1920];

const isLocalHost = (host: string) => host === 'localhost' || host === '127.0.0.1' || host.endsWith('.local');

const transformEnabled = () => getEnv('VITE_IMG_TRANSFORM_ENABLED') === 'true';
const transformPath = () => getEnv('VITE_IMG_TRANSFORM_PATH') || '';
const widthParam = () => getEnv('VITE_IMG_WIDTH_PARAM') || DEFAULT_WIDTH_PARAM;
const cardWidth = () => Number(getEnv('VITE_IMG_CARD_WIDTH')) || DEFAULT_CARD_WIDTH;
const previewWidth = () => Number(getEnv('VITE_IMG_PREVIEW_WIDTH')) || DEFAULT_PREVIEW_WIDTH;

function resize(url: string, width: number): string {
    if (!transformEnabled()) return url;
    const path = transformPath();
    if (!path) return url;
    let u: URL;
    try {
        u = new URL(url);
    } catch {
        return url;
    }
    if (isLocalHost(u.hostname)) return url;
    const last = path.slice(-1);
    const sep = last === ',' || last === '/' ? '' : path.includes('=') ? ',' : '/';
    return `${u.origin}${path}${sep}${widthParam()}=${width}${u.pathname}`;
}

function srcSet(url: string, widths: number[], originalWidth?: number): string | undefined {
    const usable = originalWidth ? widths.filter((w) => w < originalWidth) : widths;
    if (usable.length === 0 || resize(url, usable[0]) === url) return undefined;
    return usable.map((w) => `${resize(url, w)} ${w}w`).join(', ');
}

export const imgCard = (url: string) => resize(url, cardWidth());

export const imgPreview = (url: string) => resize(url, previewWidth());

export const imgCardSrcSet = (url: string, originalWidth?: number) => srcSet(url, CARD_WIDTHS, originalWidth);

export const imgPreviewSrcSet = (url: string, originalWidth?: number) => srcSet(url, PREVIEW_WIDTHS, originalWidth);
