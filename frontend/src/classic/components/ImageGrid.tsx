import { ReactNode, useState, useLayoutEffect, useRef } from 'react';
import { ImageDto, Role } from '../../types';
import ImageCard from './ImageCard';
import { RefreshCw, CloudOff } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';

export interface ImageGridProps {
    images: ImageDto[];
    isLoading: boolean;
    error?: unknown;
    onRetry?: () => void;
    onEdit?: (image: ImageDto) => void;
    onSuggest?: (image: ImageDto) => void;
    onDelete?: (id: number) => void;
    onRemove?: (id: number) => void;
    emptyState?: ReactNode;
    forceOverlay?: boolean;
    readOnly?: boolean;
    showTags?: boolean;
}

const SKELETON_HEIGHTS = [14, 18, 11, 16];

const getColumnCount = (width: number) => {
    if (width >= 1680) return 6;
    if (width >= 1320) return 5;
    if (width >= 1000) return 4;
    if (width >= 700) return 3;
    return 2;
};

const useColumnCount = () => {
    const ref = useRef<HTMLDivElement>(null);
    const [count, setCount] = useState(() =>
        typeof window !== 'undefined' ? getColumnCount(window.innerWidth - (window.innerWidth >= 1024 ? 288 : 32)) : 4
    );
    useLayoutEffect(() => {
        const el = ref.current;
        if (!el) return;
        const update = () => setCount(getColumnCount(el.clientWidth));
        update();
        const observer = new ResizeObserver(update);
        observer.observe(el);
        return () => observer.disconnect();
    }, []);
    return [ref, count] as const;
};

const ImageGrid = ({
                       images, isLoading, error, onRetry,
                       onEdit, onSuggest, onDelete, onRemove,
                       emptyState, forceOverlay = false, readOnly = false,
                       showTags = false
                   }: ImageGridProps) => {
    const { user } = useAuth();
    const [gridRef, colCount] = useColumnCount();
    const isAdminOrModerator = user && (user.role === Role.Admin || user.role === Role.Moderator);
    const isAdmin = user && user.role === Role.Admin;

    let content: ReactNode;
    if (isLoading) {
        content = (
            <div className="flex items-start gap-3 sm:gap-4" aria-busy="true" aria-label="Loading images">
                {Array.from({ length: colCount }, (_col, c) => (
                    <div key={c} className="flex min-w-0 flex-1 flex-col gap-3 sm:gap-4">
                        {SKELETON_HEIGHTS.map((_height, i) => (
                            <div key={i} className="animate-pulse rounded-[1.25rem] bg-secondary" style={{ height: `${SKELETON_HEIGHTS[(i + c) % SKELETON_HEIGHTS.length]}rem` }} />
                        ))}
                    </div>
                ))}
            </div>
        );
    } else if (error) {
        content = (
            <div className="flex min-h-64 flex-col items-center justify-center px-4 py-16 text-center">
                <span className="mb-4 grid size-12 place-items-center rounded-2xl bg-secondary text-muted-foreground" aria-hidden="true">
                    <CloudOff size={22} />
                </span>
                <p className="font-semibold">The images could not be loaded</p>
                <p className="mt-1 text-sm text-muted-foreground">The server did not respond. Try again in a moment.</p>
                {onRetry && (
                    <button type="button" onClick={onRetry} className="btn btn-secondary mt-5">
                        <RefreshCw size={16} aria-hidden="true" /> Retry
                    </button>
                )}
            </div>
        );
    } else if (images.length === 0) {
        content = emptyState || (
            <div className="flex min-h-64 flex-col items-center justify-center text-muted-foreground">
                <p>No images found.</p>
            </div>
        );
    } else {
        const columns: ImageDto[][] = Array.from({ length: colCount }, () => []);
        images.forEach((img, i) => columns[i % colCount].push(img));
        content = (
            <div className="flex items-start gap-3 pb-6 sm:gap-4">
                {columns.map((column, c) => (
                    <div key={c} className="flex min-w-0 flex-1 flex-col gap-3 sm:gap-4">
                        {column.map(img => (
                            <ImageCard
                                key={img.id}
                                image={img}
                                onDelete={isAdmin ? onDelete : undefined}
                                onRemove={onRemove}
                                onEdit={isAdminOrModerator ? onEdit : undefined}
                                onSuggest={user && !isAdminOrModerator ? onSuggest : undefined}
                                forceOverlay={forceOverlay}
                                readOnly={readOnly}
                                showTags={showTags}
                            />
                        ))}
                    </div>
                ))}
            </div>
        );
    }

    return <div ref={gridRef}>{content}</div>;
};

export default ImageGrid;
