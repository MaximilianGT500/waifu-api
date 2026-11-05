export type Design = 'collector' | 'classic';

export const DESIGN_COOKIE = 'waifu_design';

const ONE_YEAR = 60 * 60 * 24 * 365;

export const DESIGNS: { id: Design; label: string }[] = [
    { id: 'collector', label: 'Collector' },
    { id: 'classic', label: 'Classic' },
];

export function currentDesign(): Design {
    return document.documentElement.dataset.design === 'classic' ? 'classic' : 'collector';
}

export function switchDesign(design: Design) {
    if (design === currentDesign()) return;
    const secure = window.location.protocol === 'https:' ? '; Secure' : '';
    document.cookie = `${DESIGN_COOKIE}=${design}; Path=/; Max-Age=${ONE_YEAR}; SameSite=Lax${secure}`;
    window.location.reload();
}

export function applyThemeColor(dark: boolean) {
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    const color = meta?.dataset[dark ? 'dark' : 'light'];
    if (meta && color) meta.content = color;
}
