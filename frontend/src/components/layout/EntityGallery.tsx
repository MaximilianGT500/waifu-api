import { ReactNode, useEffect, useEffectEvent } from 'react';
import { useImages } from '../../hooks/useImages';
import { useGalleryParams } from '../../hooks/useGalleryParams';
import { useAlternativeSearch } from '../../hooks/useAlternativeSearch';
import { useNsfwConsent } from '../../hooks/useNsfwConsent';
import { GalleryLayout } from './GalleryLayout';
import EmptyState from '../EmptyState';
import NsfwWarning from '../NsfwWarning';
import { ImageOrderBy, NsfwMode } from '../../types';
import { isBot } from '../../utils/bot';

interface EntityGalleryProps {
    filterKey: 'includedArtists' | 'includedTags';
    filterValue: string;
    header: ReactNode;
}

const EntityGallery = ({ filterKey, filterValue, header }: EntityGalleryProps) => {
    const { filters, page, setPage, pageSize, setPageSize, searchParams, setSearchParams, isNsfw } = useGalleryParams(ImageOrderBy.UploadedAt);
    const { showWarning, setShowWarning, grantConsent, hasConsent } = useNsfwConsent(false);
    const scoped = { ...filters, [filterKey]: [filterValue, ...(filters[filterKey] ?? []).filter((value) => value !== filterValue)] };

    const { data, isLoading, error, refetch } = useImages(scoped);
    const { alternativeCount, isCheckingAlternatives } = useAlternativeSearch(isLoading, data?.items, isNsfw, scoped);

    const checkConsent = useEffectEvent((mode: NsfwMode) => {
        if (mode !== NsfwMode.False && !hasConsent() && !isBot()) setShowWarning(true);
    });

    useEffect(() => {
        checkConsent(isNsfw);
    }, [isNsfw]);

    const enableNsfw = () => setSearchParams((prev) => {
        prev.set('isNsfw', NsfwMode.All);
        return prev;
    });

    return (
        <>
            <GalleryLayout
                images={data?.items || []}
                isLoading={isLoading}
                error={error}
                refetch={refetch}
                page={page}
                totalPages={data?.totalPages || 1}
                setPage={setPage}
                pageSize={pageSize}
                setPageSize={setPageSize}
                searchParams={searchParams}
                setSearchParams={setSearchParams}
                headerContent={header}
                totalCount={data?.totalCount}
                apiRequest={{ path: '/images', defaults: { pageSize: String(pageSize) }, fixed: { [filterKey]: filterValue } }}
                emptyState={
                    <EmptyState
                        isCheckingAlternatives={isCheckingAlternatives}
                        alternativeCount={alternativeCount}
                        onEnableNsfw={enableNsfw}
                        onClearFilters={() => { setSearchParams(new URLSearchParams()); refetch(); }}
                    />
                }
            />
            {showWarning && (
                <NsfwWarning
                    variant="modal"
                    onConsent={grantConsent}
                    onDecline={() => {
                        setSearchParams((prev) => { prev.set('isNsfw', NsfwMode.False); return prev; });
                        setShowWarning(false);
                    }}
                />
            )}
        </>
    );
};

export default EntityGallery;
