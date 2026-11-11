import { readLocal, writeLocal } from './storage';

const KEY = 'last-visit';
const SESSION_KEY = 'visit-since';
const HOUR = 3_600_000;

let since: string | null | undefined;

export function visitSince(now = Date.now()): string | null {
    if (since !== undefined) return since;
    let value = '';
    try {
        const stored = sessionStorage.getItem(SESSION_KEY);
        if (stored !== null) {
            value = stored;
        } else {
            const last = Number(readLocal(KEY));
            if (Number.isFinite(last) && last > 0 && now - last > HOUR) value = new Date(Math.floor(last / HOUR) * HOUR).toISOString();
            sessionStorage.setItem(SESSION_KEY, value);
        }
    } catch {
        value = '';
    }
    writeLocal(KEY, String(now));
    since = value || null;
    return since;
}
