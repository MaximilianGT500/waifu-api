import { useEffect, useEffectEvent, useState } from 'react';
import api, { httpStatus, showApiError } from '../services/api';
import { Link, useParams, Navigate } from 'react-router';
import { Plus, Edit2, Trash2, Heart, TriangleAlert } from 'lucide-react';
import Pagination from '../components/Pagination';
import { useNotification } from '../context/NotificationContext';
import { AlbumDto, PaginatedList, User } from '../types';
import AlbumModal, { AlbumFormData } from '../components/modals/AlbumModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import { useRequireAuth } from '../hooks/useRequireAuth';
import NotFound from './NotFound';
import { isId } from '../utils/ids';

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
                <p className="mb-6 flex items-start gap-2 rounded-2xl bg-warning/12 px-4 py-3 text-sm font-bold text-foreground">
                    <TriangleAlert size={16} className="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
                    Any changes here affect the albums of {ownerName}, not your own.
                </p>
            )}

            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
                {loading ? [...Array(4)].map((_, i) => <div key={i} className="aspect-[7/5] animate-pulse rounded-[1.25rem] bg-sleeve" />) : albums.map(album => (
                    <article
                        key={album.id}
                        className="group relative flex aspect-[7/5] overflow-hidden rounded-[1.25rem] text-white shadow-card transition-[box-shadow,translate] duration-200 hover:-translate-y-0.5 hover:shadow-lift has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-ring has-[a:focus-visible]:ring-offset-2 has-[a:focus-visible]:ring-offset-background motion-reduce:hover:translate-y-0"
                        style={{ backgroundColor: album.isDefault ? 'var(--binder-fav)' : `var(--binder-${album.id % 5})` }}
                    >
                        <div className="flex w-10 shrink-0 flex-col items-center justify-evenly bg-black/20" aria-hidden="true">
                            {[0, 1, 2].map((ring) => (
                                <span key={ring} className="size-3.5 rounded-full bg-background shadow-[inset_0_1px_3px_rgb(0_0_0/0.45)]" />
                            ))}
                        </div>
                        <div className="flex min-w-0 flex-1 flex-col p-4 sm:p-5">
                            <h2 className="min-w-0 rounded-xl bg-zinc-50 px-3.5 py-2 text-lg font-black leading-snug text-zinc-900 shadow-[0_1px_0_rgb(0_0_0/0.18)]">
                                <Link
                                    to={isViewingOther ? `/users/${userId}/albums/${album.id}` : `/albums/${album.id}`}
                                    className="block truncate outline-none after:absolute after:inset-0"
                                    title={album.name}
                                >
                                    {album.name}
                                </Link>
                            </h2>
                            <p className="mt-3 line-clamp-2 text-sm text-white wrap-break-word">{album.description || 'No description'}</p>
                            <p className="mt-auto flex items-center gap-2 pr-20 text-sm font-bold tabular">
                                {album.isDefault && <Heart size={15} className="fill-current" aria-hidden="true" />}
                                {album.isDefault && <span className="sr-only">Default album,</span>}
                                {album.imageCount} image{album.imageCount !== 1 ? 's' : ''}
                            </p>
                        </div>
                        <div className="absolute bottom-3 right-3 z-10 flex gap-1 transition-opacity xl:opacity-0 xl:group-hover:opacity-100 xl:group-focus-within:opacity-100">
                            <button
                                type="button"
                                onClick={() => { setSelectedAlbum(album); setIsEditOpen(true); }}
                                aria-label={`Edit album ${album.name}`}
                                title="Edit album"
                                className="grid size-8 place-items-center rounded-full bg-white/18 text-white transition-colors hover:bg-white/30"
                            >
                                <Edit2 size={15} aria-hidden="true" />
                            </button>
                            {!album.isDefault && (
                                <button
                                    type="button"
                                    onClick={() => { setSelectedAlbum(album); setIsDeleteOpen(true); }}
                                    aria-label={`Delete album ${album.name}`}
                                    title="Delete album"
                                    className="grid size-8 place-items-center rounded-full bg-white/18 text-white transition-colors hover:bg-white hover:text-destructive"
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