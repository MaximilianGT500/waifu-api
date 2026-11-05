const SAFE_PROTOCOLS = new Set(['http:', 'https:']);

export function safeExternalUrl(raw: string | null | undefined): string | undefined {
    if (!raw) return undefined;
    const value = raw.trim();
    if (!value || /[\s\p{Cc}\p{Cf}\p{Default_Ignorable_Code_Point}\\]/u.test(value)) return undefined;
    try {
        const url = new URL(value);
        if (!SAFE_PROTOCOLS.has(url.protocol) || !url.hostname || url.username || url.password) {
            return undefined;
        }
        return url.href;
    } catch {
        return undefined;
    }
}

export function safeMailto(email: string | null | undefined): string | undefined {
    if (!email) return undefined;
    const value = email.trim();
    if (!/^[^\s@<>"'`]+@[^\s@<>"'`]+\.[^\s@<>"'`]+$/.test(value)) return undefined;
    return `mailto:${value}`;
}

export function displayUrl(url: string): string {
    return url.replace(/^https?:\/\//i, '').replace(/\/$/, '');
}

export function safeRedirectPath(raw: string | null | undefined, fallback = '/'): string {
    if (!raw || typeof raw !== 'string') return fallback;
    if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return fallback;
    try {
        const url = new URL(raw, window.location.origin);
        if (url.origin !== window.location.origin) return fallback;
        if (url.pathname.startsWith('//')) return fallback;
        if (url.pathname.startsWith('/auth/callback') || url.pathname === '/login') return fallback;
        return url.pathname + url.search + url.hash;
    } catch {
        return fallback;
    }
}
