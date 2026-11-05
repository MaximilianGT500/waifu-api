export type Accent = 'violet' | 'pink' | 'orange' | 'green' | 'blue' | 'mono';

export const ACCENT_KEY = 'accent';

export const DEFAULT_ACCENT: Accent = 'violet';

export const ACCENTS: { id: Accent; label: string; swatch: string }[] = [
    { id: 'violet', label: 'Violet', swatch: '#8b5cf6' },
    { id: 'pink', label: 'Pink', swatch: '#ec4899' },
    { id: 'orange', label: 'Orange', swatch: '#f97316' },
    { id: 'green', label: 'Green', swatch: '#10b981' },
    { id: 'blue', label: 'Blue', swatch: '#3b82f6' },
    { id: 'mono', label: 'Mono', swatch: '#52525b' },
];

export const parseAccent = (value: string | null): Accent =>
    ACCENTS.find((a) => a.id === value)?.id ?? DEFAULT_ACCENT;

export const applyAccent = (accent: Accent) => {
    const root = document.documentElement;
    if (accent === DEFAULT_ACCENT) delete root.dataset.accent;
    else root.dataset.accent = accent;
};
