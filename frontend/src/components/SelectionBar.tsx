import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { CheckCheck, Download, Ellipsis, FolderMinus, FolderPlus, Heart, Link2, Loader2, Play, X } from 'lucide-react';
import { ImageDto } from '../types';
import { useNotification } from '../context/NotificationContext';
import BatchAlbumDialog from './modals/BatchAlbumDialog';
import { applyFavoriteChange, batchAlbumImages, pluralImages } from '../utils/albumBatch';
import { Dropdown, DropdownItem, DropdownTrigger } from './Dropdown';
import { LINK_MAX_IMAGES, ZIP_MAX_IMAGES, downloadImagesAsZip, selectionLink } from '../utils/selectionExport';
import { copyText } from '../utils/clipboard';
import { saveBrowseContext } from '../utils/browseContext';

interface SelectionBarProps {
    images: ImageDto[];
    selected: ReadonlySet<number>;
    onSelectAll: () => void;
    onClear: () => void;
    onExit: () => void;
    removeFromAlbum?: { id: number; name: string; onRemoved: () => void };
}

const SelectionBar = ({ images, selected, onSelectAll, onClear, onExit, removeFromAlbum }: SelectionBarProps) => {
    const queryClient = useQueryClient();
    const { showNotification } = useNotification();
    const [isAlbumOpen, setIsAlbumOpen] = useState(false);
    const [busy, setBusy] = useState<'favorite' | 'remove' | 'zip' | null>(null);
    const [zipProgress, setZipProgress] = useState<string | null>(null);
    const navigate = useNavigate();
    const location = useLocation();
    const chosen = images.filter((image) => selected.has(image.id));
    const count = chosen.length;
    const allLiked = count > 0 && chosen.every((image) => !!image.likedAt);

    const toggleFavorites = async () => {
        if (busy || count === 0) return;
        setBusy('favorite');
        const like = !allLiked;
        try {
            const result = await batchAlbumImages('favorites', like ? { add: chosen.map((i) => i.id) } : { remove: chosen.map((i) => i.id) });
            applyFavoriteChange(queryClient, chosen.filter((image) => !result.skipped.includes(image.id)), like);
            void queryClient.invalidateQueries({ queryKey: ['my-albums-all'] });
            showNotification('success', like ? `Added ${pluralImages(result.added)} to your favorites` : `Removed ${pluralImages(result.removed)} from your favorites`);
        } catch {
            showNotification('error', 'Your favorites could not be updated. Please try again.');
        } finally {
            setBusy(null);
        }
    };

    const removeSelection = async () => {
        if (!removeFromAlbum || busy || count === 0) return;
        setBusy('remove');
        try {
            const result = await batchAlbumImages(removeFromAlbum.id, { remove: chosen.map((i) => i.id) });
            showNotification('success', `Removed ${pluralImages(result.removed)} from ${removeFromAlbum.name}`);
            onClear();
            removeFromAlbum.onRemoved();
        } catch {
            showNotification('error', 'The album could not be updated. Please try again.');
        } finally {
            setBusy(null);
        }
    };

    const copyLink = async () => {
        if (count === 0) return;
        const ok = await copyText(selectionLink(chosen));
        if (!ok) {
            showNotification('error', 'Could not copy to the clipboard');
            return;
        }
        showNotification('success', count > LINK_MAX_IMAGES ? `Link to the first ${LINK_MAX_IMAGES} images copied` : 'Link to the selection copied');
    };

    const playSlideshow = () => {
        if (count === 0) return;
        saveBrowseContext({ ids: chosen.map((image) => image.id), from: `${location.pathname}${location.search}` });
        navigate(`/images/${chosen[0].id}`, { state: { slideshow: true } });
    };

    const downloadZip = async () => {
        if (busy || count === 0) return;
        setBusy('zip');
        setZipProgress(`0/${Math.min(count, ZIP_MAX_IMAGES)}`);
        try {
            const { saved, failed } = await downloadImagesAsZip(chosen, (done, total) => setZipProgress(`${done}/${total}`));
            if (saved === 0) showNotification('error', 'None of the images could be downloaded.');
            else if (failed > 0) showNotification('warning', `Downloaded ${pluralImages(saved)}, ${pluralImages(failed)} could not be fetched.`);
            else showNotification('success', count > ZIP_MAX_IMAGES ? `Downloaded the first ${pluralImages(saved)} as a ZIP file` : `Downloaded ${pluralImages(saved)} as a ZIP file`);
        } finally {
            setBusy(null);
            setZipProgress(null);
        }
    };

    return (
        <>
            <div
                role="toolbar"
                aria-label="Selection"
                className="sticky bottom-4 z-30 mx-auto mt-2 flex w-fit max-w-full flex-wrap items-center justify-center gap-1.5 rounded-[1.5rem] bg-popover p-1.5 pl-4 text-popover-foreground shadow-float ring-1 ring-border animate-in fade-in slide-in-from-bottom-4 duration-300 dark:ring-white/8"
            >
                <p className="mr-1 text-sm font-black tabular" aria-live="polite">{count === 0 ? 'Tap images to select them' : `${pluralImages(count)} selected`}</p>
                <button type="button" onClick={onSelectAll} className="btn btn-ghost btn-sm">
                    <CheckCheck size={15} aria-hidden="true" /> All on this page
                </button>
                <button type="button" onClick={toggleFavorites} disabled={count === 0 || !!busy} className="btn btn-secondary btn-sm">
                    {busy === 'favorite' ? <Loader2 size={15} className="animate-spin" aria-hidden="true" /> : <Heart size={15} aria-hidden="true" className={allLiked ? 'fill-current' : ''} />}
                    {allLiked ? 'Unfavorite' : 'Favorite'}
                </button>
                <button type="button" onClick={() => setIsAlbumOpen(true)} disabled={count === 0 || !!busy} className="btn btn-secondary btn-sm">
                    <FolderPlus size={15} aria-hidden="true" /> Albums…
                </button>
                {removeFromAlbum && (
                    <button type="button" onClick={removeSelection} disabled={count === 0 || !!busy} className="btn btn-danger-soft btn-sm">
                        {busy === 'remove' ? <Loader2 size={15} className="animate-spin" aria-hidden="true" /> : <FolderMinus size={15} aria-hidden="true" />}
                        Remove
                    </button>
                )}
                <Dropdown
                    width="w-64"
                    label="More actions for the selection"
                    className="bottom-full mb-2 mt-0! origin-bottom-right!"
                    align="right"
                    trigger={
                        <DropdownTrigger className="btn btn-secondary btn-sm" disabled={count === 0 || !!busy}>
                            {busy === 'zip' ? <Loader2 size={15} className="animate-spin" aria-hidden="true" /> : <Ellipsis size={15} aria-hidden="true" />}
                            {zipProgress ?? 'More'}
                        </DropdownTrigger>
                    }
                >
                    <DropdownItem onClick={playSlideshow} icon={<Play size={15} />}>Slideshow of the selection</DropdownItem>
                    <DropdownItem onClick={copyLink} icon={<Link2 size={15} />}>Copy a link to the selection</DropdownItem>
                    <DropdownItem onClick={downloadZip} icon={<Download size={15} />}>Download as ZIP{count > ZIP_MAX_IMAGES ? ` (first ${ZIP_MAX_IMAGES})` : ''}</DropdownItem>
                </Dropdown>
                {count > 0 && (
                    <button type="button" onClick={onClear} className="btn btn-ghost btn-sm">Clear</button>
                )}
                <button type="button" onClick={onExit} className="btn btn-primary btn-sm" aria-label="Stop selecting (Esc)">
                    <X size={15} aria-hidden="true" /> Done
                </button>
            </div>
            <BatchAlbumDialog isOpen={isAlbumOpen} onClose={() => setIsAlbumOpen(false)} images={chosen} onDone={onClear} />
        </>
    );
};

export default SelectionBar;
