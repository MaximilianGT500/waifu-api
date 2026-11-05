import { useState, useEffect, useEffectEvent } from 'react';
import { Check, Plus, Heart, Folder } from 'lucide-react';
import api from '../../services/api';
import { AlbumDto, ImageDto, PaginatedList } from '../../types';
import { useNotification } from '../../context/NotificationContext';
import { useAuth } from '../../context/AuthContext';
import Modal from '../Modal';
import AlbumModal, { AlbumFormData } from './AlbumModal';
import { nowIso } from '../../utils/time';

interface AlbumSelectionModalProps {
    isOpen: boolean;
    onClose: () => void;
    image: ImageDto;
    onUpdate?: (updatedImage: ImageDto) => void;
}

const AlbumSelectionModal = ({ isOpen, onClose, image, onUpdate }: AlbumSelectionModalProps) => {
    const albumPageSize = 100;
    const { user } = useAuth();
    const { showNotification } = useNotification();
    const [userAlbums, setUserAlbums] = useState<AlbumDto[]>([]);
    const [isAlbumsLoaded, setIsAlbumsLoaded] = useState(false);
    const [isCreateAlbumOpen, setIsCreateAlbumOpen] = useState(false);

    const loadAlbums = async () => {
        if (isAlbumsLoaded || !user) return;
        try {
            const { data } = await api.get<PaginatedList<AlbumDto>>('/users/me/albums', { params: { pageSize: albumPageSize }, skipGlobalErrorHandler: true });
            setUserAlbums(data.items);
            setIsAlbumsLoaded(true);
        } catch {
            setIsAlbumsLoaded(false);
        }
    };

    const loadOnOpen = useEffectEvent(() => { loadAlbums(); });

    useEffect(() => {
        if (isOpen) loadOnOpen();
    }, [isOpen]);

    const handleToggleAlbum = async (e: React.MouseEvent, targetAlbum: AlbumDto) => {
        e.preventDefault();
        e.stopPropagation();

        if (!user) return;
        const isInAlbum = image.albums?.some(a => a.id === targetAlbum.id);
        const isFavorites = targetAlbum.isDefault;

        try {
            let updatedImage = { ...image };

            if (isInAlbum) {
                await api.delete(`/users/${user.id}/albums/${targetAlbum.id}/images/${image.id}`);
                showNotification('success', `Removed from ${targetAlbum.name}`);

                updatedImage.albums = image.albums?.filter(a => a.id !== targetAlbum.id) || [];

                if (isFavorites) {
                    updatedImage.likedAt = undefined;
                    updatedImage.favorites = Math.max(0, (updatedImage.favorites || 0) - 1);
                }
            } else {
                await api.post(`/users/${user.id}/albums/${targetAlbum.id}/images/${image.id}`);
                showNotification('success', `Added to ${targetAlbum.name}`);

                updatedImage.albums = [...(image.albums || []), targetAlbum];

                if (isFavorites) {
                    updatedImage.likedAt = nowIso();
                    updatedImage.favorites = (updatedImage.favorites || 0) + 1;
                }
            }

            if (onUpdate) {
                onUpdate(updatedImage);
            }

            setUserAlbums([...userAlbums]);
        } catch {
            showNotification('error', 'Could not update the album. Please try again.');
        }
    };

    const handleCreateAlbum = async (data: AlbumFormData) => {
        try {
            const { data: newAlbum } = await api.post<AlbumDto>(`/users/me/albums`, data);
            showNotification('success', 'Album created');
            setIsCreateAlbumOpen(false);
            setUserAlbums(prev => [...prev, newAlbum]);
        } catch {
            return;
        }
    };

    if (!user) return null;

    return (
        <>
            <Modal
                isOpen={isOpen}
                onClose={onClose}
                title="Add to Album"
                maxWidth="max-w-sm"
            >
                <div className="space-y-3">
                    {isAlbumsLoaded ? (
                        userAlbums.length === 0 ? (
                            <div className="py-6 text-center text-muted-foreground">
                                <p>You have no albums yet.</p>
                                <button type="button" onClick={() => setIsCreateAlbumOpen(true)} className="btn btn-primary mt-4">
                                    <Plus size={16} aria-hidden="true" /> Create your first album
                                </button>
                            </div>
                        ) : (
                            <ul className="-mx-1 max-h-[60vh] space-y-1 overflow-y-auto px-1">
                                {userAlbums.map(album => {
                                    const isInAlbum = !!image.albums?.some(a => a.id === album.id);
                                    return (
                                        <li key={album.id}>
                                            <button
                                                type="button"
                                                aria-pressed={isInAlbum}
                                                onClick={(e) => handleToggleAlbum(e, album)}
                                                className={`flex w-full items-center justify-between gap-3 rounded-full px-4 py-2.5 text-left transition-colors ${
                                                    isInAlbum ? 'bg-brand-soft text-brand-ink' : 'hover:bg-secondary'
                                                }`}
                                            >
                                                <span className="flex min-w-0 items-center gap-2.5">
                                                    {album.isDefault
                                                        ? <Heart size={16} aria-hidden="true" className={`shrink-0 ${isInAlbum ? 'fill-current' : 'text-muted-foreground'}`} />
                                                        : <Folder size={16} aria-hidden="true" className={`shrink-0 ${isInAlbum ? '' : 'text-muted-foreground'}`} />}
                                                    <span className="truncate font-medium">{album.name}</span>
                                                </span>
                                                <span className={`grid size-5 shrink-0 place-items-center rounded-full border-[1.5px] ${isInAlbum ? 'border-transparent bg-brand text-white dark:text-zinc-950' : 'border-input'}`} aria-hidden="true">
                                                    {isInAlbum && <Check size={13} strokeWidth={3} />}
                                                </span>
                                            </button>
                                        </li>
                                    );
                                })}
                            </ul>
                        )
                    ) : (
                        <div className="py-8 text-center text-muted-foreground" role="status">Loading albums…</div>
                    )}

                    {userAlbums.length > 0 && (
                        <button type="button" onClick={() => setIsCreateAlbumOpen(true)} className="btn btn-ghost w-full border border-dashed border-border">
                            <Plus size={16} aria-hidden="true" /> New album
                        </button>
                    )}
                </div>
            </Modal>

            <AlbumModal
                isOpen={isCreateAlbumOpen}
                onClose={() => setIsCreateAlbumOpen(false)}
                onSubmit={handleCreateAlbum}
                title="Create New Album"
                submitLabel="Create"
            />
        </>
    );
};

export default AlbumSelectionModal;