import { PointerEvent, useRef } from 'react';
import { effectsEnabled } from '../utils/motion';

export function useHolo<T extends HTMLElement>(maxTilt = 9) {
    const frame = useRef(0);
    const foils = useRef<HTMLElement[]>([]);

    const onPointerMove = (e: PointerEvent<T>) => {
        if (e.pointerType !== 'mouse' || !effectsEnabled()) return;
        const el = e.currentTarget;
        const { clientX, clientY } = e;
        cancelAnimationFrame(frame.current);
        frame.current = requestAnimationFrame(() => {
            const r = el.getBoundingClientRect();
            if (r.width === 0 || r.height === 0) return;
            if (!('tilting' in el.dataset)) {
                foils.current = Array.from(el.querySelectorAll<HTMLElement>('.holo-foil'));
                el.dataset.tilting = '';
            }
            const px = Math.min(Math.max((clientX - r.left) / r.width, 0), 1);
            const py = Math.min(Math.max((clientY - r.top) / r.height, 0), 1);
            const mx = `${(px * 100).toFixed(1)}%`;
            const my = `${(py * 100).toFixed(1)}%`;
            const flat = document.documentElement.dataset.design === 'manga';
            const hx = `${(15 + px * 70).toFixed(1)}%`;
            const hy = `${(15 + py * 70).toFixed(1)}%`;
            for (const foil of foils.current) {
                const s = foil.style;
                s.setProperty('--mx', mx);
                s.setProperty('--my', my);
                if (flat) continue;
                s.setProperty('--hx', hx);
                s.setProperty('--hy', hy);
            }
            if (flat) return;
            el.style.setProperty('--rx', `${((0.5 - py) * maxTilt).toFixed(2)}deg`);
            el.style.setProperty('--ry', `${((px - 0.5) * maxTilt).toFixed(2)}deg`);
        });
    };

    const onPointerLeave = (e: PointerEvent<T>) => {
        cancelAnimationFrame(frame.current);
        const el = e.currentTarget;
        el.style.setProperty('--rx', '0deg');
        el.style.setProperty('--ry', '0deg');
        delete el.dataset.tilting;
        foils.current = [];
    };

    return { onPointerMove, onPointerLeave };
}
