import { ReactNode } from 'react';
import { ImageDto, Role } from '../types';
import ImageCard from './ImageCard';
import { RefreshCw, CloudOff } from 'lucide-react';
import { useAuth } from '../context/AuthContext';

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

const POCKETS = 9;
const COLUMNS = 3;

const SPREAD = 'grid items-start gap-5 @min-[60rem]:grid-cols-2 @min-[60rem]:gap-6';
const SHEET = 'sleeve grid grid-cols-3 content-start gap-2 rounded-[1.25rem] p-2 sm:gap-3 sm:rounded-[1.5rem] sm:p-3';

const chunk = <T,>(items: T[], size: number): T[][] =>
    Array.from({ length: Math.ceil(items.length / size) }, (_, i) => items.slice(i * size, i * size + size));

const EmptyPocket = () => (
    <div className="rounded-[1.125rem] border-2 border-dashed border-input/20" aria-hidden="true">
        <div className="aspect-[5/7]" />
        <div className="h-6" />
    </div>
);

const ImageGrid = ({
    images, isLoading, error, onRetry,
    onEdit, onSuggest, onDelete, onRemove,
    emptyState, forceOverlay = false, readOnly = false,
    showTags = false,
}: ImageGridProps) => {
    const { user } = useAuth();
    const isAdminOrModerator = user && (user.role === Role.Admin || user.role === Role.Moderator);
    const isAdmin = user && user.role === Role.Admin;

    if (isLoading) {
        return (
            <div className="@container" aria-busy="true" aria-label="Loading images">
                <div className={SPREAD}>
                    {[0, 1].map((sheet) => (
                        <div key={sheet} className={`${SHEET} ${sheet === 1 ? 'hidden @min-[60rem]:grid' : ''}`}>
                            {Array.from({ length: POCKETS }, (_, i) => (
                                <div key={i} className="rounded-[1.125rem] bg-card p-1.5 shadow-card">
                                    <div className="aspect-[5/7] animate-pulse rounded-[0.8rem] bg-sleeve" />
                                    <div className="mx-1 mb-0.5 mt-2 h-2.5 w-1/2 animate-pulse rounded-full bg-sleeve" />
                                </div>
                            ))}
                        </div>
                    ))}
                </div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="sleeve flex min-h-64 flex-col items-center justify-center px-4 py-16 text-center">
                <span className="mb-4 grid size-12 place-items-center rounded-full bg-card text-muted-foreground shadow-card" aria-hidden="true">
                    <CloudOff size={22} />
                </span>
                <p className="font-bold">The images could not be loaded</p>
                <p className="mt-1 text-sm text-muted-foreground">The server did not respond. Try again in a moment.</p>
                {onRetry && (
                    <button type="button" onClick={onRetry} className="btn btn-secondary mt-5">
                        <RefreshCw size={16} aria-hidden="true" /> Retry
                    </button>
                )}
            </div>
        );
    }

    if (images.length === 0) {
        return emptyState || (
            <div className="sleeve flex min-h-64 flex-col items-center justify-center text-muted-foreground">
                <p>No images found.</p>
            </div>
        );
    }

    const sheets = chunk(images, POCKETS);

    return (
        <div className="@container pb-6">
            <div className={SPREAD}>
                {sheets.map((sheet, s) => (
                    <div key={sheet[0].id} className={SHEET}>
                        {sheet.map((img, i) => (
                            <ImageCard
                                key={img.id}
                                image={img}
                                eager={(s === 0 && i < 6) || (s === 1 && i < COLUMNS)}
                                priority={s === 0 && i < 2}
                                onDelete={isAdmin ? onDelete : undefined}
                                onRemove={onRemove}
                                onEdit={isAdminOrModerator ? onEdit : undefined}
                                onSuggest={user && !isAdminOrModerator ? onSuggest : undefined}
                                forceOverlay={forceOverlay}
                                readOnly={readOnly}
                                showTags={showTags}
                            />
                        ))}
                        {Array.from({ length: (COLUMNS - (sheet.length % COLUMNS)) % COLUMNS }, (_, k) => <EmptyPocket key={`empty-${k}`} />)}
                    </div>
                ))}
            </div>
        </div>
    );
};

export default ImageGrid;
