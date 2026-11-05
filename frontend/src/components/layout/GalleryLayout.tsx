import { useState, ReactNode } from 'react';
import { useNavigate } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ImageDto, Role, ImageFormData, ImageOrderBy, NsfwMode, PaginatedList, ReviewableContentType } from '../../types';
import { useAuth } from '../../context/AuthContext';
import { useNotification } from '../../context/NotificationContext';
import { useImageUpdate } from '../../hooks/useImageUpdate';
import api from '../../services/api';
import ImageGrid from '../ImageGrid';
import Pagination from '../Pagination';
import FilterSidebar from '../FilterSidebar';
import FilterToggleButton from '../FilterToggleButton';
import FloatingRefreshButton from '../FloatingRefreshButton';
import ConfirmModal from '../modals/ConfirmModal';
import ImageModal from '../modals/ImageModal';
import { Option } from '../SearchableSelect';
import { Tags, Shuffle, Loader2 } from 'lucide-react';
import EmptyState from '../EmptyState';
import { useTagView } from '../../hooks/useTagView';

interface GalleryLayoutProps {
    images: ImageDto[];
    isLoading: boolean;
    error?: unknown;
    refetch: () => void;
    page: number;
    totalPages: number;
    setPage: (page: number) => void;
    pageSize?: number;
    setPageSize?: (size: number) => void;
    searchParams: URLSearchParams;
    setSearchParams: (params: URLSearchParams | ((prev: URLSearchParams) => URLSearchParams)) => void;
    headerContent?: ReactNode;
    sortOptions?: Option[];
    emptyState?: ReactNode;
    onRemoveFromAlbum?: (id: number) => void;
    readOnly?: boolean;
}

export const GalleryLayout = ({
                                  images,
                                  isLoading,
                                  error,
                                  refetch,
                                  page,
                                  totalPages,
                                  setPage,
                                  pageSize,
                                  setPageSize,
                                  searchParams,
                                  setSearchParams,
                                  headerContent,
                                  sortOptions,
                                  emptyState,
                                  onRemoveFromAlbum,
                                  readOnly = false
                              }: GalleryLayoutProps) => {
    const { user } = useAuth();
    const { showNotification } = useNotification();
    const { updateImage } = useImageUpdate();
    const queryClient = useQueryClient();
    const navigate = useNavigate();

    const { showTags, toggle: toggleTags } = useTagView();

    const [showFilters, setShowFilters] = useState(false);
    const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
    const [imageToDelete, setImageToDelete] = useState<number | null>(null);
    const [editingImage, setEditingImage] = useState<ImageDto | null>(null);
    const [isEditModalOpen, setIsEditModalOpen] = useState(false);
    const [suggestingImage, setSuggestingImage] = useState<ImageDto | null>(null);
    const [isSuggestModalOpen, setIsSuggestModalOpen] = useState(false);
    const [isSurprising, setIsSurprising] = useState(false);

    const surpriseMe = async () => {
        if (isSurprising) return;
        setIsSurprising(true);
        try {
            const isNsfw = searchParams.get('isNsfw') || NsfwMode.False;
            const { data } = await api.get<PaginatedList<ImageDto>>('/images', {
                params: { isNsfw, orderBy: ImageOrderBy.Random, pageSize: 1 },
                skipGlobalErrorHandler: true,
            });
            if (data.items.length > 0) {
                navigate(`/images/${data.items[0].id}`);
            } else {
                showNotification('info', 'No images found for the current filters');
            }
        } catch {
            showNotification('error', 'Could not fetch a random image');
        } finally {
            setIsSurprising(false);
        }
    };

    const isAdmin = user?.role === Role.Admin;
    const isAdminOrModerator = user && (user.role === Role.Admin || user.role === Role.Moderator);

    const confirmDelete = (id: number) => {
        setImageToDelete(id);
        setIsDeleteModalOpen(true);
    };

    const handleRealDelete = async () => {
        if (!imageToDelete) return;
        try {
            await api.delete(`/images/${imageToDelete}`);
            refetch();
            setIsDeleteModalOpen(false);
            setImageToDelete(null);
            showNotification('success', 'Image deleted');
        } catch {
            return;
        }
    };

    const handleSaveEdit = async (data: ImageFormData) => {
        if (!editingImage) return;
        await updateImage(editingImage.id, data);
        showNotification('success', 'Image updated');
        refetch();
        setIsEditModalOpen(false);
        setEditingImage(null);
    };

    const defaultSortOptions = [
        { id: ImageOrderBy.Random, name: 'Random Shuffle' },
        { id: ImageOrderBy.UploadedAt, name: 'Newest First' },
        { id: ImageOrderBy.Favorites, name: 'Most Popular' }
    ];

    return (
        <div className="flex min-h-full items-start">
            <div className="min-w-0 flex-1">
                <FloatingRefreshButton onRefresh={refetch} />

                <div className="flex flex-col gap-5 px-4 pb-7 pt-7 sm:px-7 md:flex-row md:items-end md:justify-between lg:px-10 lg:pt-10">
                    <div className="min-w-0 flex-1">
                        {headerContent || (
                            <>
                                <h1 className="page-title">Gallery</h1>
                                <p className="page-lede">Every accepted image in the collection. Narrow it down with filters.</p>
                            </>
                        )}
                    </div>
                    <div className="flex shrink-0 items-center gap-2 self-end md:self-auto">
                        <button
                            type="button"
                            onClick={surpriseMe}
                            disabled={isSurprising}
                            title="Open a random image that matches the content rating"
                            className="btn btn-secondary"
                        >
                            {isSurprising ? <Loader2 size={16} className="animate-spin" aria-hidden="true" /> : <Shuffle size={16} aria-hidden="true" />}
                            <span className="sr-only lg:not-sr-only">Draw a card</span>
                        </button>
                        <button
                            type="button"
                            onClick={toggleTags}
                            title={showTags ? 'Hide tags' : 'Show tags'}
                            aria-label={showTags ? 'Hide tags' : 'Show tags'}
                            aria-pressed={showTags}
                            className={`btn ${showTags ? 'bg-brand-soft text-brand-ink ring-1 ring-brand/30 hover:bg-brand-soft' : 'btn-secondary'}`}
                        >
                            <Tags size={16} aria-hidden="true" />
                            <span className="hidden lg:inline">Tags</span>
                        </button>
                        <FilterToggleButton isOpen={showFilters} onToggle={() => setShowFilters(!showFilters)} />
                    </div>
                </div>

                <div className="px-4 sm:px-7 lg:px-10">
                    <ImageGrid
                        images={images}
                        isLoading={isLoading}
                        error={error}
                        onRetry={refetch}
                        onDelete={isAdmin ? confirmDelete : undefined}
                        onEdit={isAdminOrModerator ? (img) => { setEditingImage(img); setIsEditModalOpen(true); } : undefined}
                        onSuggest={user && !isAdminOrModerator ? (img) => { setSuggestingImage(img); setIsSuggestModalOpen(true); } : undefined}
                        onRemove={onRemoveFromAlbum}
                        readOnly={readOnly}
                        showTags={showTags}
                        emptyState={emptyState || (
                            <EmptyState
                                isCheckingAlternatives={false}
                                alternativeCount={null}
                                onEnableNsfw={() => {}}
                                onClearFilters={() => { setSearchParams(new URLSearchParams()); refetch(); }}
                            />
                        )}
                    />

                    <Pagination
                        currentPage={page}
                        totalPages={totalPages}
                        setPage={setPage}
                        pageSize={pageSize}
                        setPageSize={setPageSize}
                    />
                </div>
            </div>

            <FilterSidebar
                searchParams={searchParams}
                setSearchParams={setSearchParams}
                showFilters={showFilters}
                setShowFilters={setShowFilters}
                sortOptions={sortOptions || defaultSortOptions}
            />

            <ConfirmModal
                isOpen={isDeleteModalOpen}
                onClose={() => setIsDeleteModalOpen(false)}
                onConfirm={handleRealDelete}
                title="Delete Image"
                message="Permanently delete this image? This cannot be undone."
                confirmText="Delete"
                variant="destructive"
            />

            {editingImage && (
                <ImageModal
                    isOpen={isEditModalOpen}
                    onClose={() => { setIsEditModalOpen(false); setEditingImage(null); }}
                    initialData={editingImage}
                    onSubmit={handleSaveEdit}
                />
            )}

            {suggestingImage && (
                <ImageModal
                    isOpen={isSuggestModalOpen}
                    onClose={() => {
                        setIsSuggestModalOpen(false);
                        setSuggestingImage(null);
                        queryClient.invalidateQueries({ queryKey: ['my-pending-image-edits'] });
                    }}
                    initialData={suggestingImage}
                    onSubmit={() => {}}
                    suggestContext={{ targetType: ReviewableContentType.Image, targetId: suggestingImage.id, current: suggestingImage }}
                />
            )}
        </div>
    );
};