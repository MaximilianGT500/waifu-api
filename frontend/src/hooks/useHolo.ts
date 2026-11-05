import { PointerEvent, useRef } from 'react';
import { effectsEnabled } from '../utils/motion';

export function useHolo<T extends HTMLElement>(maxTilt = 9) {
    const frame = useRef(0);

    const onPointerMove = (e: PointerEvent<T>) => {
        if (e.pointerType !== 'mouse' || !effectsEnabled()) return;
        const el = e.currentTarget;
        const { clientX, clientY } = e;
        cancelAnimationFrame(frame.current);
        frame.current = requestAnimationFrame(() => {
            const r = el.getBoundingClientRect();
            if (r.width === 0 || r.height === 0) return;
            const px = Math.min(Math.max((clientX - r.left) / r.width, 0), 1);
            const py = Math.min(Math.max((clientY - r.top) / r.height, 0), 1);
            const s = el.style;
            s.setProperty('--mx', `${(px * 100).toFixed(1)}%`);
            s.setProperty('--my', `${(py * 100).toFixed(1)}%`);
            s.setProperty('--hx', `${(15 + px * 70).toFixed(1)}%`);
            s.setProperty('--hy', `${(15 + py * 70).toFixed(1)}%`);
            s.setProperty('--rx', `${((0.5 - py) * maxTilt).toFixed(2)}deg`);
            s.setProperty('--ry', `${((px - 0.5) * maxTilt).toFixed(2)}deg`);
            el.dataset.tilting = '';
        });
    };

    const onPointerLeave = (e: PointerEvent<T>) => {
        cancelAnimationFrame(frame.current);
        const el = e.currentTarget;
        el.style.setProperty('--rx', '0deg');
        el.style.setProperty('--ry', '0deg');
        delete el.dataset.tilting;
    };

    return { onPointerMove, onPointerLeave };
}
