import { getEnv } from './env';
import { sinceToIso } from './imageFilters';

export type SnippetLanguage = 'url' | 'curl' | 'javascript' | 'python';

export const SNIPPET_LANGUAGES: { id: SnippetLanguage; label: string }[] = [
    { id: 'url', label: 'URL' },
    { id: 'curl', label: 'cURL' },
    { id: 'javascript', label: 'JavaScript' },
    { id: 'python', label: 'Python' },
];

export function apiBaseUrl(): string {
    const base = getEnv('VITE_API_URL') || '/api';
    try {
        return new URL(base, window.location.origin).href.replace(/\/$/, '');
    } catch {
        return base.replace(/\/$/, '');
    }
}

const IMAGE_LIST_PARAMS = [
    'isNsfw', 'isAnimated', 'orientation', 'orderBy', 'width', 'height', 'byteSize',
    'includedTags', 'excludedTags', 'includedArtists', 'excludedArtists', 'includedIds', 'excludedIds',
    'uploaderId', 'reviewStatus', 'color', 'colorTolerance', 'uploadedAfter', 'uploadedBefore', 'mine', 'includeMyPending',
    'page', 'pageSize',
];

export function imageListParams(searchParams: URLSearchParams, defaults: Record<string, string> = {}, fixed: Record<string, string> = {}): URLSearchParams {
    const out = new URLSearchParams();
    const since = sinceToIso(searchParams.get('since'));
    for (const key of IMAGE_LIST_PARAMS) {
        const values = key === 'uploadedAfter' && since ? [since] : searchParams.getAll(key).filter((v) => v !== '');
        if (values.length === 0 && defaults[key] !== undefined) values.push(defaults[key]);
        if (fixed[key] !== undefined && !values.includes(fixed[key])) values.unshift(fixed[key]);
        for (const value of values) out.append(key, value);
    }
    return out;
}

export function apiUrl(path: string, params?: URLSearchParams): string {
    const query = params?.toString();
    return `${apiBaseUrl()}${path}${query ? `?${query}` : ''}`;
}

const shellQuote = (value: string) => `'${value.replace(/'/g, `'\\''`)}'`;

export function snippet(language: SnippetLanguage, url: string, needsAuth: boolean): string {
    switch (language) {
        case 'url':
            return url;
        case 'curl':
            return needsAuth
                ? `curl ${shellQuote(url)} \\\n  -H 'X-Api-Key: YOUR_API_KEY'`
                : `curl ${shellQuote(url)}`;
        case 'javascript':
            return [
                `const res = await fetch(${JSON.stringify(url)}${needsAuth ? ", {\n  headers: { 'X-Api-Key': 'YOUR_API_KEY' },\n}" : ''});`,
                'if (!res.ok) throw new Error(`HTTP ${res.status}`);',
                'const data = await res.json();',
                'console.log(data);',
            ].join('\n');
        case 'python':
            return [
                'import requests',
                '',
                `res = requests.get(${JSON.stringify(url)}${needsAuth ? ", headers={'X-Api-Key': 'YOUR_API_KEY'}" : ''}, timeout=10)`,
                'res.raise_for_status()',
                'data = res.json()',
                'print(data)',
            ].join('\n');
    }
}
