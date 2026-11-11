import { readLocal, removeLocal, writeLocal } from './storage';
import { isBot } from './bot';
import { savedDesign } from './design';

export const WELCOME_KEY = 'welcome-done';
const HINT_KEY = 'design-hint';
const QUIET_PATHS = ['/auth/', '/privacy', '/imprint', '/contact', '/login'];

export function shouldWelcome(pathname: string): boolean {
    if (isBot() || readLocal(WELCOME_KEY) === '1' || savedDesign()) return false;
    return !QUIET_PATHS.some((path) => (path.endsWith('/') ? pathname.startsWith(path) : pathname === path));
}

export const markWelcomeDone = () => writeLocal(WELCOME_KEY, '1');

export const resetWelcome = () => removeLocal(WELCOME_KEY);

export const requestDesignHint = () => writeLocal(HINT_KEY, '1');

export function takeDesignHint(): boolean {
    if (readLocal(HINT_KEY) !== '1') return false;
    removeLocal(HINT_KEY);
    return true;
}

export function flashCoachMark(ms = 7000): () => void {
    const root = document.documentElement;
    root.dataset.coach = '';
    const handle = window.setTimeout(() => delete root.dataset.coach, ms);
    return () => {
        window.clearTimeout(handle);
        delete root.dataset.coach;
    };
}

export const WELCOME_EVENT = 'waifu:welcome';

export const openWelcome = () => window.dispatchEvent(new Event(WELCOME_EVENT));
