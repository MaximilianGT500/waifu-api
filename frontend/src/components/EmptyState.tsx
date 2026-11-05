import { SearchX, EyeOff } from 'lucide-react';
import { ReactNode } from 'react';

interface EmptyStateProps {
    isCheckingAlternatives: boolean;
    alternativeCount: number | null;
    onEnableNsfw: () => void;
    onClearFilters: () => void;
    customEmptyMessage?: ReactNode;
}

const EmptyState = ({
    isCheckingAlternatives,
    alternativeCount,
    onEnableNsfw,
    onClearFilters,
    customEmptyMessage,
}: EmptyStateProps) => {
    const hiddenByRating = !isCheckingAlternatives && alternativeCount !== null && alternativeCount > 0;
    const Icon = hiddenByRating ? EyeOff : SearchX;

    return (
        <div className="sleeve flex min-h-[50vh] flex-col items-center justify-center px-4 py-16 text-center">
            <span className="mb-6 grid aspect-[5/7] w-16 place-items-center rounded-xl border-2 border-dashed border-input/50 text-muted-foreground" aria-hidden="true">
                <Icon size={24} strokeWidth={2} />
            </span>
            <p className="text-lg font-black text-foreground">No images found</p>

            {isCheckingAlternatives ? (
                <p className="mt-2 text-sm text-muted-foreground animate-pulse">Checking other content ratings…</p>
            ) : hiddenByRating ? (
                <div className="mt-2 flex flex-col items-center animate-in fade-in slide-in-from-bottom-4 duration-500">
                    <p className="max-w-sm text-sm text-muted-foreground">
                        <span className="font-bold text-foreground tabular">{alternativeCount}</span> images match your search but are hidden by the NSFW filter.
                    </p>
                    <button type="button" onClick={onEnableNsfw} className="btn btn-primary mt-5">
                        Include NSFW Content
                    </button>
                </div>
            ) : customEmptyMessage ? (
                <div className="mt-3 text-sm text-muted-foreground">{customEmptyMessage}</div>
            ) : (
                <>
                    <p className="mt-2 max-w-sm text-sm text-muted-foreground">Nothing matches the current filters. Remove a few or start over.</p>
                    <button type="button" onClick={onClearFilters} className="btn btn-secondary mt-5">
                        Clear All Filters
                    </button>
                </>
            )}
        </div>
    );
};

export default EmptyState;
