import { useEffect, useEffectEvent } from 'react';

export type HotkeyMap = Record<string, (event: KeyboardEvent) => void | false>;

interface HotkeyOptions {
    enabled?: boolean;
    inDialogs?: boolean;
}

const EDITABLE = 'input, textarea, select, [contenteditable=""], [contenteditable="true"]';

export const isTypingTarget = (target: EventTarget | null) =>
    target instanceof Element && !!target.closest(EDITABLE);

export const dialogOpen = () => !!document.querySelector('[aria-modal="true"]');

function keyOf(event: KeyboardEvent): string | null {
    if (event.altKey) return null;
    const key = event.key.length === 1 ? event.key.toLowerCase() : event.key;
    if (event.ctrlKey || event.metaKey) return `mod+${key}`;
    return key;
}

export function useHotkeys(map: HotkeyMap, { enabled = true, inDialogs = false }: HotkeyOptions = {}) {
    const onKeyDown = useEffectEvent((event: KeyboardEvent) => {
        if (event.defaultPrevented || event.repeat || event.isComposing) return;
        const key = keyOf(event);
        if (!key) return;
        const handler = map[key];
        if (!handler) return;
        const isModCombo = key.startsWith('mod+');
        if (!isModCombo && isTypingTarget(event.target)) return;
        if (!inDialogs && dialogOpen()) return;
        if (handler(event) === false) return;
        event.preventDefault();
    });

    useEffect(() => {
        if (!enabled) return;
        const listener = (event: KeyboardEvent) => onKeyDown(event);
        window.addEventListener('keydown', listener);
        return () => window.removeEventListener('keydown', listener);
    }, [enabled]);
}

export const modKeyLabel = () =>
    /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent) ? '⌘' : 'Ctrl';
