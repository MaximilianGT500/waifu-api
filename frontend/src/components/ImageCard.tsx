import { ImageDto, Role, ReviewStatus } from '../types';
import { Heart, Trash2, Pencil, FolderMinus, FolderPlus, Clock, GitPullRequest, ImageOff, Check } from 'lucide-react';
import { Link } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import TagChip from './TagChip';
import { useAuth } from '../context/AuthContext';
import { useMyPendingImageEdits } from '../hooks/useMyPendingImageEdits';
import { useAuthGuard } from '../hooks/useRequireAuth';
import { useHolo } from '../hooks/useHolo';
import { sfx } from '../utils/sfx';
import { ReactNode, useEffect, useRef, useState, MouseEvent } from 'react';
import api from '../services/api';
import AlbumSelectionModal from './modals/AlbumSelectionModal';
import { imgCard, imgCardSrcSet } from '../utils/cfImage';
import { safeColor } from '../utils/color';
import { patchCachedImage } from '../utils/imageCache';
import { nowIso } from '../utils/time';
import { prefetchImageDetail } from '../hooks/useImageDetail';

interface ImageCardProps {
    image: ImageDto;
    eager?: boolean;
    priority?: boolean;
    onDelete?: (id: number) => void;
    onRemove?: (id: number) => void;
    onEdit?: (image: ImageDto) => void;
    onSuggest?: (image: ImageDto) => void;
    forceOverlay?: boolean;
    readOnly?: boolean;
    showTags?: boolean;
    onOpen?: () => void;
    selected?: boolean;
    onToggleSelect?: (id: number, range: boolean) => void;
    featured?: boolean;
}

const INTENT_DELAY_MS = 90;

export const CARD_SIZES = '(min-width: 1280px) 13rem, (min-width: 1024px) 16rem, (min-width: 640px) 30vw, 32vw';
const FEATURED_CARD_SIZES = '(min-width: 1280px) 27rem, (min-width: 1024px) 33rem, (min-width: 640px) 62vw, 66vw';

const OverlayButton = ({ label, onClick, children, danger = false }: { label: string; onClick: () => void; children: ReactNode; danger?: boolean }) => (
    <button
        type="button"
        title={label}
        aria-label={label}
        onClick={(e: MouseEvent) => { e.preventDefault(); e.stopPropagation(); onClick(); }}
        className={`pointer-events-auto grid size-8 place-items-center rounded-full shadow-card transition-colors ${danger ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90' : 'bg-card/92 text-foreground hover:bg-card'}`}
    >
        {children}
    </button>
);

const CornerSticker = ({ children, tone }: { children: ReactNode; tone: 'nsfw' | 'gif' }) => (
    <span className={`rounded-full px-1.5 py-px text-[0.625rem] font-black leading-4 shadow-card @[8.5rem]:px-2 @[8.5rem]:py-0.5 @[8.5rem]:text-[0.6875rem] @[8.5rem]:leading-5 ${tone === 'nsfw' ? 'bg-destructive text-destructive-foreground' : 'bg-card text-foreground'}`}>
        {children}
    </span>
);

const TAG_CAP = 6;

const ImageCard = ({ image, eager = false, priority = false, onDelete, onRemove, onEdit, onSuggest, forceOverlay = false, readOnly = false, showTags = false, onOpen, selected = false, onToggleSelect, featured = false }: ImageCardProps) => {
    const { user } = useAuth();
    const checkAuth = useAuthGuard();
    const queryClient = useQueryClient();
    const holo = useHolo<HTMLDivElement>();
    const [isLiked, setIsLiked] = useState(!!image.likedAt);
    const [likesCount, setLikesCount] = useState(image.favorites);
    const [localAlbums, setLocalAlbums] = useState(image.albums || []);
    const [syncedImage, setSyncedImage] = useState(image);
    const [isLikeLoading, setIsLikeLoading] = useState(false);
    const [isImageLoaded, setIsImageLoaded] = useState(false);
    const [isImageBroken, setIsImageBroken] = useState(false);
    const [isAlbumModalOpen, setIsAlbumModalOpen] = useState(false);
    const intentTimer = useRef<number | undefined>(undefined);

    useEffect(() => () => window.clearTimeout(intentTimer.current), []);

    const prefetchDetail = () => {
        window.clearTimeout(intentTimer.current);
        void prefetchImageDetail(queryClient, image.id, user);
    };
    const onPointerIntent = () => {
        window.clearTimeout(intentTimer.current);
        intentTimer.current = window.setTimeout(prefetchDetail, INTENT_DELAY_MS);
    };
    const cancelIntent = () => window.clearTimeout(intentTimer.current);

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
        if (!prevLiked) sfx('DOKI!', e.currentTarget as Element);
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

    const selecting = !!onToggleSelect;
    const showActions = !selecting && (!readOnly && (!!user || !!onRemove) || !!onEdit || !!onSuggest || (!!onDelete && !!isAdmin));
    const revealOnHover = forceOverlay ? 'flex' : 'hidden @[8.5rem]:flex xl:opacity-0 xl:group-hover:opacity-100 xl:group-focus-within:opacity-100';

    return (
        <article data-featured={featured ? '' : undefined} className="card-shell @container group relative flex min-w-0 flex-col [contain-intrinsic-size:auto_16rem] [content-visibility:auto]">
            <div
                onPointerMove={holo.onPointerMove}
                onPointerLeave={holo.onPointerLeave}
                data-selected={selected ? '' : undefined}
                className={`holo-card relative rounded-[1.125rem] bg-card p-1.5 shadow-card ring-offset-2 ring-offset-background hover:shadow-lift has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-ring has-[button[aria-pressed]:focus-visible]:ring-2 has-[button[aria-pressed]:focus-visible]:ring-ring dark:ring-1 dark:ring-white/6 ${selected ? 'ring-[3px]! ring-inset! ring-brand! dark:ring-brand!' : ''}`}
            >
                {selecting && (
                    <button
                        type="button"
                        aria-pressed={selected}
                        aria-label={`Select image #${image.id}`}
                        onClick={(e) => onToggleSelect(image.id, e.shiftKey)}
                        className="absolute inset-0 z-10 cursor-pointer rounded-[inherit] outline-none"
                    >
                        <span
                            aria-hidden="true"
                            className={`absolute right-3 top-3 grid size-7 place-items-center rounded-full shadow-card transition-colors ${selected ? 'bg-brand text-white' : 'bg-card/90 text-transparent ring-2 ring-inset ring-foreground/30'}`}
                        >
                            <Check size={16} strokeWidth={3} />
                        </span>
                    </button>
                )}
                <Link
                    to={`/images/${image.id}`}
                    onClick={onOpen}
                    onPointerEnter={onPointerIntent}
                    onPointerLeave={cancelIntent}
                    onFocus={prefetchDetail}
                    onTouchStart={prefetchDetail}
                    data-nsfw={image.isNsfw ? '' : undefined}
                    className="relative block overflow-hidden rounded-[0.8rem] bg-sleeve outline-none"
                    style={{ aspectRatio: '5 / 7', backgroundColor: placeholder ?? undefined }}
                >
                    {isImageBroken ? (
                        <span className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-sleeve text-xs text-muted-foreground">
                            <ImageOff size={22} aria-hidden="true" />
                            <span>Image unavailable</span>
                        </span>
                    ) : (
                        <img
                            src={imgCard(image.url)}
                            srcSet={imgCardSrcSet(image.url, image.width)}
                            sizes={featured ? FEATURED_CARD_SIZES : CARD_SIZES}
                            alt={altText}
                            width={image.width}
                            height={image.height}
                            loading={eager ? 'eager' : 'lazy'}
                            fetchPriority={priority ? 'high' : 'auto'}
                            decoding="async"
                            onLoad={() => setIsImageLoaded(true)}
                            onError={() => setIsImageBroken(true)}
                            className={`size-full object-cover object-[50%_28%] transition-opacity duration-500 ${isImageLoaded ? 'opacity-100' : 'opacity-0'}`}
                        />
                    )}
                    <span className="holo-foil" aria-hidden="true" />
                    {(image.isNsfw || image.isAnimated) && (
                        <span className="pointer-events-none absolute left-1.5 top-1.5 flex gap-1 @[8.5rem]:left-2 @[8.5rem]:top-2">
                            {image.isNsfw && <CornerSticker tone="nsfw">NSFW</CornerSticker>}
                            {image.isAnimated && <CornerSticker tone="gif">GIF</CornerSticker>}
                        </span>
                    )}
                </Link>

                {showActions && (
                    <div className={`pointer-events-none absolute right-3 top-3 flex-col gap-1.5 transition-opacity duration-200 ${revealOnHover}`}>
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

                <div className="flex items-center gap-2 px-1 pb-0.5 pt-1.5 @[8.5rem]:px-1.5 @[8.5rem]:pt-2">
                    <span className="text-[0.8125rem] font-black leading-none tabular @[8.5rem]:text-sm">#{image.id}</span>
                    <span className="hidden truncate text-[0.6875rem] leading-none text-muted-foreground tabular @[10rem]:inline">{image.width}×{image.height}</span>
                    {readOnly ? (
                        <span className="ml-auto flex items-center gap-1 text-xs font-bold text-muted-foreground" aria-label={`${likesCount} favorites`}>
                            <Heart size={13} aria-hidden="true" />
                            <span className="tabular" aria-hidden="true">{likesCount}</span>
                        </span>
                    ) : (
                        <button
                            type="button"
                            onClick={toggleLike}
                            disabled={isLikeLoading}
                            aria-pressed={isLiked}
                            aria-label={`${isLiked ? 'Remove image #' + image.id + ' from favorites' : 'Add image #' + image.id + ' to favorites'} (${likesCount} favorites)`}
                            className={`ml-auto -my-1 -mr-1 flex items-center gap-1 rounded-full px-2 py-1 text-xs font-bold transition-colors ${isLiked ? 'text-brand-ink hover:bg-brand-soft' : 'text-muted-foreground hover:bg-secondary hover:text-foreground'}`}
                        >
                            <Heart size={14} aria-hidden="true" className={isLiked ? 'fill-current' : ''} />
                            <span className="tabular" aria-hidden="true">{likesCount}</span>
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
                <div className="mt-2.5 flex flex-wrap gap-1 px-0.5">
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
        </article>
    );
};

export default ImageCard;
