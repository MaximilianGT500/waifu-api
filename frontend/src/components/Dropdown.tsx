import {
    createContext,
    KeyboardEvent as ReactKeyboardEvent,
    MouseEvent as ReactMouseEvent,
    ReactNode,
    useContext,
    useEffect,
    useEffectEvent,
    useId,
    useRef,
    useState,
} from 'react';
import { Link } from 'react-router';
import { Check } from 'lucide-react';

export type TriggerProps = {
    type: 'button';
    onClick: (e: ReactMouseEvent<HTMLButtonElement>) => void;
    onKeyDown: (e: ReactKeyboardEvent<HTMLButtonElement>) => void;
    'aria-haspopup': 'menu';
    'aria-expanded': boolean;
    'aria-controls'?: string;
    'aria-label'?: string;
};

const TriggerContext = createContext<TriggerProps | null>(null);

export const DropdownTrigger = ({ className, children }: { className?: string; children: ReactNode }) => {
    const props = useContext(TriggerContext);
    return <button {...props} type="button" className={className}>{children}</button>;
};

interface DropdownProps {
    trigger: ReactNode;
    children: ReactNode;
    align?: 'left' | 'right';
    width?: string;
    className?: string;
    isOpen?: boolean;
    onClose?: () => void;
    onOpen?: () => void;
    padding?: string;
    label?: string;
}

const ITEM_SELECTOR = '[role="menuitem"]:not([aria-disabled="true"]), [role="menuitemradio"]:not([aria-disabled="true"]), [role="menuitemcheckbox"]:not([aria-disabled="true"])';

const menuItems = (menu: HTMLElement | null) => Array.from(menu?.querySelectorAll<HTMLElement>(ITEM_SELECTOR) ?? []);

export const Dropdown = ({
    trigger,
    children,
    align = 'right',
    width = 'w-48',
    className = '',
    isOpen: controlledIsOpen,
    onClose,
    onOpen,
    padding = 'p-1.5',
    label,
}: DropdownProps) => {
    const [internalIsOpen, setInternalIsOpen] = useState(false);
    const [focusFirst, setFocusFirst] = useState(false);
    const ref = useRef<HTMLDivElement>(null);
    const menuRef = useRef<HTMLDivElement>(null);
    const triggerRef = useRef<HTMLElement | null>(null);
    const menuId = useId();

    const isControlled = controlledIsOpen !== undefined;
    const isOpen = isControlled ? controlledIsOpen : internalIsOpen;

    const setOpen = (next: boolean) => {
        if (isControlled) {
            if (next) onOpen?.();
            else onClose?.();
        } else {
            setInternalIsOpen(next);
        }
    };

    const close = (restoreFocus = false) => {
        setOpen(false);
        setFocusFirst(false);
        if (restoreFocus) triggerRef.current?.focus();
    };

    const onOutside = useEffectEvent((target: EventTarget | null) => {
        if (ref.current && !ref.current.contains(target as Node)) close();
    });

    useEffect(() => {
        if (!isOpen) return;
        const handlePointerDown = (event: PointerEvent) => onOutside(event.target);
        const handleFocusIn = (event: FocusEvent) => onOutside(event.target);
        document.addEventListener('pointerdown', handlePointerDown);
        document.addEventListener('focusin', handleFocusIn);
        return () => {
            document.removeEventListener('pointerdown', handlePointerDown);
            document.removeEventListener('focusin', handleFocusIn);
        };
    }, [isOpen]);

    useEffect(() => {
        if (isOpen && focusFirst) menuItems(menuRef.current)[0]?.focus();
    }, [isOpen, focusFirst]);

    const handleMenuKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
        const list = menuItems(menuRef.current);
        const index = list.indexOf(document.activeElement as HTMLElement);
        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                list[(index + 1) % list.length]?.focus();
                break;
            case 'ArrowUp':
                e.preventDefault();
                list[(index - 1 + list.length) % list.length]?.focus();
                break;
            case 'Home':
                e.preventDefault();
                list[0]?.focus();
                break;
            case 'End':
                e.preventDefault();
                list[list.length - 1]?.focus();
                break;
            case 'Escape':
                e.preventDefault();
                e.stopPropagation();
                close(true);
                break;
            case 'Tab':
                close();
                break;
        }
    };

    const handleContentClick = (e: ReactMouseEvent) => {
        const target = e.target as HTMLElement;
        if (target.closest('a') || target.closest('[data-dropdown-close]')) close();
    };

    const handleTriggerClick = (e: ReactMouseEvent<HTMLButtonElement>) => {
        triggerRef.current = e.currentTarget;
        if (e.detail > 0) setFocusFirst(false);
        setOpen(!isOpen);
    };

    const handleTriggerKeyDown = (e: ReactKeyboardEvent<HTMLButtonElement>) => {
        triggerRef.current = e.currentTarget;
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            if (!isOpen) {
                setFocusFirst(true);
                setOpen(true);
            } else {
                menuItems(menuRef.current)[0]?.focus();
            }
        } else if (e.key === 'Enter' || e.key === ' ') {
            setFocusFirst(!isOpen);
        } else if (e.key === 'Escape' && isOpen) {
            e.preventDefault();
            e.stopPropagation();
            close(true);
        }
    };

    const triggerProps: TriggerProps = {
        type: 'button',
        onClick: handleTriggerClick,
        onKeyDown: handleTriggerKeyDown,
        'aria-haspopup': 'menu',
        'aria-expanded': isOpen,
        'aria-controls': isOpen ? menuId : undefined,
        ...(label ? { 'aria-label': label } : {}),
    };

    return (
        <div
            className="relative"
            ref={ref}
            onFocusCapture={(e) => {
                if (!menuRef.current?.contains(e.target as Node)) triggerRef.current = e.target as HTMLElement;
            }}
        >
            <TriggerContext.Provider value={triggerProps}>{trigger}</TriggerContext.Provider>
            {isOpen && (
                <div
                    ref={menuRef}
                    id={menuId}
                    role="menu"
                    tabIndex={-1}
                    aria-orientation="vertical"
                    className={`absolute z-50 mt-2 origin-top overflow-hidden rounded-[1.25rem] bg-popover text-popover-foreground shadow-float outline-hidden ring-1 ring-border animate-in fade-in zoom-in-95 duration-200 dark:ring-white/8 ${padding} ${align === 'right' ? 'right-0 origin-top-right' : 'left-0 origin-top-left'} ${width} ${className}`}
                    onClick={handleContentClick}
                    onKeyDown={handleMenuKeyDown}
                >
                    {children}
                </div>
            )}
        </div>
    );
};

interface DropdownItemProps {
    children: ReactNode;
    onClick?: (e: ReactMouseEvent) => void;
    className?: string;
    active?: boolean;
    icon?: ReactNode;
    danger?: boolean;
    to?: string;
    checkbox?: boolean;
}

export const DropdownItem = ({ children, onClick, className = '', active, icon, danger = false, to, checkbox = false }: DropdownItemProps) => {
    const selected = !!active && !checkbox;
    const tone = danger
        ? 'text-destructive hover:bg-destructive/10 focus-visible:bg-destructive/10'
        : selected
            ? 'bg-brand-soft text-brand-ink'
            : 'text-foreground/85 hover:bg-secondary hover:text-foreground focus-visible:bg-secondary focus-visible:text-foreground';
    const classes = `flex w-full cursor-pointer items-center gap-2.5 rounded-full px-3 py-2 text-left text-sm font-bold outline-hidden transition-colors duration-100 ${tone} ${className}`;
    const isChoice = active !== undefined && !checkbox;
    const content = (
        <>
            {icon && (
                <span aria-hidden="true" className={`grid size-5 shrink-0 place-items-center ${danger ? 'text-destructive' : selected ? 'text-brand-ink' : 'text-muted-foreground'}`}>
                    {icon}
                </span>
            )}
            <span className="flex min-w-0 flex-1 items-center gap-2">{children}</span>
            {isChoice && active && <Check size={15} aria-hidden="true" className="shrink-0" />}
            {checkbox && (
                <span aria-hidden="true" className={`relative h-5 w-9 shrink-0 rounded-full transition-colors ${active ? 'bg-brand' : 'bg-muted-foreground/45'}`}>
                    <span className={`absolute left-0.5 top-0.5 size-4 rounded-full bg-white shadow-sm transition-transform ${active ? 'translate-x-4' : ''}`} />
                </span>
            )}
        </>
    );

    if (to) {
        return (
            <Link to={to} role="menuitem" tabIndex={-1} onClick={onClick} data-dropdown-close className={classes}>
                {content}
            </Link>
        );
    }
    return (
        <button
            type="button"
            role={checkbox ? 'menuitemcheckbox' : isChoice ? 'menuitemradio' : 'menuitem'}
            aria-checked={checkbox ? !!active : isChoice ? active : undefined}
            tabIndex={-1}
            onClick={onClick}
            data-dropdown-close
            className={classes}
        >
            {content}
        </button>
    );
};

export const DropdownSwatch = ({ color, label, active, onClick }: { color: string; label: string; active: boolean; onClick: () => void }) => (
    <button
        type="button"
        role="menuitemradio"
        aria-checked={active}
        aria-label={label}
        title={label}
        tabIndex={-1}
        onClick={onClick}
        className={`grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-white outline-hidden ring-offset-2 ring-offset-popover transition-shadow focus-visible:ring-2 focus-visible:ring-ring ${active ? 'ring-2 ring-foreground/70' : 'ring-1 ring-black/10 dark:ring-white/20'}`}
        style={{ backgroundColor: color }}
    >
        {active && <Check size={15} strokeWidth={3} aria-hidden="true" />}
    </button>
);

export const DropdownSeparator = () => <hr className="mx-2 my-1.5 border-0 border-t-2 border-dashed border-border" />;

export const DropdownLabel = ({ children }: { children: ReactNode }) => (
    <div role="presentation" className="px-3 pb-1 pt-2 text-xs font-bold text-muted-foreground">
        {children}
    </div>
);
