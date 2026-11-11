export type Design = 'collector' | 'manga' | 'classic';

export const DESIGN_COOKIE = 'waifu_design';

const ONE_YEAR = 60 * 60 * 24 * 365;

export interface DesignInfo {
    id: Design;
    label: string;
    tagline: string;
}

export const DESIGNS: DesignInfo[] = [
    { id: 'collector', label: 'Collector', tagline: 'Trading cards in a binder, with holo foil.' },
    { id: 'manga', label: 'Manga', tagline: 'Ink lines, screentone and speed lines.' },
    { id: 'classic', label: 'Classic', tagline: 'The original compact layout.' },
];

const SHARED_BUNDLE: Design[] = ['collector', 'manga'];

export const DESIGN_SWITCHED_EVENT = 'waifu:design';

export function parseDesign(value: string | null | undefined): Design | null {
    return value === 'collector' || value === 'manga' || value === 'classic' ? value : null;
}

export function currentDesign(): Design {
    return parseDesign(document.documentElement.dataset.design) ?? 'collector';
}

export function savedDesign(): Design | null {
    const match = new RegExp(`(?:^|;\\s*)${DESIGN_COOKIE}=([a-z]+)`).exec(document.cookie);
    return parseDesign(match?.[1]);
}

export function needsReload(target: Design, from: Design = currentDesign()): boolean {
    return target !== from && !(SHARED_BUNDLE.includes(target) && SHARED_BUNDLE.includes(from));
}

function writeDesignCookie(design: Design) {
    const secure = window.location.protocol === 'https:' ? '; Secure' : '';
    document.cookie = `${DESIGN_COOKIE}=${design}; Path=/; Max-Age=${ONE_YEAR}; SameSite=Lax${secure}`;
}

export function previewDesign(design: Design): boolean {
    const from = currentDesign();
    if (design === from) return true;
    if (needsReload(design, from)) return false;
    document.documentElement.dataset.design = design;
    applyThemeColor(document.documentElement.classList.contains('dark'));
    window.dispatchEvent(new CustomEvent(DESIGN_SWITCHED_EVENT, { detail: design }));
    return true;
}

export function switchDesign(design: Design, options: { beforeReload?: () => void } = {}): boolean {
    writeDesignCookie(design);
    if (previewDesign(design)) return false;
    options.beforeReload?.();
    window.location.reload();
    return true;
}

export function applyThemeColor(dark: boolean) {
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    if (!meta) return;
    const manga = currentDesign() === 'manga';
    const color = (manga ? meta.dataset[dark ? 'mangaDark' : 'mangaLight'] : undefined) ?? meta.dataset[dark ? 'dark' : 'light'];
    if (color) meta.content = color;
}
