import { ImageDto, Role, ReviewStatus } from '../../types';
import { Heart, Trash2, Pencil, FolderMinus, FolderPlus, Clock, GitPullRequest, ImageOff } from 'lucide-react';
import { Link } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import TagChip from './TagChip';
import { useAuth } from '../../context/AuthContext';
import { useMyPendingImageEdits } from '../../hooks/useMyPendingImageEdits';
import { useAuthGuard } from '../../hooks/useRequireAuth';
import { ReactNode, useState, MouseEvent } from 'react';
import api from '../../services/api';
import AlbumSelectionModal from './modals/AlbumSelectionModal';
import { imgCard } from '../../utils/cfImage';
import { safeColor } from '../../utils/color';
import { patchCachedImage } from '../../utils/imageCache';
import { nowIso } from '../../utils/time';

interface ImageCardProps {
    image: ImageDto;
    onDelete?: (id: number) => void;
    onRemove?: (id: number) => void;
    onEdit?: (image: ImageDto) => void;
    onSuggest?: (image: ImageDto) => void;
    forceOverlay?: boolean;
    readOnly?: boolean;
    showTags?: boolean;
}

const OverlayButton = ({ label, onClick, children, danger = false }: { label: string; onClick: () => void; children: ReactNode; danger?: boolean }) => (
    <button
        type="button"
        title={label}
        aria-label={label}
        onClick={(e: MouseEvent) => { e.preventDefault(); e.stopPropagation(); onClick(); }}
        className={`pointer-events-auto grid size-8 place-items-center rounded-full text-white backdrop-blur-md transition-colors ${danger ? 'bg-rose-600/85 hover:bg-rose-600' : 'bg-zinc-950/55 hover:bg-zinc-950/80'}`}
    >
        {children}
    </button>
);

const TAG_CAP = 6;

const ImageCard = ({ image, onDelete, onRemove, onEdit, onSuggest, forceOverlay = false, readOnly = false, showTags = false }: ImageCardProps) => {
    const { user } = useAuth();
    const checkAuth = useAuthGuard();
    const queryClient = useQueryClient();
    const [isLiked, setIsLiked] = useState(!!image.likedAt);
    const [likesCount, setLikesCount] = useState(image.favorites);
    const [localAlbums, setLocalAlbums] = useState(image.albums || []);
    const [syncedImage, setSyncedImage] = useState(image);
    const [isLikeLoading, setIsLikeLoading] = useState(false);
    const [isImageLoaded, setIsImageLoaded] = useState(false);
    const [isImageBroken, setIsImageBroken] = useState(false);
    const [isAlbumModalOpen, setIsAlbumModalOpen] = useState(false);

    if (syncedImage !== image) {
        setSyncedImage(image);
        setIsLiked(!!image.likedAt);
        setLikesCount(image.favorites);
        setLocalAlbums(image.albums || []);
    }

    const isAdmin = user && user.role === Role.Admin;
    const artistNames = (image.artists ?? []).map(a => a.name).join(', ');
    const altText = artistNames ? `Image #${image.id} by ${artistNames}` : `Image #${image.id}`;
    const placeholder = safeColor(image.dominantColor);

    const { data: pendingEdits } = useMyPendingImageEdits(showTags);
    const edit = pendingEdits?.get(image.id);
    const removedTagIds = new Set((edit?.removeTags ?? []).map(t => t.id));
    const addedTags = edit?.addTags ?? [];

    const allTags = image.tags ?? [];
    const removedTags = allTags.filter(t => removedTagIds.has(t.id));
    const myPendingTags = allTags.filter(t => t.reviewStatus === ReviewStatus.Pending && !removedTagIds.has(t.id));
    const acceptedTags = allTags.filter(t => t.reviewStatus !== ReviewStatus.Pending && !removedTagIds.has(t.id));
    const shownTags = acceptedTags.slice(0, TAG_CAP);
    const hiddenTagCount = acceptedTags.length - shownTags.length;
    const hasAnyTag = acceptedTags.length || myPendingTags.length || removedTags.length || addedTags.length;

    const toggleLike = async (e: MouseEvent) => {
        e.preventDefault();
        e.stopPropagation();
        if (!checkAuth('You must be logged in to like images.', `/images/${image.id}`)) return;
        if (isLikeLoading) return;

        setIsLikeLoading(true);
        const prevLiked = isLiked;
        const nextCount = prevLiked ? likesCount - 1 : likesCount + 1;
        setIsLiked(!prevLiked);
        setLikesCount(nextCount);

        try {
            if (prevLiked) {
                await api.delete(`/users/me/albums/favorites/images/${image.id}`);
            } else {
                await api.post(`/users/me/albums/favorites/images/${image.id}`);
            }
            const albums = prevLiked ? localAlbums.filter(a => !a.isDefault) : localAlbums;
            setLocalAlbums(albums);
            patchCachedImage(queryClient, image.id, {
                likedAt: prevLiked ? undefined : nowIso(),
                favorites: nextCount,
                albums,
            });
        } catch {
            setIsLiked(prevLiked);
            setLikesCount(likesCount);
        } finally {
            setIsLikeLoading(false);
        }
    };

    const handleAlbumUpdate = (updatedImage: ImageDto) => {
        setIsLiked(!!updatedImage.likedAt);
        setLikesCount(updatedImage.favorites);
        setLocalAlbums(updatedImage.albums || []);
        patchCachedImage(queryClient, image.id, {
            albums: updatedImage.albums,
            likedAt: updatedImage.likedAt,
            favorites: updatedImage.favorites,
        });
    };

    const showActions = !readOnly && (!!user || !!onRemove) || !!onEdit || !!onSuggest || (!!onDelete && !!isAdmin);
    const revealOnHover = 'xl:opacity-0 xl:group-hover:opacity-100 xl:group-focus-within:opacity-100';

    return (
        <div className="group relative flex flex-col">
            <div
                className="relative overflow-hidden rounded-[1.25rem] bg-secondary ring-offset-2 ring-offset-canvas has-[>a:focus-visible]:ring-2 has-[>a:focus-visible]:ring-ring"
                style={placeholder ? { backgroundColor: placeholder } : undefined}
            >
                <Link to={`/images/${image.id}`} className="block w-full outline-none">
                    <div className="relative w-full" style={{ aspectRatio: `${image.width} / ${image.height}` }}>
                        {isImageBroken ? (
                            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-secondary text-xs text-muted-foreground">
                                <ImageOff size={22} aria-hidden="true" />
                                <span>Image unavailable</span>
                            </div>
                        ) : (
                            <img
                                src={imgCard(image.url)}
                                alt={altText}
                                loading="lazy"
                                decoding="async"
                                onLoad={() => setIsImageLoaded(true)}
                                onError={() => setIsImageBroken(true)}
                                className={`size-full object-cover transition-opacity duration-500 ${isImageLoaded ? 'opacity-100' : 'opacity-0'}`}
                            />
                        )}
                    </div>
                </Link>

                {showActions && (
                    <div className={`pointer-events-none absolute inset-x-0 top-0 justify-end gap-1.5 p-2.5 transition-opacity duration-200 ${forceOverlay ? 'flex' : 'hidden xl:flex'} ${revealOnHover}`}>
                        {!readOnly && user && (
                            <OverlayButton label="Add to Album" onClick={() => setIsAlbumModalOpen(true)}>
                                <FolderPlus size={15} aria-hidden="true" />
                            </OverlayButton>
                        )}
                        {!readOnly && onRemove && (
                            <OverlayButton label="Remove from Album" onClick={() => onRemove(image.id)}>
                                <FolderMinus size={15} aria-hidden="true" />
                            </OverlayButton>
                        )}
                        {onEdit && (
                            <OverlayButton label="Edit Image" onClick={() => onEdit(image)}>
                                <Pencil size={14} aria-hidden="true" />
                            </OverlayButton>
                        )}
                        {onSuggest && (
                            <OverlayButton label="Suggest an edit" onClick={() => onSuggest(image)}>
                                <GitPullRequest size={15} aria-hidden="true" />
                            </OverlayButton>
                        )}
                        {onDelete && isAdmin && (
                            <OverlayButton label="Delete" onClick={() => onDelete(image.id)} danger>
                                <Trash2 size={15} aria-hidden="true" />
                            </OverlayButton>
                        )}
                    </div>
                )}

                <div className={`pointer-events-none absolute inset-x-0 bottom-0 flex items-end justify-between bg-linear-to-t from-zinc-950/65 via-zinc-950/20 to-transparent px-3 pb-2.5 pt-10 text-white transition-opacity duration-200 ${revealOnHover}`}>
                    <span className="text-xs font-semibold tabular text-white/90">#{image.id}</span>
                    {readOnly ? (
                        <span className="flex items-center gap-1.5 rounded-full bg-white/15 px-2 py-1 backdrop-blur-md" aria-label={`${likesCount} favorites`}>
                            <Heart size={13} aria-hidden="true" />
                            <span className="text-xs font-bold tabular" aria-hidden="true">{likesCount}</span>
                        </span>
                    ) : (
                        <button
                            type="button"
                            onClick={toggleLike}
                            disabled={isLikeLoading}
                            aria-pressed={isLiked}
                            aria-label={`${isLiked ? 'Remove image #' + image.id + ' from favorites' : 'Add image #' + image.id + ' to favorites'} (${likesCount} favorites)`}
                            className="pointer-events-auto flex items-center gap-1.5 rounded-full bg-white/15 px-2 py-1 backdrop-blur-md transition-colors hover:bg-white/25 focus-visible:outline-2 focus-visible:outline-white"
                        >
                            <Heart size={13} aria-hidden="true" className={`transition-colors ${isLiked ? 'fill-rose-500 text-rose-500' : 'text-white'}`} />
                            <span className="text-xs font-bold tabular" aria-hidden="true">{likesCount}</span>
                        </button>
                    )}
                </div>
            </div>

            {!readOnly && user && (
                <AlbumSelectionModal
                    isOpen={isAlbumModalOpen}
                    onClose={() => setIsAlbumModalOpen(false)}
                    image={{ ...image, albums: localAlbums }}
                    onUpdate={handleAlbumUpdate}
                />
            )}

            {showTags && (
                <div className="mt-2 flex flex-wrap gap-1 px-0.5">
                    {shownTags.map(t => (
                        <TagChip key={`t-${t.id}`} label={t.name} href={`/gallery?includedTags=${encodeURIComponent(t.slug)}`} />
                    ))}
                    {hiddenTagCount > 0 && (
                        <TagChip label={`+${hiddenTagCount} more`} href={`/images/${image.id}`} title="See all tags" />
                    )}
                    {addedTags.map(t => (
                        <TagChip key={`add-${t.id}`} tone="add" label={t.name}
                            href={`/gallery?includedTags=${encodeURIComponent(t.slug)}`}
                            trailingIcon={t.reviewStatus === ReviewStatus.Pending ? <Clock size={11} className="shrink-0" /> : undefined}
                            title={t.reviewStatus === ReviewStatus.Pending
                                ? 'You proposed adding this tag (also awaiting review)'
                                : 'You proposed adding this tag (awaiting review)'} />
                    ))}
                    {removedTags.map(t => (
                        <TagChip key={`rm-${t.id}`} tone="remove" label={t.name} title="You proposed removing this tag (awaiting review)" />
                    ))}
                    {myPendingTags.map(t => (
                        <TagChip key={`p-${t.id}`} tone="pending" label={t.name} title="Your tag (awaiting review)" />
                    ))}
                    {!hasAnyTag && <span className="text-xs text-muted-foreground">No tags yet</span>}
                </div>
            )}
        </div>
    );
};

export default ImageCard;
