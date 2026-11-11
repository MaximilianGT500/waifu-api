import { useState, ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router';
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
import { Tags, Shuffle, Loader2, Braces, Play, Bookmark, Settings2 } from 'lucide-react';
import EmptyState from '../EmptyState';
import { useTagView } from '../../hooks/useTagView';
import { useHotkeys } from '../../hooks/useHotkeys';
import ActiveFilters from '../ActiveFilters';
import ApiRequestDialog from '../modals/ApiRequestDialog';
import { imageListParams } from '../../utils/apiSnippets';
import { BrowseSource, saveBrowseContext } from '../../utils/browseContext';
import { Dropdown, DropdownTrigger, DropdownItem, DropdownSeparator } from '../Dropdown';
import { SaveSearchDialog, ManageSavedSearchesDialog } from '../modals/SavedSearchesDialog';
import { useSavedSearches } from '../../hooks/useSavedSearches';
import { addSavedSearch, savedSearchHref, searchQueryOf } from '../../utils/savedSearches';

export interface GalleryApiRequest {
    path: string;
    defaults?: Record<string, string>;
    fixed?: Record<string, string>;
    needsAuth?: boolean;
}

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
    totalCount?: number;
    apiRequest?: GalleryApiRequest;
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
                                  readOnly = false,
                                  totalCount,
                                  apiRequest,
                              }: GalleryLayoutProps) => {
    const { user } = useAuth();
    const { showNotification } = useNotification();
    const { updateImage } = useImageUpdate();
    const queryClient = useQueryClient();
    const navigate = useNavigate();
    const location = useLocation();

    const { showTags, toggle: toggleTags } = useTagView();

    const [showFilters, setShowFilters] = useState(false);
    const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
    const [imageToDelete, setImageToDelete] = useState<number | null>(null);
    const [editingImage, setEditingImage] = useState<ImageDto | null>(null);
    const [isEditModalOpen, setIsEditModalOpen] = useState(false);
    const [suggestingImage, setSuggestingImage] = useState<ImageDto | null>(null);
    const [isSuggestModalOpen, setIsSuggestModalOpen] = useState(false);
    const [isSurprising, setIsSurprising] = useState(false);
    const [isApiOpen, setIsApiOpen] = useState(false);
    const [saveName, setSaveName] = useState<string | null>(null);
    const [isManageOpen, setIsManageOpen] = useState(false);
    const savedSearches = useSavedSearches();
    const canSaveSearches = !readOnly;

    const saveSearch = (name: string) => {
        addSavedSearch(name, location.pathname, searchQueryOf(searchParams));
        setSaveName(null);
        showNotification('success', 'Search saved');
    };

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

    const listParams = apiRequest ? imageListParams(searchParams, apiRequest.defaults, apiRequest.fixed) : null;
    const browseSource: BrowseSource | undefined = apiRequest && listParams ? {
        path: apiRequest.path,
        params: listParams.toString(),
        firstPage: page,
        lastPage: page,
        totalPages: Math.max(totalPages, 1),
        pageSize: Number(listParams.get('pageSize')) || Math.max(images.length, 1),
        totalCount,
    } : undefined;

    const startSlideshow = () => {
        if (images.length === 0) return false;
        saveBrowseContext({ ids: images.map((img) => img.id), from: `${location.pathname}${location.search}`, source: browseSource });
        navigate(`/images/${images[0].id}`, { state: { slideshow: true } });
    };

    const goToPage = (next: number) => {
        if (next < 1 || next > totalPages || next === page) return false;
        setPage(next);
        window.scrollTo({ top: 0, behavior: 'instant' as ScrollBehavior });
    };

    useHotkeys({
        ArrowLeft: () => goToPage(page - 1),
        ArrowRight: () => goToPage(page + 1),
        f: () => setShowFilters((open) => !open),
        t: () => toggleTags(),
        r: () => refetch(),
        d: () => { void surpriseMe(); },
        s: () => startSlideshow(),
    });

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
                        {canSaveSearches && savedSearches.length > 0 && (
                            <Dropdown
                                width="w-72"
                                label="Saved searches"
                                trigger={
                                    <DropdownTrigger className="btn btn-secondary">
                                        <Bookmark size={16} aria-hidden="true" />
                                        <span className="hidden xl:inline">Saved</span>
                                    </DropdownTrigger>
                                }
                            >
                                {savedSearches.slice(0, 12).map((search) => (
                                    <DropdownItem key={search.id} to={savedSearchHref(search)} icon={<Bookmark size={15} />}>
                                        <span className="truncate">{search.name}</span>
                                    </DropdownItem>
                                ))}
                                <DropdownSeparator />
                                <DropdownItem onClick={() => setIsManageOpen(true)} icon={<Settings2 size={15} />}>Manage saved searches…</DropdownItem>
                            </Dropdown>
                        )}
                        {apiRequest && (
                            <button
                                type="button"
                                onClick={() => setIsApiOpen(true)}
                                title="Get this request for the API"
                                aria-label="Use this in the API"
                                className="btn btn-secondary"
                            >
                                <Braces size={16} aria-hidden="true" />
                                <span className="hidden lg:inline">API</span>
                            </button>
                        )}
                        <button
                            type="button"
                            onClick={startSlideshow}
                            disabled={isLoading || images.length === 0}
                            title="Play these results as a slideshow (S)"
                            aria-label="Play a slideshow"
                            className="btn btn-secondary"
                        >
                            <Play size={16} aria-hidden="true" />
                            <span className="hidden xl:inline">Slideshow</span>
                        </button>
                        <button
                            type="button"
                            onClick={surpriseMe}
                            disabled={isSurprising}
                            title="Open a random image that matches the content rating (D)"
                            className="btn btn-secondary"
                        >
                            {isSurprising ? <Loader2 size={16} className="animate-spin" aria-hidden="true" /> : <Shuffle size={16} aria-hidden="true" />}
                            <span className="sr-only lg:not-sr-only">Draw a card</span>
                        </button>
                        <button
                            type="button"
                            onClick={toggleTags}
                            title={showTags ? 'Hide tags (T)' : 'Show tags (T)'}
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
                    <ActiveFilters
                        searchParams={searchParams}
                        setSearchParams={setSearchParams}
                        totalCount={totalCount}
                        isLoading={isLoading}
                        onSave={canSaveSearches ? setSaveName : undefined}
                    />
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
                        browseSource={browseSource}
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

            <SaveSearchDialog
                isOpen={saveName !== null}
                onClose={() => setSaveName(null)}
                defaultName={saveName ?? ''}
                onSave={saveSearch}
            />
            <ManageSavedSearchesDialog isOpen={isManageOpen} onClose={() => setIsManageOpen(false)} />

            {apiRequest && (
                <ApiRequestDialog
                    isOpen={isApiOpen}
                    onClose={() => setIsApiOpen(false)}
                    path={apiRequest.path}
                    params={listParams ?? undefined}
                    needsAuth={apiRequest.needsAuth}
                    description="The request behind this page of results, with the current filters. Change the page or the filters and it follows along."
                />
            )}

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