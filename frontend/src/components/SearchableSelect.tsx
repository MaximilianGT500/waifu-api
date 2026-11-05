import { useState, useEffect, useEffectEvent, useRef, useCallback, useId } from 'react';
import { X, Plus, ChevronDown, Check, Loader2, Clock } from 'lucide-react';
import { useDebounce } from '../hooks/useDebounce';
import { ReviewStatus } from '../types';

export interface Option {
    id: number | string;
    name: string;
    description?: string;
    slug?: string;
    reviewStatus?: ReviewStatus;
    [key: string]: unknown;
}

interface SearchableSelectProps {
    options?: Option[];
    selectedOptions: Option[];
    onSelect: (option: Option) => void;
    onRemove: (option: Option) => void;
    onCreate?: (name: string) => void;
    loadOptions?: (query: string, page?: number) => Promise<Option[]>;
    placeholder?: string;
    label?: string;
    isMulti?: boolean;
    clearable?: boolean;
}

function sameOption(a: Option, b: Option): boolean {
    if (a.id !== undefined && b.id !== undefined) return String(a.id) === String(b.id);
    const aKey = a.slug || a.name;
    const bKey = b.slug || b.name;
    return aKey?.toLowerCase() === bKey?.toLowerCase();
}

const CREATE_INDEX = -1;

const SearchableSelect = ({
                              options = [],
                              selectedOptions,
                              onSelect,
                              onRemove,
                              onCreate,
                              loadOptions,
                              placeholder = "Select...",
                              label,
                              isMulti = true,
                              clearable = true
                          }: SearchableSelectProps) => {
    const [isOpen, setIsOpen] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const [asyncOptions, setAsyncOptions] = useState<Option[]>([]);
    const [isLoading, setIsLoading] = useState(false);
    const [page, setPage] = useState(1);
    const [hasMore, setHasMore] = useState(true);
    const [activeIndex, setActiveIndex] = useState<number | null>(null);
    const wrapperRef = useRef<HTMLDivElement>(null);
    const inputRef = useRef<HTMLInputElement>(null);
    const listRef = useRef<HTMLDivElement>(null);
    const suppressOpenRef = useRef(false);
    const baseId = useId();
    const labelId = `${baseId}-label`;
    const listId = `${baseId}-list`;
    const optionId = (index: number) => `${baseId}-option-${index === CREATE_INDEX ? 'create' : index}`;

    const isAsync = !!loadOptions;
    const loadPage = useEffectEvent((term: string, pageNumber: number) => loadOptions?.(term, pageNumber) ?? Promise.resolve([]));

    const debouncedSearchTerm = useDebounce(searchTerm, 300);

    useEffect(() => {
        const handleClickOutside = (event: PointerEvent) => {
            if (wrapperRef.current && !wrapperRef.current.contains(event.target as Node)) {
                setIsOpen(false);
            }
        };
        document.addEventListener('pointerdown', handleClickOutside);
        return () => document.removeEventListener('pointerdown', handleClickOutside);
    }, []);

    useEffect(() => {
        if (isOpen) {
            setPage(1);
            setHasMore(true);
            setAsyncOptions([]);
        }
        setActiveIndex(null);
    }, [debouncedSearchTerm, isOpen]);

    useEffect(() => {
        if (!isAsync || !isOpen) return;

        let isMounted = true;
        const fetchOptions = async () => {
            setIsLoading(true);
            try {
                const results = await loadPage(debouncedSearchTerm, page);
                if (isMounted) {
                    if (page === 1) {
                        setAsyncOptions(results);
                    } else {
                        setAsyncOptions(prev => {
                            const existingIds = new Set(prev.map(o => o.id));
                            const newOptions = results.filter(o => !existingIds.has(o.id));
                            return [...prev, ...newOptions];
                        });
                    }
                    setHasMore(results.length > 0);
                }
            } catch {
                if (isMounted) setHasMore(false);
            } finally {
                if (isMounted) setIsLoading(false);
            }
        };

        fetchOptions();

        return () => { isMounted = false; };
    }, [debouncedSearchTerm, isAsync, isOpen, page]);

    const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
        const { scrollTop, clientHeight, scrollHeight } = e.currentTarget;
        if (scrollHeight - scrollTop <= clientHeight + 50 && !isLoading && hasMore && isAsync) {
            setPage(prev => prev + 1);
        }
    }, [isLoading, hasMore, isAsync]);

    const displayedOptions = isAsync
        ? asyncOptions
        : options.filter(option => option.name.toLowerCase().includes(searchTerm.toLowerCase()));

    const showCreate = !!onCreate && !displayedOptions.some(o => o.name.toLowerCase() === searchTerm.toLowerCase());

    const handleOptionClick = (option: Option) => {
        const itemToRemove = selectedOptions.find(selected => sameOption(selected, option));
        if (itemToRemove) {
            onRemove(itemToRemove);
        } else {
            onSelect(option);
        }

        setSearchTerm('');
        if (!isMulti) {
            setIsOpen(false);
            suppressOpenRef.current = true;
            window.setTimeout(() => { suppressOpenRef.current = false; }, 0);
        }
        inputRef.current?.focus();
    };

    const handleCreate = () => {
        if (!onCreate) return;
        onCreate(searchTerm);
        setIsOpen(false);
        setSearchTerm('');
    };

    const scrollOptionIntoView = useEffectEvent((index: number) => {
        document.getElementById(optionId(index))?.scrollIntoView({ block: 'nearest' });
    });

    useEffect(() => {
        if (activeIndex === null || !isOpen) return;
        scrollOptionIntoView(activeIndex);
    }, [activeIndex, isOpen]);

    const moveActive = (delta: number) => {
        const first = showCreate ? CREATE_INDEX : 0;
        const last = displayedOptions.length - 1;
        if (last < first) return;
        setActiveIndex(prev => {
            if (prev === null) return delta > 0 ? first : last;
            const next = prev + delta;
            if (next < first) return last;
            if (next > last) return first;
            return next;
        });
    };

    const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                if (!isOpen) setIsOpen(true);
                else moveActive(1);
                break;
            case 'ArrowUp':
                e.preventDefault();
                if (!isOpen) setIsOpen(true);
                else moveActive(-1);
                break;
            case 'Enter':
                if (!isOpen || activeIndex === null) {
                    if (isOpen) e.preventDefault();
                    return;
                }
                e.preventDefault();
                if (activeIndex === CREATE_INDEX) handleCreate();
                else if (displayedOptions[activeIndex]) handleOptionClick(displayedOptions[activeIndex]);
                break;
            case 'Escape':
                if (isOpen) {
                    e.preventDefault();
                    e.stopPropagation();
                    setIsOpen(false);
                }
                break;
            case 'Backspace':
                if (!searchTerm && clearable && selectedOptions.length > 0) {
                    onRemove(selectedOptions[selectedOptions.length - 1]);
                }
                break;
            case 'Tab':
                setIsOpen(false);
                break;
        }
    };

    return (
        <div
            className="relative group"
            ref={wrapperRef}
            onBlur={(e) => {
                if (!wrapperRef.current?.contains(e.relatedTarget as Node | null)) setIsOpen(false);
            }}
        >
            {label && <label id={labelId} htmlFor={`${baseId}-input`} className="field-label">{label}</label>}

            <div
                className="flex min-h-11 cursor-text flex-wrap items-center gap-1.5 rounded-[var(--radius-control)] border-[1.5px] border-input bg-card p-1.5 pl-2 transition-[border-color,box-shadow] duration-150 hover:border-foreground/60 focus-within:border-brand focus-within:ring-4 focus-within:ring-brand/15"
                onClick={() => { setIsOpen(true); inputRef.current?.focus(); }}
            >
                {selectedOptions.length === 0 && !searchTerm && (
                    <span className="pointer-events-none absolute select-none px-1.5 text-sm text-muted-foreground/70" aria-hidden="true">{placeholder}</span>
                )}

                {selectedOptions.map(option => (
                    <span key={option.id} className={`flex max-w-full items-center gap-1 text-sm ${
                        !isMulti
                            ? `pl-1 font-medium ${option.reviewStatus === ReviewStatus.Pending ? 'text-warning' : 'text-foreground'}`
                            : `rounded-full py-0.5 pl-2.5 pr-1 font-bold animate-in fade-in zoom-in duration-200 ${option.reviewStatus === ReviewStatus.Pending ? 'bg-warning/12 text-warning' : 'bg-secondary text-secondary-foreground'} ${clearable ? '' : 'pr-2.5'}`
                    }`}>
                        {option.reviewStatus === ReviewStatus.Pending && <Clock size={12} className="shrink-0" aria-label="Pending review" />}
                        <span className="truncate">{option.name}</span>
                        {clearable && (
                            <button
                                type="button"
                                onClick={(e) => { e.stopPropagation(); onRemove(option); }}
                                aria-label={`Remove ${option.name}`}
                                className="grid size-5 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-foreground/10 hover:text-foreground"
                            >
                                <X size={12} aria-hidden="true" />
                            </button>
                        )}
                    </span>
                ))}

                <input
                    ref={inputRef}
                    id={`${baseId}-input`}
                    type="text"
                    role="combobox"
                    aria-expanded={isOpen}
                    aria-controls={listId}
                    aria-autocomplete="list"
                    aria-activedescendant={isOpen && activeIndex !== null ? optionId(activeIndex) : undefined}
                    aria-label={label ? undefined : placeholder}
                    autoComplete="off"
                    spellCheck={false}
                    className="h-7 min-w-8 flex-1 bg-transparent px-1 text-sm outline-none"
                    value={searchTerm}
                    onChange={(e) => { setSearchTerm(e.target.value); setIsOpen(true); }}
                    onFocus={() => { if (!suppressOpenRef.current) setIsOpen(true); }}
                    onKeyDown={handleKeyDown}
                />

                <div className="mr-1.5 text-muted-foreground" aria-hidden="true">
                    {isLoading ? <Loader2 size={16} className="animate-spin" /> : <ChevronDown size={16} />}
                </div>
            </div>

            {isOpen && (
                <div
                    id={listId}
                    role="listbox"
                    aria-multiselectable={isMulti || undefined}
                    aria-labelledby={label ? labelId : undefined}
                    aria-busy={isLoading}
                    className="absolute z-50 mt-2 flex max-h-64 w-full flex-col overflow-auto overscroll-contain rounded-[1.25rem] bg-popover p-1.5 text-popover-foreground shadow-float ring-1 ring-border animate-in fade-in slide-in-from-top-2 dark:ring-white/8"
                    onScroll={handleScroll}
                    tabIndex={-1}
                    ref={listRef}
                >
                    {showCreate && (
                        <div
                            id={optionId(CREATE_INDEX)}
                            role="option"
                            aria-selected={false}
                            className={`sticky top-0 z-10 mb-1 flex cursor-pointer items-center gap-2 rounded-full px-3 py-2 text-sm font-bold text-brand-ink ${
                                activeIndex === CREATE_INDEX ? 'bg-brand-soft' : 'bg-popover hover:bg-brand-soft'
                            }`}
                            onMouseEnter={() => setActiveIndex(CREATE_INDEX)}
                            onClick={handleCreate}
                        >
                            <Plus size={16} aria-hidden="true" /> {searchTerm ? `Create "${searchTerm}"` : "Create New"}
                        </div>
                    )}

                    {displayedOptions.map((option, index) => {
                        const isSelected = selectedOptions.some(s => sameOption(s, option));
                        const isPending = option.reviewStatus === ReviewStatus.Pending;
                        const isActive = activeIndex === index;

                        return (
                            <div
                                key={option.id}
                                id={optionId(index)}
                                role="option"
                                aria-selected={isSelected}
                                className={`flex cursor-pointer items-center justify-between rounded-full px-3 py-2 text-sm transition-colors duration-100 ${
                                    isSelected
                                        ? 'bg-brand-soft font-medium text-brand-ink'
                                        : isActive ? 'bg-secondary text-foreground' : 'text-foreground/85 hover:bg-secondary'
                                } ${isActive && isSelected ? 'ring-1 ring-inset ring-brand/40' : ''}`}
                                onMouseEnter={() => setActiveIndex(index)}
                                onClick={() => handleOptionClick(option)}
                            >
                                <div className="flex items-center gap-2 overflow-hidden">
                                    {isSelected && <Check size={14} className="shrink-0" aria-hidden="true" />}
                                    {isPending && <Clock size={12} className="shrink-0 text-warning" aria-label="Pending review" />}
                                    <span className="truncate">{option.name}</span>
                                </div>
                                {option.description && <span className="ml-3 max-w-[40%] truncate text-xs text-muted-foreground">{option.description}</span>}
                            </div>
                        );
                    })}

                    {displayedOptions.length === 0 && !searchTerm && !isLoading && !onCreate && (
                        <div className="px-3 py-6 text-center text-sm text-muted-foreground">No options found.</div>
                    )}

                    {isLoading && (
                         <div className="flex items-center justify-center gap-2 px-3 py-2.5 text-center text-sm text-muted-foreground" role="status">
                             <Loader2 size={14} className="animate-spin" aria-hidden="true" /> Loading...
                         </div>
                    )}
                </div>
            )}
        </div>
    );
};

export default SearchableSelect;
