const SAMPLE = 56;
const MIN_DISTANCE_SQ = 52 * 52;

const toHex = (r: number, g: number, b: number) =>
    `#${[r, g, b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')}`.toUpperCase();

const distanceSq = (a: number[], b: number[]) => {
    const rm = (a[0] + b[0]) / 2;
    const dr = a[0] - b[0];
    const dg = a[1] - b[1];
    const db = a[2] - b[2];
    return ((2 + rm / 256) * dr * dr + 4 * dg * dg + (2 + (255 - rm) / 256) * db * db) / 3;
};

export async function extractPalette(src: string, count = 6): Promise<string[]> {
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.decoding = 'async';
    img.src = src;
    await img.decode();
    const scale = Math.min(1, SAMPLE / Math.max(img.naturalWidth, img.naturalHeight, 1));
    const width = Math.max(1, Math.round(img.naturalWidth * scale));
    const height = Math.max(1, Math.round(img.naturalHeight * scale));
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    if (!ctx) return [];
    ctx.drawImage(img, 0, 0, width, height);
    const { data } = ctx.getImageData(0, 0, width, height);

    const buckets = new Map<number, { n: number; r: number; g: number; b: number }>();
    for (let i = 0; i < data.length; i += 4) {
        if (data[i + 3] < 128) continue;
        const r = data[i];
        const g = data[i + 1];
        const b = data[i + 2];
        const key = ((r >> 4) << 8) | ((g >> 4) << 4) | (b >> 4);
        const bucket = buckets.get(key);
        if (bucket) {
            bucket.n += 1;
            bucket.r += r;
            bucket.g += g;
            bucket.b += b;
        } else {
            buckets.set(key, { n: 1, r, g, b });
        }
    }

    const ranked = [...buckets.values()]
        .map((bucket) => ({ n: bucket.n, rgb: [bucket.r / bucket.n, bucket.g / bucket.n, bucket.b / bucket.n] }))
        .toSorted((a, b) => b.n - a.n);
    const picked: number[][] = [];
    for (const { rgb } of ranked) {
        if (picked.every((other) => distanceSq(rgb, other) > MIN_DISTANCE_SQ)) picked.push(rgb);
        if (picked.length === count) break;
    }
    return picked.map(([r, g, b]) => toHex(r, g, b));
}
