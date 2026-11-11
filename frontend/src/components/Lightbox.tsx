import { useEffect, useEffectEvent, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { ChevronLeft, ChevronRight, ExternalLink as ExternalLinkIcon, Loader2, Pause, Play, Timer, X, ZoomIn, ZoomOut } from 'lucide-react';
import { readLocal, writeLocal } from '../utils/storage';
import { safeExternalUrl } from '../utils/safeUrl';

interface LightboxProps {
    url: string;
    alt: string;
    width: number;
    height: number;
    placeholder?: string;
    onClose: () => void;
    onPrev?: () => void;
    onNext?: () => void;
    position?: string;
    autoPlay?: boolean;
    canContinue?: boolean;
}

const INTERVALS = [3, 5, 10];
const INTERVAL_KEY = 'slideshow-interval';

const readInterval = () => {
    const saved = Number(readLocal(INTERVAL_KEY));
    return INTERVALS.includes(saved) ? saved : 5;
};

const toolButton = 'grid size-11 place-items-center rounded-full bg-white/10 text-white backdrop-blur-sm transition-colors hover:bg-white/20 disabled:pointer-events-none disabled:opacity-30';

const Lightbox = ({ url, alt, width, height, placeholder, onClose, onPrev, onNext, position, autoPlay = false, canContinue = false }: LightboxProps) => {
    const [zoomed, setZoomed] = useState(false);
    const [playing, setPlaying] = useState(autoPlay);
    const [seconds, setSeconds] = useState(readInterval);
    const [loadedUrl, setLoadedUrl] = useState<string | null>(null);
    const [viewUrl, setViewUrl] = useState(url);
    const dialogRef = useRef<HTMLDivElement>(null);
    const scrollRef = useRef<HTMLDivElement>(null);
    const safeUrl = safeExternalUrl(url);
    const loaded = loadedUrl === url;

    if (viewUrl !== url) {
        setViewUrl(url);
        setZoomed(false);
    }

    useEffect(() => {
        const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        const { overflow } = document.body.style;
        document.body.style.overflow = 'hidden';
        dialogRef.current?.focus({ preventScroll: true });
        return () => {
            document.body.style.overflow = overflow;
            if (previous?.isConnected) previous.focus({ preventScroll: true });
        };
    }, []);

    const advance = useEffectEvent(() => onNext?.());
    const canAdvance = !!onNext;

    useEffect(() => {
        if (!playing || !loaded || zoomed || !canAdvance) return;
        const handle = window.setTimeout(() => advance(), seconds * 1000);
        return () => window.clearTimeout(handle);
    }, [playing, loaded, zoomed, canAdvance, seconds, url]);

    if (playing && !onNext && !canContinue) setPlaying(false);

    const togglePlay = () => {
        if (!playing) setZoomed(false);
        setPlaying(!playing);
    };

    const cycleInterval = () => {
        const next = INTERVALS[(INTERVALS.indexOf(seconds) + 1) % INTERVALS.length];
        setSeconds(next);
        writeLocal(INTERVAL_KEY, String(next));
    };

    useEffect(() => {
        if (!zoomed || !scrollRef.current) return;
        const el = scrollRef.current;
        el.scrollTo({ left: (el.scrollWidth - el.clientWidth) / 2, top: (el.scrollHeight - el.clientHeight) / 2 });
    }, [zoomed, viewUrl]);

    const onKeyDown = useEffectEvent((e: KeyboardEvent) => {
        if (e.altKey || e.ctrlKey || e.metaKey) return;
        switch (e.key) {
            case 'Escape':
                e.preventDefault();
                e.stopPropagation();
                onClose();
                break;
            case 'ArrowLeft':
                if (!zoomed && onPrev) { e.preventDefault(); onPrev(); }
                break;
            case 'ArrowRight':
                if (!zoomed && onNext) { e.preventDefault(); onNext(); }
                break;
            case ' ':
            case 'p':
            case 'P':
                if (!onNext && !canContinue) break;
                e.preventDefault();
                togglePlay();
                break;
            case 'z':
            case 'Z':
            case '+':
            case '-':
                e.preventDefault();
                setZoomed((z) => (e.key === '+' ? true : e.key === '-' ? false : !z));
                break;
            case 'Tab': {
                const items = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]), a[href]') ?? []);
                if (items.length === 0) break;
                const first = items[0];
                const last = items[items.length - 1];
                if (e.shiftKey && (document.activeElement === first || document.activeElement === dialogRef.current)) {
                    e.preventDefault();
                    last.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                    e.preventDefault();
                    first.focus();
                }
                break;
            }
        }
    });

    useEffect(() => {
        const listener = (e: KeyboardEvent) => onKeyDown(e);
        document.addEventListener('keydown', listener);
        return () => document.removeEventListener('keydown', listener);
    }, []);

    return createPortal(
        <div
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            aria-label={`${alt} – full screen`}
            tabIndex={-1}
            className="fixed inset-0 z-[90] flex flex-col bg-zinc-950/98 text-white outline-hidden animate-in fade-in"
        >
            <div className="pointer-events-none absolute inset-x-0 top-0 z-10 flex items-center justify-between gap-3 p-3 sm:p-4">
                <p className="pointer-events-auto rounded-full bg-white/10 px-3.5 py-1.5 text-sm font-bold backdrop-blur-sm tabular">
                    {width} × {height}{position ? <span className="text-white/60"> · {position}</span> : null}
                </p>
                <div className="pointer-events-auto flex items-center gap-2">
                    {(onNext || onPrev || canContinue) && (
                        <>
                            <button type="button" onClick={togglePlay} disabled={!onNext && !canContinue} className={toolButton} aria-pressed={playing} aria-label={playing ? 'Pause the slideshow (Space)' : 'Play a slideshow (Space)'} title={playing ? 'Pause (Space)' : 'Slideshow (Space)'}>
                                {playing ? <Pause size={19} aria-hidden="true" /> : <Play size={19} aria-hidden="true" />}
                            </button>
                            <button type="button" onClick={cycleInterval} className="flex h-11 items-center gap-1.5 rounded-full bg-white/10 px-3.5 text-xs font-bold text-white backdrop-blur-sm transition-colors hover:bg-white/20 tabular" aria-label={`Slideshow speed: ${seconds} seconds per image`} title="Seconds per image">
                                <Timer size={16} aria-hidden="true" />{seconds}s
                            </button>
                        </>
                    )}
                    <button type="button" onClick={() => setZoomed((z) => !z)} className={toolButton} aria-pressed={zoomed} aria-label={zoomed ? 'Fit to screen (Z)' : 'Show original size (Z)'} title={zoomed ? 'Fit to screen (Z)' : 'Original size (Z)'}>
                        {zoomed ? <ZoomOut size={19} aria-hidden="true" /> : <ZoomIn size={19} aria-hidden="true" />}
                    </button>
                    {safeUrl && (
                        <a href={safeUrl} target="_blank" rel="noopener noreferrer" className={toolButton} aria-label="Open the original file in a new tab" title="Open the original file">
                            <ExternalLinkIcon size={18} aria-hidden="true" />
                        </a>
                    )}
                    <button type="button" onClick={onClose} className={toolButton} aria-label="Close (Esc)" title="Close (Esc)">
                        <X size={20} aria-hidden="true" />
                    </button>
                </div>
            </div>

            <div
                ref={scrollRef}
                className={`relative flex-1 overscroll-contain ${zoomed ? 'overflow-auto' : 'overflow-hidden'}`}
            >
                <div className={zoomed ? 'grid min-h-full min-w-full place-items-center p-4' : 'flex size-full items-center justify-center p-3 pt-16 pb-16 sm:p-16'}>
                    {!loaded && (
                        <span className="pointer-events-none absolute inset-0 grid place-items-center" aria-hidden="true">
                            <Loader2 className="size-7 animate-spin text-white/60" />
                        </span>
                    )}
                    <button
                        type="button"
                        onClick={() => setZoomed((z) => !z)}
                        aria-label={zoomed ? 'Fit to screen' : 'Show original size'}
                        className={`block shrink-0 ${zoomed ? 'cursor-zoom-out' : 'max-h-full max-w-full cursor-zoom-in'}`}
                    >
                        <img
                            key={url}
                            src={url}
                            alt={alt}
                            width={width}
                            height={height}
                            decoding="async"
                            onLoad={() => setLoadedUrl(url)}
                            style={{ backgroundColor: placeholder }}
                            className={zoomed
                                ? 'block h-auto w-auto max-w-none rounded-sm'
                                : 'block max-h-[calc(100dvh-8rem)] w-auto max-w-full rounded-lg object-contain shadow-2xl sm:max-h-[calc(100dvh-8rem)]'}
                        />
                    </button>
                </div>
            </div>

            {playing && loaded && canAdvance && !zoomed && (
                <div className="pointer-events-none absolute inset-x-0 bottom-0 h-1 bg-white/10" aria-hidden="true">
                    <div key={`${url}-${seconds}`} className="slideshow-progress h-full origin-left bg-white/70" style={{ animationDuration: `${seconds}s` }} />
                </div>
            )}

            {(onPrev || onNext) && !zoomed && (
                <>
                    <button type="button" onClick={onPrev} disabled={!onPrev} className={`${toolButton} absolute left-3 top-1/2 -translate-y-1/2 sm:left-5`} aria-label="Previous image (←)" title="Previous image (←)">
                        <ChevronLeft size={22} aria-hidden="true" />
                    </button>
                    <button type="button" onClick={onNext} disabled={!onNext} className={`${toolButton} absolute right-3 top-1/2 -translate-y-1/2 sm:right-5`} aria-label="Next image (→)" title="Next image (→)">
                        <ChevronRight size={22} aria-hidden="true" />
                    </button>
                </>
            )}
        </div>,
        document.body,
    );
};

export default Lightbox;
