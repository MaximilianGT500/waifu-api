import { useState, useId } from 'react';
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-react';

interface PaginationProps {
    currentPage: number;
    totalPages: number;
    setPage: (page: number) => void;
    pageSize?: number;
    setPageSize?: (size: number) => void;
    maxPageSize?: number;
    scrollTargetId?: string;
}

const stepClass = 'grid size-9 place-items-center rounded-xl text-foreground transition-colors hover:bg-secondary disabled:pointer-events-none disabled:opacity-35';
const numberInputClass = 'h-9 rounded-xl border border-input bg-card text-center text-sm font-semibold text-foreground tabular outline-none transition-[border-color,box-shadow] focus:border-brand focus:ring-4 focus:ring-brand/15';

const Pagination = ({ currentPage, totalPages, setPage, pageSize, setPageSize, maxPageSize = 100, scrollTargetId }: PaginationProps) => {
    const [inputVal, setInputVal] = useState(currentPage.toString());
    const [syncedPage, setSyncedPage] = useState(currentPage);
    const [pageSizeInput, setPageSizeInput] = useState(pageSize?.toString() ?? '');
    const [syncedPageSize, setSyncedPageSize] = useState(pageSize);
    const pageInputId = useId();
    const pageSizeInputId = useId();

    if (syncedPage !== currentPage) {
        setSyncedPage(currentPage);
        setInputVal(currentPage.toString());
    }
    if (syncedPageSize !== pageSize) {
        setSyncedPageSize(pageSize);
        if (pageSize !== undefined) setPageSizeInput(pageSize.toString());
    }

    const changePage = (page: number) => {
        if (page === currentPage) return;
        setPage(page);
        const target = scrollTargetId ? document.getElementById(scrollTargetId) : null;
        if (target) {
            target.scrollIntoView({ behavior: 'instant' as ScrollBehavior });
        } else {
            const scroller = document.querySelector('main') ?? document.scrollingElement;
            scroller?.scrollTo({ top: 0, behavior: 'instant' as ScrollBehavior });
            window.scrollTo({ top: 0, behavior: 'instant' as ScrollBehavior });
        }
    };

    if (totalPages <= 1 && !setPageSize) return null;

    const commitPageInput = () => {
        const val = parseInt(inputVal, 10);
        if (!isNaN(val) && val >= 1 && val <= totalPages) {
            changePage(val);
        } else {
            setInputVal(currentPage.toString());
        }
    };

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        commitPageInput();
    };

    const commitPageSize = () => {
        if (!setPageSize || pageSize === undefined) return;
        const val = parseInt(pageSizeInput, 10);
        if (!isNaN(val) && val >= 1) {
            const clamped = Math.min(val, maxPageSize);
            setPageSizeInput(clamped.toString());
            if (clamped !== pageSize) setPageSize(clamped);
        } else {
            setPageSizeInput(pageSize.toString());
        }
    };

    return (
        <nav aria-label="Pagination" className="flex select-none flex-wrap items-center justify-center gap-x-6 gap-y-3 py-8">
            {totalPages > 1 && (
                <div className="flex items-center gap-1 rounded-2xl border border-border bg-card p-1">
                    <button type="button" onClick={() => changePage(1)} disabled={currentPage === 1} className={stepClass} title="First page" aria-label="First page">
                        <ChevronsLeft size={17} aria-hidden="true" />
                    </button>
                    <button type="button" onClick={() => changePage(Math.max(1, currentPage - 1))} disabled={currentPage === 1} className={stepClass} title="Previous page" aria-label="Previous page">
                        <ChevronLeft size={17} aria-hidden="true" />
                    </button>

                    <form onSubmit={handleSubmit} className="mx-1.5 flex items-center gap-2 text-sm">
                        <label htmlFor={pageInputId} className="sr-only">Page number, {totalPages} pages in total</label>
                        <input
                            id={pageInputId}
                            type="text"
                            inputMode="numeric"
                            pattern="[0-9]*"
                            autoComplete="off"
                            maxLength={7}
                            value={inputVal}
                            onChange={(e) => setInputVal(e.target.value.replace(/\D/g, ''))}
                            onBlur={commitPageInput}
                            enterKeyHint="go"
                            aria-current="page"
                            className={`${numberInputClass} w-14`}
                        />
                        <span className="whitespace-nowrap text-muted-foreground tabular" aria-hidden="true">of {totalPages}</span>
                    </form>

                    <button type="button" onClick={() => changePage(Math.min(totalPages, currentPage + 1))} disabled={currentPage === totalPages} className={stepClass} title="Next page" aria-label="Next page">
                        <ChevronRight size={17} aria-hidden="true" />
                    </button>
                    <button type="button" onClick={() => changePage(totalPages)} disabled={currentPage === totalPages} className={stepClass} title="Last page" aria-label={`Last page (${totalPages})`}>
                        <ChevronsRight size={17} aria-hidden="true" />
                    </button>
                </div>
            )}

            {setPageSize && pageSize !== undefined && (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <label htmlFor={pageSizeInputId}>Per page</label>
                    <input
                        id={pageSizeInputId}
                        type="text"
                        inputMode="numeric"
                        pattern="[0-9]*"
                        autoComplete="off"
                        maxLength={4}
                        enterKeyHint="done"
                        value={pageSizeInput}
                        aria-describedby={`${pageSizeInputId}-hint`}
                        onChange={(e) => setPageSizeInput(e.target.value.replace(/\D/g, ''))}
                        onBlur={commitPageSize}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                                e.preventDefault();
                                commitPageSize();
                            }
                        }}
                        className={`${numberInputClass} w-14`}
                    />
                    <span id={`${pageSizeInputId}-hint`} className="sr-only">Between 1 and {maxPageSize}</span>
                </div>
            )}
        </nav>
    );
};

export default Pagination;
