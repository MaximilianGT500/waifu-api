import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Folder, FolderMinus, FolderPlus, Heart, Loader2 } from 'lucide-react';
import Modal from '../Modal';
import api from '../../services/api';
import { AlbumDto, ImageDto, PaginatedList } from '../../types';
import { useNotification } from '../../context/NotificationContext';
import { applyFavoriteChange, batchAlbumImages, pluralImages } from '../../utils/albumBatch';

interface BatchAlbumDialogProps {
    isOpen: boolean;
    onClose: () => void;
    images: ImageDto[];
    onDone?: () => void;
}

const BatchAlbumDialog = ({ isOpen, onClose, images, onDone }: BatchAlbumDialogProps) => {
    const queryClient = useQueryClient();
    const { showNotification } = useNotification();
    const [busy, setBusy] = useState<string | null>(null);
    const albums = useQuery({
        queryKey: ['my-albums-all'],
        enabled: isOpen,
        staleTime: 30_000,
        queryFn: async ({ signal }) => (await api.get<PaginatedList<AlbumDto>>('/users/me/albums', { params: { pageSize: 100 }, signal })).data.items,
    });

    const ids = images.map((image) => image.id);

    const run = async (album: AlbumDto, mode: 'add' | 'remove') => {
        if (busy) return;
        setBusy(`${album.id}-${mode}`);
        try {
            const result = await batchAlbumImages(album.id, mode === 'add' ? { add: ids } : { remove: ids });
            if (album.isDefault) applyFavoriteChange(queryClient, images.filter((image) => !result.skipped.includes(image.id)), mode === 'add');
            void queryClient.invalidateQueries({ queryKey: ['my-albums-all'] });
            void queryClient.invalidateQueries({ queryKey: ['albums'] });
            if (!album.isDefault) void queryClient.invalidateQueries({ queryKey: ['images'] });
            const changed = mode === 'add' ? result.added : result.removed;
            const verb = mode === 'add' ? `Added ${pluralImages(changed)} to` : `Removed ${pluralImages(changed)} from`;
            const skipped = result.skipped.length > 0 ? ` ${pluralImages(result.skipped.length)} could not be found.` : '';
            showNotification('success', `${verb} ${album.name}.${skipped}`);
            onDone?.();
            onClose();
        } catch {
            showNotification('error', 'The album could not be updated. Please try again.');
        } finally {
            setBusy(null);
        }
    };

    return (
        <Modal isOpen={isOpen} onClose={onClose} title={`${pluralImages(images.length)} selected`} maxWidth="max-w-lg">
            <p className="text-sm text-muted-foreground">Add the selection to an album or take it out again. Images that are already there are left alone.</p>
            <div className="mt-4 space-y-2">
                {albums.isPending && Array.from({ length: 3 }, (_, i) => (
                    <div key={i} className="h-14 animate-pulse rounded-[var(--radius-control)] bg-sleeve" aria-hidden="true" />
                ))}
                {albums.isError && <p className="text-sm font-bold text-destructive">Your albums could not be loaded.</p>}
                {albums.data?.map((album) => (
                    <div key={album.id} className="flex items-center gap-3 rounded-[var(--radius-control)] bg-secondary/60 p-2 pl-3 ring-1 ring-border/60">
                        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-card text-muted-foreground shadow-card" aria-hidden="true">
                            {album.isDefault ? <Heart size={16} className="fill-brand text-brand" /> : <Folder size={16} />}
                        </span>
                        <span className="min-w-0 flex-1">
                            <span className="block truncate font-bold">{album.name}</span>
                            <span className="block text-xs text-muted-foreground tabular">{pluralImages(album.imageCount ?? 0)}</span>
                        </span>
                        <button type="button" onClick={() => run(album, 'remove')} disabled={!!busy} className="btn btn-ghost btn-sm" aria-label={`Remove the selection from ${album.name}`} title="Remove the selection">
                            {busy === `${album.id}-remove` ? <Loader2 size={15} className="animate-spin" aria-hidden="true" /> : <FolderMinus size={15} aria-hidden="true" />}
                        </button>
                        <button type="button" onClick={() => run(album, 'add')} disabled={!!busy} className="btn btn-secondary btn-sm">
                            {busy === `${album.id}-add` ? <Loader2 size={15} className="animate-spin" aria-hidden="true" /> : <FolderPlus size={15} aria-hidden="true" />}
                            Add
                        </button>
                    </div>
                ))}
            </div>
        </Modal>
    );
};

export default BatchAlbumDialog;
