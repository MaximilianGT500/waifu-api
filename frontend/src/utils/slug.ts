export const SLUG_PATTERN = /^[a-z0-9-]+$/;
export const SLUG_MAX = 50;

export function slugify(text: string): string {
    return text
        .toString()
        .normalize('NFD')
        .replace(/[̀-ͯ]/g, '')
        .toLowerCase()
        .replace(/[\s_]+/g, '-')
        .replace(/[^a-z0-9-]+/g, '')
        .replace(/-{2,}/g, '-')
        .replace(/^-+|-+$/g, '');
}
