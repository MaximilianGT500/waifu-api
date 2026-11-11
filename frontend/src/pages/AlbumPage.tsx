import { useEffect, useState, useEffectEvent } from 'react';
import { useParams, Link, useNavigate, Navigate } from 'react-router';
import api, { httpStatus, showApiError } from '../services/api';
import { AlbumDto, ImageOrderBy, NsfwMode, User as UserType } from '../types';
import { ChevronLeft, Edit2, Trash2, FolderMinus, TriangleAlert, Heart } from 'lucide-react';
import { useNotification } from '../context/NotificationContext';
import Modal from '../components/Modal';
import ConfirmModal from '../components/modals/ConfirmModal';
import { useRequireAuth } from '../hooks/useRequireAuth';
import { useImages } from '../hooks/useImages';
import { GalleryLayout } from '../components/layout/GalleryLayout';
import { useGalleryParams } from '../hooks/useGalleryParams';
import { useAlternativeSearch } from '../hooks/useAlternativeSearch';
import EmptyState from '../components/EmptyState';
import NotFound from './NotFound';
import { MetaTags } from '../hooks/useMetaTags';
import { isId } from '../utils/ids';

const AlbumPage = () => {
    const { id, userId } = useParams<{ id: string; userId?: string }>();
    if (!isId(id) || (userId !== undefined && !isId(userId))) return <NotFound />;
    return <AlbumView id={id} userId={userId} />;
};

const AlbumView = ({ id, userId }: { id: string; userId?: string }) => {
    const user = useRequireAuth();
    const navigate = useNavigate();
    const isViewingOther = !!userId;
    const apiUserPath = isViewingOther ? `/users/${userId}` : '/users/me';
    const albumsBasePath = isViewingOther ? `/users/${userId}/albums` : '/albums';
    const { showNotification } = useNotification();

    const { filters, page, setPage, searchParams, setSearchParams, isNsfw } = useGalleryParams(ImageOrderBy.AddedToAlbum);

    const [album, setAlbum] = useState<AlbumDto | null>(null);
    const [targetUserName, setTargetUserName] = useState<string | null>(null);
    const [loadingAlbum, setLoadingAlbum] = useState(true);
    const [notFound, setNotFound] = useState(false);
    const [isEditAlbumOpen, setIsEditAlbumOpen] = useState(false);
    const [editAlbumFormData, setEditAlbumFormData] = useState({ name: '', description: '' });
    const [isDeleteAlbumOpen, setIsDeleteAlbumOpen] = useState(false);
    const [imageToRemove, setImageToRemove] = useState<number | null>(null);

    const loadAlbum = useEffectEvent(() => {
        setLoadingAlbum(true);
        api.get<AlbumDto>(`${apiUserPath}/albums/${id}`, { skipGlobalErrorHandler: true })
            .then(res => setAlbum(res.data))
            .catch((e: unknown) => { if (httpStatus(e) === 404) setNotFound(true); else showApiError(e); })
            .finally(() => setLoadingAlbum(false));
        if (isViewingOther) {
            api.get<UserType>(`/users/${userId}`, { skipGlobalErrorHandler: true })
                .then(res => setTargetUserName(res.data.name))
                .catch((e: unknown) => { if (httpStatus(e) !== 403) showApiError(e); });
        }
    });

    useEffect(() => {
        if (user && id) loadAlbum();
    }, [id, user]);

    const { data: paginatedImages, isLoading: loadingImages, refetch } = useImages({ ...filters, albumId: id, albumUserId: userId });

    const { alternativeCount, isCheckingAlternatives } = useAlternativeSearch(
        loadingImages,
        paginatedImages?.items,
        isNsfw,
        { ...filters, albumId: id, albumUserId: userId }
    );

    const handleRemoveImage = async () => {
        if (!id || !imageToRemove) return;
        try {
            await api.delete(`${apiUserPath}/albums/${id}/images/${imageToRemove}`);
            refetch();
            if (album) setAlbum({ ...album, imageCount: Math.max(0, (album.imageCount || 0) - 1) });
            showNotification('success', 'Image removed from album');
            setImageToRemove(null);
        } catch {
            return;
        }
    };

    const handleDeleteAlbum = async () => {
        if (!album) return;
        try {
            await api.delete(`${apiUserPath}/albums/${album.id}`);
            showNotification('success', 'Album deleted');
            setIsDeleteAlbumOpen(false);
            navigate(albumsBasePath);
        } catch {
            return;
        }
    };

    const handleEditAlbum = async () => {
        if (!album) return;
        try {
            const { data } = await api.patch<AlbumDto>(`${apiUserPath}/albums/${album.id}`, editAlbumFormData);
            setAlbum(data);
            setIsEditAlbumOpen(false);
            showNotification('success', 'Album updated');
        } catch {
            return;
        }
    };

    const handleEnableNsfw = () => {
        setSearchParams(prev => {
            prev.set('isNsfw', NsfwMode.All);
            return prev;
        });
    };

    const sortOptions = [
        { id: ImageOrderBy.AddedToAlbum, name: 'Date Added' },
        { id: ImageOrderBy.UploadedAt, name: 'Newest Upload' },
        { id: ImageOrderBy.Favorites, name: 'Most Popular' },
        { id: ImageOrderBy.Random, name: 'Random Shuffle' },
    ];

    if (!user) return null;
    if (userId && user.id.toString() === userId) return <Navigate to={`/albums/${id}`} replace />;
    if (notFound) return <NotFound />;
    if (loadingAlbum && !album) return <div className="page text-center text-muted-foreground" role="status">Loading album…</div>;

    const AlbumHeader = (
        <div className="flex min-w-0 max-w-full items-start gap-3">
            <Link to={albumsBasePath} aria-label="Back to albums" title="Back to albums" className="mt-0.5 grid size-10 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground">
                <ChevronLeft size={22} aria-hidden="true" />
            </Link>
            <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <h1 className="page-title truncate">{album?.name || 'Album'}</h1>
                    <div className="flex shrink-0 items-center gap-0.5">
                        <button
                            type="button"
                            onClick={() => { setEditAlbumFormData({ name: album?.name || '', description: album?.description || '' }); setIsEditAlbumOpen(true); }}
                            className="grid size-8 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                            title="Edit album"
                            aria-label="Edit album"
                        >
                            <Edit2 size={15} aria-hidden="true" />
                        </button>
                        {!album?.isDefault && (
                            <button
                                type="button"
                                onClick={() => setIsDeleteAlbumOpen(true)}
                                className="grid size-8 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                                title="Delete album"
                                aria-label="Delete album"
                            >
                                <Trash2 size={15} aria-hidden="true" />
                            </button>
                        )}
                    </div>
                </div>
                <div className="mt-2 flex items-center gap-2">
                    {album?.isDefault && <span className="chip bg-brand-soft text-brand-ink"><Heart size={11} className="fill-current" aria-hidden="true" />Default</span>}
                    <span className="chip tabular">{album?.imageCount || 0} image{album?.imageCount !== 1 ? 's' : ''}</span>
                </div>
                {album?.description && <p className="mt-3 max-w-3xl whitespace-pre-wrap text-sm text-muted-foreground wrap-break-word">{album.description}</p>}
                {isViewingOther && (
                    <p className="mt-3 inline-flex items-start gap-2 rounded-2xl bg-warning/12 px-3 py-2 text-sm font-bold text-foreground">
                        <TriangleAlert size={15} className="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
                        Any changes here affect the album of {targetUserName || `user ${userId}`}, not your own.
                    </p>
                )}
            </div>
        </div>
    );

    return (
        <>
            <MetaTags
              title={album?.name || 'Album'}
              description={album?.description || `Album with ${album?.imageCount || 0} images.`}
            />
            <GalleryLayout
                images={paginatedImages?.items || []}
                isLoading={loadingImages}
                refetch={refetch}
                page={page}
                totalPages={paginatedImages?.totalPages || 1}
                setPage={setPage}
                searchParams={searchParams}
                setSearchParams={setSearchParams}
                headerContent={AlbumHeader}
                sortOptions={sortOptions}
                totalCount={paginatedImages?.totalCount}
                apiRequest={{ path: `${apiUserPath}/albums/${id}/images`, defaults: { pageSize: String(filters.pageSize ?? 36) }, needsAuth: true }}
                onRemoveFromAlbum={setImageToRemove}
                album={!isViewingOther && album ? { id: album.id, name: album.name } : undefined}
                readOnly={isViewingOther}
                emptyState={
                    <EmptyState
                        isCheckingAlternatives={isCheckingAlternatives}
                        alternativeCount={alternativeCount}
                        onEnableNsfw={handleEnableNsfw}
                        onClearFilters={() => { setSearchParams(new URLSearchParams()); refetch(); }}
                        customEmptyMessage={
                            !isCheckingAlternatives && (!alternativeCount || alternativeCount === 0) ? (
                                <div className="flex flex-col items-center">
                                    <p>This album is empty.</p>
                                    <Link to="/gallery" className="btn btn-secondary mt-4">Browse the gallery</Link>
                                </div>
                            ) : undefined
                        }
                    />
                }
            />

            <Modal
                isOpen={isEditAlbumOpen}
                onClose={() => setIsEditAlbumOpen(false)}
                title="Edit Album Details"
                footer={
                    <div className="flex justify-end gap-2">
                        <button type="button" onClick={() => setIsEditAlbumOpen(false)} className="btn btn-secondary">Cancel</button>
                        <button type="button" onClick={handleEditAlbum} className="btn btn-primary">Save</button>
                    </div>
                }
            >
                <div className="space-y-4">
                    <div>
                        <label htmlFor="album-page-name" className="field-label">Name</label>
                        <input id="album-page-name" value={editAlbumFormData.name} onChange={e => setEditAlbumFormData({ ...editAlbumFormData, name: e.target.value })} className="input" />
                    </div>
                    <div>
                        <label htmlFor="album-page-description" className="field-label">Description</label>
                        <textarea id="album-page-description" value={editAlbumFormData.description} onChange={e => setEditAlbumFormData({ ...editAlbumFormData, description: e.target.value })} className="input h-24 resize-none" />
                    </div>
                </div>
            </Modal>

            <ConfirmModal isOpen={!!imageToRemove} onClose={() => setImageToRemove(null)} onConfirm={handleRemoveImage} title="Remove Image" message="Remove this image from the album? It will remain in the gallery." confirmText="Remove" variant="warning" icon={FolderMinus} />

            <ConfirmModal isOpen={isDeleteAlbumOpen} onClose={() => setIsDeleteAlbumOpen(false)} onConfirm={handleDeleteAlbum} title="Delete Album" message={<p>Delete the album <strong className="text-foreground">{album?.name}</strong>? The images inside stay in the gallery.</p>} confirmText="Delete Album" variant="destructive" />
        </>
    );
};

export default AlbumPage;