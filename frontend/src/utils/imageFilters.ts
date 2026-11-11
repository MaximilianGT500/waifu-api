export interface SinceOption {
    id: string;
    label: string;
    hours: number;
}

export const SINCE_OPTIONS: SinceOption[] = [
    { id: '24h', label: 'Last 24 hours', hours: 24 },
    { id: '7d', label: 'Last 7 days', hours: 24 * 7 },
    { id: '30d', label: 'Last 30 days', hours: 24 * 30 },
    { id: '365d', label: 'Last 12 months', hours: 24 * 365 },
];

const HOUR = 3_600_000;

export function sinceToIso(since: string | null | undefined, now = Date.now()): string | undefined {
    const option = SINCE_OPTIONS.find((o) => o.id === since);
    if (!option) return undefined;
    return new Date(Math.floor((now - option.hours * HOUR) / HOUR) * HOUR).toISOString();
}

export function effectiveUploadedAfter(params: URLSearchParams): string | undefined {
    return sinceToIso(params.get('since')) ?? (params.get('uploadedAfter') || undefined);
}

export interface ColorSwatch {
    hex: string;
    name: string;
}

export const COLOR_SWATCHES: ColorSwatch[] = [
    { hex: 'E53935', name: 'Red' },
    { hex: 'FB8C00', name: 'Orange' },
    { hex: 'FDD835', name: 'Yellow' },
    { hex: '43A047', name: 'Green' },
    { hex: '26A69A', name: 'Teal' },
    { hex: '4FC3F7', name: 'Sky' },
    { hex: '1E88E5', name: 'Blue' },
    { hex: '8E24AA', name: 'Purple' },
    { hex: 'F06292', name: 'Pink' },
    { hex: 'F5CBA7', name: 'Peach' },
    { hex: '795548', name: 'Brown' },
    { hex: '9E9E9E', name: 'Gray' },
    { hex: '212121', name: 'Black' },
    { hex: 'F5F5F5', name: 'White' },
];

export const DEFAULT_COLOR_TOLERANCE = 15;

export function normalizeHex(value: string | null | undefined): string | null {
    const trimmed = (value ?? '').trim().replace(/^#/, '');
    return /^[0-9a-f]{6}$/i.test(trimmed) ? trimmed.toUpperCase() : null;
}

export function colorName(hex: string): string {
    return COLOR_SWATCHES.find((s) => s.hex === hex)?.name ?? `#${hex}`;
}

export function formatDay(iso: string): string {
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? iso : date.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}

export const EXTRA_FILTER_PARAMS = ['color', 'colorTolerance', 'since', 'uploadedAfter', 'uploadedBefore'];
