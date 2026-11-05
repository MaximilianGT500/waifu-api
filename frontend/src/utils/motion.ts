export type MotionPreference = 'system' | 'on' | 'off';

export const MOTION_KEY = 'motion';

const REDUCED_QUERY = '(prefers-reduced-motion: reduce)';

export const parseMotionPreference = (value: string | null): MotionPreference =>
    value === 'on' || value === 'off' ? value : 'system';

export const systemPrefersReducedMotion = () => window.matchMedia(REDUCED_QUERY).matches;

export const watchSystemMotion = (onChange: (reduced: boolean) => void) => {
    const query = window.matchMedia(REDUCED_QUERY);
    const listener = () => onChange(query.matches);
    query.addEventListener('change', listener);
    return () => query.removeEventListener('change', listener);
};

export const applyMotion = (enabled: boolean) => {
    const root = document.documentElement;
    if (enabled) delete root.dataset.motion;
    else root.dataset.motion = 'off';
};

export const effectsEnabled = () => document.documentElement.dataset.motion !== 'off';
