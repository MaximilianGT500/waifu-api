const TOKEN_KEY = 'token';
const REDIRECT_KEY = 'auth_redirect';
const STATE_KEY = 'oauth_states';
const LEGACY_STATE_KEY = 'oauth_state';
const STATE_TTL_MS = 10 * 60_000;
const MAX_STATES = 5;

function isPlausibleJwt(token: string): boolean {
    return token.length < 4096 && /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$/.test(token);
}

export function getStoredToken(): string | null {
    try {
        const token = localStorage.getItem(TOKEN_KEY);
        if (!token) return null;
        if (!isPlausibleJwt(token)) {
            localStorage.removeItem(TOKEN_KEY);
            return null;
        }
        return token;
    } catch {
        return null;
    }
}

export function storeToken(token: string | null) {
    try {
        if (token && isPlausibleJwt(token)) localStorage.setItem(TOKEN_KEY, token);
        else localStorage.removeItem(TOKEN_KEY);
    } catch {
    }
}

export function clearAuthStorage() {
    try {
        localStorage.removeItem(TOKEN_KEY);
        sessionStorage.removeItem(LEGACY_STATE_KEY);
    } catch {
    }
}

export function decodeJwtPayload(token: string): Record<string, unknown> | null {
    try {
        const parts = token.split('.');
        if (parts.length !== 3) return null;
        const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
        const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
        const json = decodeURIComponent(
            atob(padded)
                .split('')
                .map((c) => '%' + c.charCodeAt(0).toString(16).padStart(2, '0'))
                .join('')
        );
        const payload = JSON.parse(json);
        return payload && typeof payload === 'object' ? payload : null;
    } catch {
        return null;
    }
}

type PendingState = { v: string; t: number };

function readStates(now: number): PendingState[] {
    try {
        const parsed: unknown = JSON.parse(localStorage.getItem(STATE_KEY) || '[]');
        if (!Array.isArray(parsed)) return [];
        return parsed.filter(
            (s): s is PendingState =>
                !!s && typeof s.v === 'string' && typeof s.t === 'number' && s.t <= now && now - s.t < STATE_TTL_MS
        );
    } catch {
        return [];
    }
}

function writeStates(states: PendingState[]) {
    try {
        if (states.length > 0) localStorage.setItem(STATE_KEY, JSON.stringify(states));
        else localStorage.removeItem(STATE_KEY);
    } catch {
    }
}

function constantTimeEqual(a: string, b: string): boolean {
    if (a.length !== b.length) return false;
    let diff = 0;
    for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
    return diff === 0;
}

export function createOAuthState(): string | null {
    const bytes = new Uint8Array(32);
    crypto.getRandomValues(bytes);
    const state = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    const now = Date.now();
    writeStates([...readStates(now).slice(-(MAX_STATES - 1)), { v: state, t: now }]);
    return readStates(now).some((s) => s.v === state) ? state : null;
}

export function consumeOAuthState(state: string | null): boolean {
    const now = Date.now();
    const states = readStates(now);
    const match = state ? states.findIndex((s) => constantTimeEqual(s.v, state)) : -1;
    if (match >= 0) states.splice(match, 1);
    writeStates(states);
    return match >= 0;
}

export function storeRedirect(path: string) {
    try {
        localStorage.setItem(REDIRECT_KEY, path);
    } catch {
    }
}

export function takeRedirect(): string | null {
    try {
        const v = localStorage.getItem(REDIRECT_KEY);
        localStorage.removeItem(REDIRECT_KEY);
        return v;
    } catch {
        return null;
    }
}
