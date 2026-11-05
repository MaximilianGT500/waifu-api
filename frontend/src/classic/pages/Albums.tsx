import { useEffect, useEffectEvent, useState } from 'react';
import api, { httpStatus, showApiError } from '../../services/api';
import { Link, useParams, Navigate } from 'react-router';
import { Folder, Plus, Edit2, Trash2, Heart, TriangleAlert } from 'lucide-react';
import Pagination from '../components/Pagination';
import { useNotification } from '../../context/NotificationContext';
import { AlbumDto, PaginatedList, User } from '../../types';
import AlbumModal, { AlbumFormData } from '../components/modals/AlbumModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import { useRequireAuth } from '../../hooks/useRequireAuth';
import NotFound from './NotFound';
import { isId } from '../../utils/ids';

const Albums = () => {
    const { userId } = useParams<{ userId?: string }>();
    if (userId !== undefined && !isId(userId)) return <NotFound />;
    return <AlbumList userId={userId} />;
};

const AlbumList = ({ userId }: { userId?: string }) => {
    const user = useRequireAuth();
    const { showNotification } = useNotification();
    const isViewingOther = !!userId;
    const [targetUserName, setTargetUserName] = useState<string | null>(null);
    const [notFound, setNotFound] = useState(false);
    const [albums, setAlbums] = useState<AlbumDto[]>([]);
    const [loading, setLoading] = useState(true);

    const [page, setPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);
    const pageSize = 30;

    const [isCreateOpen, setIsCreateOpen] = useState(false);
    const [isEditOpen, setIsEditOpen] = useState(false);
    const [isDeleteOpen, setIsDeleteOpen] = useState(false);
    const [selectedAlbum, setSelectedAlbum] = useState<AlbumDto | null>(null);

    const apiUserPath = isViewingOther ? `/users/${userId}` : '/users/me';

    const fetchAlbums = async () => {
        if(!user) return;
        setLoading(true);
        try {
            const { data } = await api.get<PaginatedList<AlbumDto>>(`${apiUserPath}/albums`, {
                params: { page, pageSize },
                skipGlobalErrorHandler: true
            });
            setAlbums(data.items);
            setTotalPages(data.totalPages);
        } catch (e) {
            if (httpStatus(e) === 404) setNotFound(true);
            else showApiError(e);
        } finally { setLoading(false); }
    };

    const loadPage = useEffectEvent(() => {
        fetchAlbums();
        if (isViewingOther) {
            api.get<User>(`/users/${userId}`, { skipGlobalErrorHandler: true })
                .then(res => setTargetUserName(res.data.name))
                .catch((e: unknown) => { if (httpStatus(e) !== 403) showApiError(e); });
        }
    });

    useEffect(() => {
        if (user) loadPage();
    }, [user, page]);

    const handleCreate = async (data: AlbumFormData) => {
        try {
            await api.post(`${apiUserPath}/albums`, data);
            showNotification('success', 'Album created');
            setIsCreateOpen(false);
            fetchAlbums();
        } catch {
            return;
        }
    };

    const handleEdit = async (data: AlbumFormData) => {
        if (!selectedAlbum) return;
        try {
            await api.patch(`${apiUserPath}/albums/${selectedAlbum.id}`, data);
            showNotification('success', 'Album updated');
            setIsEditOpen(false);
            fetchAlbums();
        } catch {
            return;
        }
    };

    const handleDelete = async () => {
        if (!selectedAlbum) return;
        try {
            await api.delete(`${apiUserPath}/albums/${selectedAlbum.id}`);
            showNotification('success', 'Album deleted');
            setIsDeleteOpen(false);
            fetchAlbums();
        } catch {
            return;
        }
    };

    if (!user) return null;
    if (userId && user.id.toString() === userId) return <Navigate to="/albums" replace />;
    if (notFound) return <NotFound />;

    const totalImages = albums.reduce((sum, album) => sum + (album.imageCount || 0), 0);
    const ownerName = targetUserName || `user ${userId}`;

    return (
        <div className="page">
            <div className="page-header">
                <div className="min-w-0">
                    <h1 className="page-title">{isViewingOther ? `${targetUserName || `User ${userId}`}'s Albums` : 'My Albums'}</h1>
                    <p className="page-lede">
                        {isViewingOther ? `Viewing the albums of ${ownerName}.` : 'Collections of images you saved.'}{' '}
                        <span className="tabular">{totalImages} image{totalImages !== 1 ? 's' : ''} in total.</span>
                    </p>
                </div>
                <button type="button" aria-label="Create album" onClick={() => setIsCreateOpen(true)} className="btn btn-primary shrink-0 self-start md:self-auto">
                    <Plus size={18} aria-hidden="true" /> <span className="hidden sm:inline">Create Album</span>
                </button>
            </div>
            {isViewingOther && (
                <p className="mb-6 flex items-start gap-2 rounded-xl bg-warning/12 px-4 py-3 text-sm font-medium text-foreground">
                    <TriangleAlert size={16} className="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
                    Any changes here affect the albums of {ownerName}, not your own.
                </p>
            )}

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
                {loading ? [...Array(4)].map((_, i) => <div key={i} className="h-44 animate-pulse rounded-2xl bg-secondary" />) : albums.map(album => (
                    <article key={album.id} className="panel group relative flex flex-col p-5 transition-[border-color,box-shadow] hover:border-brand/40 hover:shadow-float">
                        <span className={`mb-4 grid size-12 place-items-center rounded-2xl ${album.isDefault ? 'bg-rose-500/12 text-rose-600 dark:text-rose-400' : 'bg-brand-soft text-brand-ink'}`} aria-hidden="true">
                            {album.isDefault ? <Heart size={22} className="fill-current" /> : <Folder size={22} />}
                        </span>
                        <h2 className="min-w-0 pr-16 text-lg font-bold leading-snug">
                            <Link
                                to={isViewingOther ? `/users/${userId}/albums/${album.id}` : `/albums/${album.id}`}
                                className="block truncate outline-none after:absolute after:inset-0 after:rounded-2xl focus-visible:after:ring-2 focus-visible:after:ring-ring"
                                title={album.name}
                            >
                                {album.name}
                            </Link>
                        </h2>
                        <p className="mt-1 line-clamp-2 min-h-[2.75em] text-sm text-muted-foreground wrap-break-word">{album.description || 'No description'}</p>
                        <div className="mt-4 flex items-center gap-2">
                            {album.isDefault && <span className="chip bg-rose-500/12 text-rose-600 dark:text-rose-400">Default</span>}
                            <span className="chip tabular">{album.imageCount} image{album.imageCount !== 1 ? 's' : ''}</span>
                        </div>
                        <div className="absolute right-3 top-3 z-10 flex gap-0.5 transition-opacity xl:opacity-0 xl:group-hover:opacity-100 xl:group-focus-within:opacity-100">
                            <button
                                type="button"
                                onClick={() => { setSelectedAlbum(album); setIsEditOpen(true); }}
                                aria-label={`Edit album ${album.name}`}
                                title="Edit album"
                                className="grid size-8 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                            >
                                <Edit2 size={15} aria-hidden="true" />
                            </button>
                            {!album.isDefault && (
                                <button
                                    type="button"
                                    onClick={() => { setSelectedAlbum(album); setIsDeleteOpen(true); }}
                                    aria-label={`Delete album ${album.name}`}
                                    title="Delete album"
                                    className="grid size-8 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                                >
                                    <Trash2 size={15} aria-hidden="true" />
                                </button>
                            )}
                        </div>
                    </article>
                ))}
            </div>

            {!loading && <Pagination currentPage={page} totalPages={totalPages} setPage={setPage} />}

            <AlbumModal isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} onSubmit={handleCreate} title="Create New Album" submitLabel="Create" />
            <AlbumModal isOpen={isEditOpen} onClose={() => setIsEditOpen(false)} onSubmit={handleEdit} initialData={selectedAlbum ? { name: selectedAlbum.name, description: selectedAlbum.description } : undefined} title="Edit Album" submitLabel="Save" />
            <ConfirmModal isOpen={isDeleteOpen} onClose={() => setIsDeleteOpen(false)} onConfirm={handleDelete} title="Delete Album" message={<p>Delete the album <strong className="text-foreground">{selectedAlbum?.name}</strong>? The images inside stay in the gallery.</p>} />
        </div>
    );
};

export default Albums;