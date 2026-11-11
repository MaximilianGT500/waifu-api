import { useEffect, useEffectEvent, useRef } from 'react';
import { X } from 'lucide-react';
import { flashCoachMark } from '../utils/welcome';

const VISIBLE_MS = 12_000;

const ThemeHint = ({ onClose }: { onClose: () => void }) => {
    const ref = useRef<HTMLDivElement>(null);
    const close = useEffectEvent(() => onClose());

    useEffect(() => {
        const stop = flashCoachMark(VISIBLE_MS);
        const handle = window.setTimeout(close, VISIBLE_MS);
        const onPointerDown = (e: PointerEvent) => {
            if (!ref.current?.contains(e.target as Node)) close();
        };
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') close();
        };
        document.addEventListener('pointerdown', onPointerDown);
        document.addEventListener('keydown', onKeyDown);
        return () => {
            stop();
            window.clearTimeout(handle);
            document.removeEventListener('pointerdown', onPointerDown);
            document.removeEventListener('keydown', onKeyDown);
        };
    }, []);

    return (
        <div
            ref={ref}
            role="status"
            className="absolute right-0 top-full z-50 mt-3 w-[min(18rem,calc(100vw-1.5rem))] rounded-[1.1rem] bg-popover p-4 pr-10 text-sm text-popover-foreground shadow-float ring-1 ring-border animate-in fade-in slide-in-from-top-2 duration-300 dark:ring-white/8"
        >
            <span className="absolute -top-[0.4rem] right-[0.95rem] size-3 rotate-45 border-l border-t border-border bg-popover dark:border-white/10" aria-hidden="true" />
            <p className="font-black">Change your look any time</p>
            <p className="mt-1 leading-snug text-muted-foreground">Design, color mode, accent and effects are all in this menu.</p>
            <button
                type="button"
                onClick={onClose}
                aria-label="Dismiss the hint"
                className="absolute right-2 top-2 grid size-7 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            >
                <X size={15} aria-hidden="true" />
            </button>
        </div>
    );
};

export default ThemeHint;
