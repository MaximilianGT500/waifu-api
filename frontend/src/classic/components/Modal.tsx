import { X } from 'lucide-react';
import { ReactNode, useEffect, useEffectEvent, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

interface ModalProps {
    isOpen: boolean;
    onClose: () => void;
    title: string;
    children: ReactNode;
    footer?: ReactNode;
    maxWidth?: string;
    headerContent?: ReactNode;
    zIndex?: number;
}

const FOCUSABLE = [
    'a[href]',
    'area[href]',
    'button:not([disabled])',
    'input:not([disabled]):not([type="hidden"])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    'iframe',
    '[contenteditable="true"]',
    '[tabindex]:not([tabindex="-1"])',
].join(',');

const modalStack: symbol[] = [];

function focusableElements(container: HTMLElement): HTMLElement[] {
    return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        (el) => el.tabIndex >= 0 && el.getClientRects().length > 0 && !el.closest('[inert]')
    );
}

function currentFocus(): HTMLElement | null {
    const active = document.activeElement;
    return active instanceof HTMLElement && active !== document.body ? active : null;
}

const Modal = ({ isOpen, onClose, title, children, footer, maxWidth = 'max-w-md', headerContent, zIndex = 60 }: ModalProps) => {
    const titleId = useId();
    const panelRef = useRef<HTMLDivElement>(null);
    const [wasOpen, setWasOpen] = useState(isOpen);
    const [returnFocus, setReturnFocus] = useState<HTMLElement | null>(() => (isOpen ? currentFocus() : null));
    if (isOpen !== wasOpen) {
        setWasOpen(isOpen);
        if (isOpen) setReturnFocus(currentFocus());
    }

    const requestClose = useEffectEvent(() => onClose());

    useEffect(() => {
        if (!isOpen) return;
        const id = Symbol('modal');
        modalStack.push(id);
        const previouslyFocused = returnFocus;

        const panel = panelRef.current;
        if (panel && !panel.contains(document.activeElement)) {
            const initial = panel.querySelector<HTMLElement>('[data-autofocus]') ?? panel;
            initial.focus({ preventScroll: true });
        }

        const isTop = () => modalStack[modalStack.length - 1] === id;

        const onKeyDown = (e: KeyboardEvent) => {
            const current = panelRef.current;
            if (!isTop() || !current) return;
            if (e.key === 'Escape') {
                e.stopPropagation();
                requestClose();
                return;
            }
            if (e.key !== 'Tab') return;
            const items = focusableElements(current);
            if (items.length === 0) {
                e.preventDefault();
                current.focus();
                return;
            }
            const first = items[0];
            const last = items[items.length - 1];
            const active = document.activeElement;
            if (e.shiftKey && (active === first || active === current)) {
                e.preventDefault();
                last.focus();
            } else if (!e.shiftKey && active === last) {
                e.preventDefault();
                first.focus();
            } else if (!current.contains(active)) {
                e.preventDefault();
                first.focus();
            }
        };
        document.addEventListener('keydown', onKeyDown);

        return () => {
            document.removeEventListener('keydown', onKeyDown);
            const index = modalStack.lastIndexOf(id);
            if (index !== -1) modalStack.splice(index, 1);
            if (previouslyFocused && previouslyFocused.isConnected) {
                previouslyFocused.focus({ preventScroll: true });
            }
        };
    }, [isOpen, returnFocus]);

    if (!isOpen) return null;

    return createPortal(
        <div className="fixed inset-0 flex items-end justify-center sm:items-center sm:p-4" style={{ zIndex }}>
            <div
                className="absolute inset-0 bg-zinc-950/55 backdrop-blur-[2px] animate-in fade-in"
                onClick={onClose}
                aria-hidden="true"
            />

            <div
                ref={panelRef}
                role="dialog"
                aria-modal="true"
                aria-labelledby={titleId}
                tabIndex={-1}
                className={`relative flex max-h-[92dvh] w-full flex-col rounded-t-[1.25rem] border border-border bg-card text-card-foreground shadow-float outline-hidden animate-in fade-in slide-in-from-bottom-4 duration-300 sm:max-h-[88vh] sm:rounded-[1.25rem] ${maxWidth}`}
            >
                <div className="flex items-start justify-between gap-3 px-5 pb-3 pt-5 sm:px-6">
                    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5 pt-1">
                        <h2 id={titleId} className="text-lg font-bold leading-snug tracking-[-0.01em] wrap-break-word">{title}</h2>
                        {headerContent}
                    </div>
                    <button
                        type="button"
                        onClick={onClose}
                        aria-label="Close dialog"
                        className="-mr-1.5 grid size-9 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                    >
                        <X size={18} aria-hidden="true" />
                    </button>
                </div>

                <div className="overflow-y-auto overscroll-contain px-5 pb-6 pt-1 sm:px-6">
                    {children}
                </div>

                {footer && (
                    <div className="rounded-b-[1.25rem] border-t border-border bg-secondary/35 px-5 py-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:px-6 sm:pb-4">
                        {footer}
                    </div>
                )}
            </div>
        </div>,
        document.body
    );
};

export default Modal;
