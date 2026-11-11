import { ReactNode, useEffect, useEffectEvent, useId, useRef, useState, KeyboardEvent as ReactKeyboardEvent } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight, Bookmark, Brush, Flame, Hash, Keyboard, Loader2, LucideIcon, Moon, Palette, Search, Shuffle, Sun, Tag as TagIcon } from 'lucide-react';
import api from '../services/api';
import { Artist, ImageDto, ImageOrderBy, PaginatedList, Tag } from '../types';
import { useDebounce } from '../hooks/useDebounce';
import { useTheme } from '../context/ThemeContext';
import { useNotification } from '../context/NotificationContext';
import { useSavedSearches } from '../hooks/useSavedSearches';
import { savedSearchHref } from '../utils/savedSearches';
import { openWelcome } from '../utils/welcome';

export interface PaletteDestination {
    key: string;
    label: string;
    to: string;
    icon: LucideIcon;
}

interface CommandPaletteProps {
    onClose: () => void;
    destinations: PaletteDestination[];
    onShowShortcuts: () => void;
}

interface Entry {
    id: string;
    group: string;
    label: ReactNode;
    detail?: ReactNode;
    icon: ReactNode;
    run: () => void | Promise<void>;
}

const IMAGE_ID = /^#?\s*([1-9]\d{0,17})$/;

const fetchList = async <T,>(endpoint: string, params: Record<string, string | number>, signal: AbortSignal) => {
    const { data } = await api.get<PaginatedList<T>>(endpoint, { params, skipGlobalErrorHandler: true, signal });
    return data.items;
};

const CommandPalette = ({ onClose, destinations, onShowShortcuts }: CommandPaletteProps) => {
    const navigate = useNavigate();
    const { resolvedTheme, setTheme } = useTheme();
    const { showNotification } = useNotification();
    const savedSearches = useSavedSearches();
    const [query, setQuery] = useState('');
    const [active, setActive] = useState(0);
    const [busy, setBusy] = useState(false);
    const inputRef = useRef<HTMLInputElement>(null);
    const listId = useId();
    const trimmed = query.trim();
    const debounced = useDebounce(trimmed, 180);
    const searching = debounced !== trimmed;

    const tags = useQuery({
        queryKey: ['palette', 'tags', debounced],
        staleTime: 60_000,
        queryFn: ({ signal }) => fetchList<Tag>('/tags', debounced ? { name: debounced, pageSize: 6 } : { pageSize: 8 }, signal),
    });
    const artists = useQuery({
        queryKey: ['palette', 'artists', debounced],
        enabled: debounced.length > 0,
        staleTime: 60_000,
        queryFn: ({ signal }) => fetchList<Artist>('/artists', { name: debounced, pageSize: 5 }, signal),
    });

    const requestClose = useEffectEvent(() => onClose());

    useEffect(() => {
        const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        inputRef.current?.focus();
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') requestClose();
        };
        document.addEventListener('keydown', onKeyDown);
        return () => {
            document.removeEventListener('keydown', onKeyDown);
            if (previous?.isConnected) previous.focus({ preventScroll: true });
        };
    }, []);

    const go = (to: string) => {
        onClose();
        navigate(to);
    };

    const randomImage = async () => {
        setBusy(true);
        try {
            const { data } = await api.get<PaginatedList<ImageDto>>('/images', { params: { orderBy: ImageOrderBy.Random, pageSize: 1 }, skipGlobalErrorHandler: true });
            if (data.items[0]) go(`/images/${data.items[0].id}`);
            else showNotification('info', 'There are no images yet');
        } catch {
            showNotification('error', 'Could not fetch a random image');
        } finally {
            setBusy(false);
        }
    };

    const needle = trimmed.toLowerCase();
    const entries: Entry[] = [];
    const idMatch = IMAGE_ID.exec(trimmed);
    if (idMatch) {
        entries.push({ id: `image-${idMatch[1]}`, group: 'Image', label: <>Open image <span className="tabular">#{idMatch[1]}</span></>, icon: <Hash size={16} />, run: () => go(`/images/${idMatch[1]}`) });
    }
    const matchingSearches = savedSearches.filter((search) => !needle || search.name.toLowerCase().includes(needle)).slice(0, needle ? 5 : 3);
    for (const search of matchingSearches) {
        entries.push({ id: `saved-${search.id}`, group: 'Saved searches', label: search.name, icon: <Bookmark size={16} />, run: () => go(savedSearchHref(search)) });
    }
    for (const tag of tags.data ?? []) {
        entries.push({
            id: `tag-${tag.id}`,
            group: trimmed ? 'Tags' : 'Popular tags',
            label: tag.name,
            detail: tag.imageCount ? `${tag.imageCount.toLocaleString()} images` : undefined,
            icon: <TagIcon size={16} />,
            run: () => go(`/tags/${encodeURIComponent(tag.slug)}`),
        });
    }
    if (trimmed) {
        for (const artist of artists.data ?? []) {
            entries.push({
                id: `artist-${artist.id}`,
                group: 'Artists',
                label: artist.name,
                detail: artist.imageCount ? `${artist.imageCount.toLocaleString()} images` : undefined,
                icon: <Palette size={16} />,
                run: () => go(`/artists/${artist.id}`),
            });
        }
    }
    for (const page of destinations) {
        if (needle && !page.label.toLowerCase().includes(needle)) continue;
        const Icon = page.icon;
        entries.push({ id: `page-${page.key}`, group: 'Pages', label: page.label, icon: <Icon size={16} />, run: () => go(page.to) });
    }
    const actions: Entry[] = [
        { id: 'action-random', group: 'Actions', label: 'Draw a random image', icon: <Shuffle size={16} />, run: randomImage },
        {
            id: 'action-theme',
            group: 'Actions',
            label: resolvedTheme === 'dark' ? 'Switch to the light theme' : 'Switch to the dark theme',
            icon: resolvedTheme === 'dark' ? <Sun size={16} /> : <Moon size={16} />,
            run: () => { setTheme(resolvedTheme === 'dark' ? 'light' : 'dark'); onClose(); },
        },
        { id: 'action-trending', group: 'Actions', label: 'Show what is trending this week', icon: <Flame size={16} />, run: () => go(`/gallery?orderBy=${ImageOrderBy.Trending}`) },
        { id: 'action-look', group: 'Actions', label: 'Change the design and colors', icon: <Brush size={16} />, run: () => { onClose(); openWelcome(); } },
        { id: 'action-shortcuts', group: 'Actions', label: 'Keyboard shortcuts', icon: <Keyboard size={16} />, run: () => { onClose(); onShowShortcuts(); } },
    ];
    for (const action of actions) {
        if (!needle || String(action.label).toLowerCase().includes(needle)) entries.push(action);
    }

    const activeIndex = entries.length === 0 ? -1 : Math.min(active, entries.length - 1);
    const activeEntry = activeIndex >= 0 ? entries[activeIndex] : undefined;
    const loading = searching || tags.isFetching || artists.isFetching;

    const activeId = activeEntry?.id;
    useEffect(() => {
        if (!activeId) return;
        document.getElementById(`${listId}-${activeId}`)?.scrollIntoView({ block: 'nearest' });
    }, [activeId, listId]);

    const run = (entry: Entry | undefined) => {
        if (!entry || busy) return;
        void entry.run();
    };

    const onKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                if (entries.length) setActive((activeIndex + 1) % entries.length);
                break;
            case 'ArrowUp':
                e.preventDefault();
                if (entries.length) setActive((activeIndex - 1 + entries.length) % entries.length);
                break;
            case 'Home':
                if (e.ctrlKey) { e.preventDefault(); setActive(0); }
                break;
            case 'End':
                if (e.ctrlKey) { e.preventDefault(); setActive(entries.length - 1); }
                break;
            case 'Enter':
                e.preventDefault();
                run(activeEntry);
                break;
            case 'Escape':
                e.preventDefault();
                e.stopPropagation();
                onClose();
                break;
            case 'Tab':
                e.preventDefault();
                break;
        }
    };

    let lastGroup = '';

    return createPortal(
        <div className="fixed inset-0 z-[70] flex items-start justify-center p-3 pt-[min(12vh,6rem)] sm:p-6 sm:pt-[12vh]">
            <div className="absolute inset-0 bg-zinc-950/55 backdrop-blur-[2px] animate-in fade-in" aria-hidden="true" onClick={onClose} />
            <div
                role="dialog"
                aria-modal="true"
                aria-label="Quick search"
                className="relative flex max-h-[min(36rem,80dvh)] w-full max-w-xl flex-col overflow-hidden rounded-[var(--radius-sheet)] bg-popover text-popover-foreground shadow-float animate-in fade-in slide-in-from-top-2 duration-200 dark:ring-1 dark:ring-white/8"
            >
                <div className="flex items-center gap-3 border-b-2 border-dashed border-border px-5">
                    {loading || busy
                        ? <Loader2 size={18} className="shrink-0 animate-spin text-muted-foreground" aria-hidden="true" />
                        : <Search size={18} className="shrink-0 text-muted-foreground" aria-hidden="true" />}
                    <input
                        ref={inputRef}
                        type="text"
                        role="combobox"
                        aria-expanded="true"
                        aria-controls={listId}
                        aria-autocomplete="list"
                        aria-activedescendant={activeEntry ? `${listId}-${activeEntry.id}` : undefined}
                        aria-label="Search tags, artists, image numbers and pages"
                        placeholder="Search tags, artists, #image or pages…"
                        autoComplete="off"
                        autoCorrect="off"
                        spellCheck={false}
                        enterKeyHint="go"
                        value={query}
                        onChange={(e) => { setQuery(e.target.value); setActive(0); }}
                        onKeyDown={onKeyDown}
                        className="h-14 min-w-0 flex-1 bg-transparent text-base font-bold outline-none placeholder:font-medium placeholder:text-muted-foreground/80"
                    />
                    <button type="button" onClick={onClose} className="shrink-0 rounded-md border border-border px-1.5 py-0.5 font-mono text-[0.6875rem] font-bold text-muted-foreground transition-colors hover:text-foreground" aria-label="Close quick search">
                        Esc
                    </button>
                </div>

                <div id={listId} role="listbox" aria-label="Results" className="flex-1 overflow-y-auto overscroll-contain p-2">
                    {entries.length === 0 && !loading && (
                        <p className="px-3 py-8 text-center text-sm text-muted-foreground">Nothing found for “{trimmed}”.</p>
                    )}
                    {entries.map((entry, index) => {
                        const header = entry.group !== lastGroup ? entry.group : null;
                        lastGroup = entry.group;
                        const selected = index === activeIndex;
                        return (
                            <div key={entry.id} role="presentation">
                                {header && <p role="presentation" className="px-3 pb-1 pt-3 text-xs font-bold text-muted-foreground first:pt-1">{header}</p>}
                                <div
                                    id={`${listId}-${entry.id}`}
                                    role="option"
                                    aria-selected={selected}
                                    tabIndex={-1}
                                    onPointerMove={() => { if (!selected) setActive(index); }}
                                    onClick={() => run(entry)}
                                    onKeyDown={(e) => { if (e.key === 'Enter') run(entry); }}
                                    className={`flex cursor-pointer items-center gap-3 rounded-full px-3 py-2 text-sm font-bold transition-colors ${selected ? 'bg-brand-soft text-brand-ink' : 'text-foreground/85'}`}
                                >
                                    <span aria-hidden="true" className={`grid size-5 shrink-0 place-items-center ${selected ? 'text-brand-ink' : 'text-muted-foreground'}`}>{entry.icon}</span>
                                    <span className="min-w-0 flex-1 truncate">{entry.label}</span>
                                    {entry.detail && <span className="shrink-0 text-xs font-medium text-muted-foreground tabular">{entry.detail}</span>}
                                    {selected && <ArrowRight size={15} aria-hidden="true" className="shrink-0" />}
                                </div>
                            </div>
                        );
                    })}
                </div>

                <div className="hidden items-center gap-4 border-t-2 border-dashed border-border px-5 py-2.5 text-xs text-muted-foreground sm:flex">
                    <span><kbd className="font-mono font-bold">↑</kbd> <kbd className="font-mono font-bold">↓</kbd> to move</span>
                    <span><kbd className="font-mono font-bold">Enter</kbd> to open</span>
                    <button type="button" onClick={() => { onClose(); onShowShortcuts(); }} className="ml-auto font-bold transition-colors hover:text-foreground">
                        All shortcuts <kbd className="font-mono">?</kbd>
                    </button>
                </div>
            </div>
        </div>,
        document.body,
    );
};

export default CommandPalette;
