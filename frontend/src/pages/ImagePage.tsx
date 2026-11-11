import { useState, useEffect, ReactNode, CSSProperties } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useParams, useNavigate, useLocation, Link } from 'react-router';
import api, { httpStatus } from '../services/api';
import { ImageDto, Role, ImageFormData, ReviewStatus, ReviewableContentType, NsfwMode } from '../types';
import {
  Heart, Trash2, Edit, FolderPlus, Flag, Image, GitPullRequest, Clock, Plus, Download, Link2, Check,
  Share2, Braces, Maximize2, ChevronLeft, ChevronRight, ArrowLeft, ImageOff, Code2, MessageSquareCode, Send, Play, Copy, Palette,
} from 'lucide-react';
import { useMyPendingImageEdits } from '../hooks/useMyPendingImageEdits';
import { invalidateNavBadges } from '../hooks/useNavBadges';
import { useAuth } from '../context/AuthContext';
import { useNotification } from '../context/NotificationContext';
import { useNsfwConsent } from '../hooks/useNsfwConsent';
import { useImageUpdate } from '../hooks/useImageUpdate';
import { useAuthGuard } from '../hooks/useRequireAuth';
import { useImageDetail, imageDetailKey, prefetchImageDetail } from '../hooks/useImageDetail';
import { useHotkeys } from '../hooks/useHotkeys';
import ImageModal from '../components/modals/ImageModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import ReasonModal from '../components/modals/ReasonModal';
import AlbumSelectionModal from '../components/modals/AlbumSelectionModal';
import ApiRequestDialog from '../components/modals/ApiRequestDialog';
import NsfwWarning from '../components/NsfwWarning';
import Lightbox from '../components/Lightbox';
import RelatedImages from '../components/RelatedImages';
import SimilarImages from '../components/SimilarImages';
import ColorPalette from '../components/ColorPalette';
import { sfx } from '../utils/sfx';
import { normalizeHex } from '../utils/imageFilters';
import { Dropdown, DropdownTrigger, DropdownItem, DropdownSeparator } from '../components/Dropdown';
import NotFound from './NotFound';
import { MetaTags } from '../hooks/useMetaTags';
import { isBot } from '../utils/bot';
import ExternalLink from '../components/ExternalLink';
import StatusBadge, { StatusTone } from '../components/StatusBadge';
import { displayUrl, safeExternalUrl } from '../utils/safeUrl';
import { safeColor } from '../utils/color';
import { nowIso } from '../utils/time';
import { isId } from '../utils/ids';
import { useHolo } from '../hooks/useHolo';
import { imgCard, imgPreview, imgPreviewSrcSet } from '../utils/cfImage';
import { patchCachedImage } from '../utils/imageCache';
import { copyText } from '../utils/clipboard';
import { extendBrowseContext, neighboursOf } from '../utils/browseContext';
import { useBrowseContext } from '../hooks/useBrowseContext';
import { addRecentImage, removeRecentImage } from '../utils/recentImages';

const DETAIL_SIZES = '(min-width: 1280px) min(56vw, 62rem), (min-width: 1024px) 60vw, 100vw';

const StatRow = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex min-w-0 items-center justify-between gap-4 py-3">
    <dt className="shrink-0 text-muted-foreground">{label}</dt>
    <dd className="min-w-0 text-right font-bold tabular">{children}</dd>
  </div>
);

const formatBytes = (bytes: number) => {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
};

const ImagePage = () => {
  const { id } = useParams<{ id: string }>();
  if (!isId(id)) return <NotFound />;
  return <ImageView id={id} />;
};

const preloadDetailImage = (image: ImageDto, original: boolean) => {
  const url = safeExternalUrl(image.url);
  if (!url || image.isNsfw) return;
  const preload = new window.Image();
  preload.decoding = 'async';
  if (original) {
    preload.src = url;
    return;
  }
  const srcSet = imgPreviewSrcSet(url, image.width);
  if (srcSet) {
    preload.sizes = DETAIL_SIZES;
    preload.srcset = srcSet;
  }
  preload.src = imgPreview(url);
};

const ImageView = ({ id }: { id: string }) => {
  const navigate = useNavigate();
  const location = useLocation();
  const wantsSlideshow = !!(location.state as { slideshow?: boolean } | null)?.slideshow;
  const { user } = useAuth();
  const { showNotification } = useNotification();
  const checkAuth = useAuthGuard();
  const queryClient = useQueryClient();
  const { data: pendingEdits } = useMyPendingImageEdits(!!user);

  const detail = useImageDetail(id);
  const image = detail.data ?? null;
  const loadStatus = httpStatus(detail.error);
  const notFound = loadStatus === 404 || loadStatus === 400;

  const { grantConsent, hasConsent } = useNsfwConsent(false);
  const [consented, setConsented] = useState(() => hasConsent() || isBot());
  const { updateImage } = useImageUpdate();

  const [showEditModal, setShowEditModal] = useState(false);
  const [showSuggestModal, setShowSuggestModal] = useState(false);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [isReportModalOpen, setIsReportModalOpen] = useState(false);
  const [isAlbumModalOpen, setIsAlbumModalOpen] = useState(false);
  const [isApiOpen, setIsApiOpen] = useState(false);
  const [isViewerOpen, setIsViewerOpen] = useState(wantsSlideshow);
  const [autoPlay, setAutoPlay] = useState(wantsSlideshow);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [isDownloading, setIsDownloading] = useState(false);
  const [isLiking, setIsLiking] = useState(false);
  const [loadedSrc, setLoadedSrc] = useState<string | null>(null);
  const [brokenSrc, setBrokenSrc] = useState<string | null>(null);
  const browse = useBrowseContext();
  const holo = useHolo<HTMLDivElement>(4);

  const numericId = Number(id);
  const nav = neighboursOf(browse, numericId);
  const isAdminOrModerator = user && (user.role === Role.Admin || user.role === Role.Moderator);
  const pageUrl = `${window.location.origin}/images/${id}`;
  const needsConsent = !!image?.isNsfw && !consented;

  useEffect(() => {
    if (image && !detail.isPlaceholderData) addRecentImage(image);
  }, [image, detail.isPlaceholderData]);

  useEffect(() => {
    if (notFound) removeRecentImage(numericId);
  }, [notFound, numericId]);

  const nextId = nav?.next;
  const prevId = nav?.prev;
  useEffect(() => {
    if (!nextId && !prevId) return;
    const handle = window.setTimeout(() => {
      if (nextId) {
        void prefetchImageDetail(queryClient, nextId, user).then(() => {
          const next = queryClient.getQueryData<ImageDto>(imageDetailKey(nextId, user?.id));
          if (next) preloadDetailImage(next, isViewerOpen);
        });
      }
      if (prevId) void prefetchImageDetail(queryClient, prevId, user);
    }, 250);
    return () => window.clearTimeout(handle);
  }, [nextId, prevId, queryClient, user, isViewerOpen]);

  const nearEnd = !!nav && nav.moreNext && nav.index >= (browse?.ids.length ?? 0) - 3;
  const nearStart = !!nav && nav.morePrev && nav.index <= 2;
  useEffect(() => {
    if (nearEnd) void extendBrowseContext('next');
    if (nearStart) void extendBrowseContext('prev');
  }, [nearEnd, nearStart]);

  useEffect(() => {
    if (wantsSlideshow) navigate(`${location.pathname}${location.search}`, { replace: true, state: null });
  }, [wantsSlideshow, navigate, location.pathname, location.search]);

  const openViewer = (play: boolean) => {
    setAutoPlay(play);
    setIsViewerOpen(true);
  };

  const goTo = (target?: number) => {
    if (!target) return false;
    navigate(`/images/${target}`, { replace: true });
  };

  useHotkeys({
    ArrowLeft: () => goTo(prevId),
    ArrowRight: () => goTo(nextId),
    f: () => {
      if (!image || needsConsent) return false;
      openViewer(false);
    },
    s: () => {
      if (!image || needsConsent || !nav) return false;
      openViewer(true);
    },
    l: () => {
      if (!image || needsConsent) return false;
      void handleLike();
    },
    a: () => {
      if (!image || needsConsent) return false;
      if (checkAuth('You must be logged in to add images to albums.')) setIsAlbumModalOpen(true);
    },
    d: () => {
      if (!image || needsConsent) return false;
      void handleDownload();
    },
    c: () => {
      if (!image) return false;
      void copy('link', pageUrl, 'Link copied to clipboard');
    },
  });

  const flashCopied = (key: string) => {
    setCopiedKey(key);
    window.setTimeout(() => setCopiedKey((current) => (current === key ? null : current)), 2000);
  };

  const copy = async (key: string, text: string, message: string) => {
    if (await copyText(text)) {
      flashCopied(key);
      showNotification('success', message);
    } else {
      showNotification('error', 'Could not copy to the clipboard');
    }
  };

  const handleCopyImage = async () => {
    const url = image ? safeExternalUrl(image.url) : undefined;
    if (!url) return;
    const png = (async () => {
      const res = await fetch(url, { mode: 'cors', credentials: 'omit' });
      if (!res.ok) throw new Error('fetch failed');
      const blob = await res.blob();
      if (blob.type === 'image/png') return blob;
      const bitmap = await createImageBitmap(blob);
      const canvas = document.createElement('canvas');
      canvas.width = bitmap.width;
      canvas.height = bitmap.height;
      canvas.getContext('2d')?.drawImage(bitmap, 0, 0);
      bitmap.close();
      return new Promise<Blob>((resolve, reject) => canvas.toBlob((out) => (out ? resolve(out) : reject(new Error('encode failed'))), 'image/png'));
    })();
    try {
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': png })]);
      flashCopied('image');
      showNotification('success', 'Image copied to clipboard');
    } catch {
      showNotification('error', 'Could not copy the image');
    }
  };

  const handleNativeShare = async () => {
    if (!image) return;
    try {
      await navigator.share({ title: `Image #${image.id}`, url: pageUrl });
    } catch {
    }
  };

  const handleDownload = async () => {
    if (!image || isDownloading) return;
    setIsDownloading(true);
    const url = safeExternalUrl(image.url);
    if (!url) {
      setIsDownloading(false);
      return;
    }
    try {
      const res = await fetch(url, { mode: 'cors', credentials: 'omit' });
      if (!res.ok) throw new Error('download failed');
      const blob = await res.blob();
      const objectUrl = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = objectUrl;
      const extension = /^\.[a-z0-9]{1,5}$/i.test(image.extension) ? image.extension : '.png';
      anchor.download = `${image.id}${extension}`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 30_000);
      showNotification('success', 'Download started');
    } catch {
      window.open(url, '_blank', 'noopener,noreferrer');
    } finally {
      setIsDownloading(false);
    }
  };

  const setImage = (updated: ImageDto) => {
    queryClient.setQueryData(imageDetailKey(id, user?.id), updated);
    patchCachedImage(queryClient, updated.id, {
      likedAt: updated.likedAt,
      favorites: updated.favorites,
      albums: updated.albums,
    });
  };

  const refreshSilently = () => queryClient.invalidateQueries({ queryKey: imageDetailKey(id, user?.id) });

  const handleDelete = async () => {
    try {
      await api.delete(`/images/${id}`);
      removeRecentImage(numericId);
      void queryClient.invalidateQueries({ queryKey: ['images'] });
      showNotification('success', 'Image deleted successfully');
      navigate('/gallery');
    } catch {
      return;
    }
  };

  const handleLike = async () => {
    if (!checkAuth('You must be logged in to like images.')) return;
    if (!image || isLiking) return;
    setIsLiking(true);
    const liked = !!image.likedAt;
    if (!liked) sfx('DOKI!', document.querySelector('[data-sfx="like"]'));
    const patch = liked
      ? { likedAt: undefined, favorites: image.favorites - 1, albums: (image.albums ?? []).filter((a) => !a.isDefault) }
      : { likedAt: nowIso(), favorites: image.favorites + 1 };
    try {
      if (liked) {
        await api.delete(`/users/${user!.id}/albums/favorites/images/${image.id}`, { skipGlobalErrorHandler: true });
      } else {
        await api.post(`/users/${user!.id}/albums/favorites/images/${image.id}`, {}, { skipGlobalErrorHandler: true });
      }
      patchCachedImage(queryClient, image.id, patch);
      void refreshSilently();
    } catch {
      showNotification('error', 'Could not update your favorites. Please try again.');
    } finally {
      setIsLiking(false);
    }
  };

  const handleSaveEdit = async (data: ImageFormData) => {
    if (!isAdminOrModerator || !image) return;
    const updatedImage = await updateImage(image.id, data);
    queryClient.setQueryData(imageDetailKey(id, user?.id), updatedImage);
    void queryClient.invalidateQueries({ queryKey: ['images'] });
    setShowEditModal(false);
    showNotification('success', 'Image updated successfully');
    void refreshSilently();
  };

  const handleReport = async (description: string) => {
    if (!image) return;
    try {
      await api.post('/reports', { imageId: image.id, description });
      showNotification('success', 'Report submitted');
      setIsReportModalOpen(false);
      invalidateNavBadges(queryClient);
    } catch {
      return;
    }
  };

  const consent = () => {
    grantConsent();
    setConsented(true);
  };

  const statusInfo: Record<ReviewStatus, { label: string; tone: StatusTone }> = {
      [ReviewStatus.Pending]: { label: 'Pending review', tone: 'pending' },
      [ReviewStatus.Accepted]: { label: 'Accepted', tone: 'success' },
      [ReviewStatus.Rejected]: { label: 'Rejected', tone: 'danger' },
  };

  if (notFound) return <NotFound />;
  if (!image && detail.isPending) {
    return (
      <div className="page" role="status" aria-live="polite">
        <span className="sr-only">Loading…</span>
        <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_23rem] xl:grid-cols-[minmax(0,1fr)_25rem] xl:gap-12" aria-hidden="true">
          <div className="sleeve aspect-[4/3] animate-pulse" />
          <div className="space-y-4">
            <div className="h-10 w-44 animate-pulse rounded-full bg-sleeve" />
            <div className="h-11 w-full animate-pulse rounded-full bg-sleeve" />
            <div className="h-52 w-full animate-pulse rounded-[var(--radius-panel)] bg-sleeve" />
          </div>
        </div>
      </div>
    );
  }
  if (!image) {
    return (
      <div className="page flex min-h-[50vh] flex-col items-center justify-center text-center" role="alert">
        <p className="font-bold">Failed to load image.</p>
        <p className="mt-1 text-sm text-muted-foreground">The server did not respond. Try again in a moment.</p>
        <button type="button" onClick={() => void detail.refetch()} className="btn btn-secondary mt-5">
          Try again
        </button>
      </div>
    );
  }

  const edit = pendingEdits?.get(image.id);
  const removedTagIds = new Set((edit?.removeTags ?? []).map(t => t.id));
  const removedArtistIds = new Set((edit?.removeArtists ?? []).map(a => a.id));
  const addedTags = edit?.addTags ?? [];
  const addedArtists = edit?.addArtists ?? [];

  const tagNames = image.tags.map(t => t.name).join(', ');
  const artistNames = image.artists.map(a => a.name).join(', ');
  const metaDescription = [
    artistNames && `By ${artistNames}`,
    tagNames && `Tags: ${tagNames}`,
    `${image.width}x${image.height}`,
  ].filter(Boolean).join(' | ');
  const tint = safeColor(image.dominantColor);
  const aspect = image.height > 0 ? Math.round((image.width / image.height) * 10000) / 10000 : 1;
  const status = statusInfo[image.reviewStatus] ?? { label: 'Unknown', tone: 'neutral' as StatusTone };
  const altText = artistNames ? `Image #${image.id} by ${artistNames}` : `Image #${image.id}`;
  const previewSrc = imgPreview(image.url);
  const lowResSrc = imgCard(image.url);
  const showLowRes = lowResSrc !== previewSrc && loadedSrc !== previewSrc;
  const isBroken = brokenSrc === previewSrc;
  const format = image.extension.replace(/^\./, '').toUpperCase() || '—';
  const canShare = typeof navigator.share === 'function';
  const canCopyImage = typeof ClipboardItem !== 'undefined' && typeof navigator.clipboard?.write === 'function' && !image.isAnimated;
  const position = nav ? `${nav.position.toLocaleString()} of ${nav.total.toLocaleString()}` : undefined;

  const entityChip = (opts: {
    key: string; name: string; entityHref: string; galleryHref: string; galleryTitle: string;
    removed?: boolean; pending?: boolean; added?: boolean; addedPending?: boolean; title?: string;
  }) => {
    const tone = opts.added
      ? 'bg-[color-mix(in_oklab,var(--success)_14%,var(--card))] text-success'
      : opts.removed
        ? 'bg-[color-mix(in_oklab,var(--destructive)_12%,var(--card))] text-destructive'
        : opts.pending
          ? 'text-warning outline-1 outline-dashed -outline-offset-1 outline-warning/60'
          : '';
    return (
      <span key={opts.key} title={opts.title} className={`sticker items-stretch gap-0 overflow-hidden p-0 text-sm ${tone}`}>
        <Link to={opts.entityHref} className={`flex min-w-0 items-center gap-1 py-1.5 pl-3.5 pr-2.5 transition-colors hover:bg-foreground/6 ${opts.removed ? 'line-through decoration-destructive/60' : ''}`}>
          {opts.added && <Plus size={13} className="shrink-0" aria-hidden="true" />}
          {opts.pending && !opts.added && <Clock size={12} className="shrink-0" aria-hidden="true" />}
          <span className="truncate">{opts.name}</span>
          {opts.addedPending && <Clock size={12} className="shrink-0" aria-hidden="true" />}
        </Link>
        <Link to={opts.galleryHref} title={opts.galleryTitle} aria-label={`${opts.galleryTitle}: ${opts.name}`} className="flex items-center border-l-2 border-dashed border-foreground/10 px-2.5 opacity-70 transition-[background-color,opacity] hover:bg-foreground/6 hover:opacity-100">
          <Image size={14} aria-hidden="true" />
        </Link>
      </span>
    );
  };

  const secondaryAction = 'btn btn-secondary btn-icon';
  const stepButton = 'grid size-9 place-items-center rounded-full text-foreground transition-colors hover:bg-secondary disabled:pointer-events-none disabled:opacity-35';

  return (
      <div className="page">
        <MetaTags
          title={`Image #${image.id}`}
          description={metaDescription}
          image={image.url}
          imageWidth={image.width}
          imageHeight={image.height}
          type="article"
          isNsfw={image.isNsfw}
        />

        {nav && (
          <nav aria-label="Browse the list" className="-mt-2 mb-5 flex flex-wrap items-center justify-between gap-3">
            <Link to={browse!.from} className="btn btn-ghost btn-sm -ml-3">
              <ArrowLeft size={15} aria-hidden="true" /> Back to the list
            </Link>
            <div className="flex items-center gap-1 rounded-full bg-card p-1 shadow-card dark:ring-1 dark:ring-white/6">
              <button type="button" onClick={() => openViewer(true)} disabled={needsConsent} className={stepButton} aria-label="Play a slideshow (S)" title="Slideshow (S)">
                <Play size={16} aria-hidden="true" />
              </button>
              <span className="h-5 w-px bg-border" aria-hidden="true" />
              <button type="button" onClick={() => goTo(prevId)} disabled={!prevId} className={stepButton} aria-label="Previous image (←)" title="Previous image (←)">
                <ChevronLeft size={17} aria-hidden="true" />
              </button>
              <span className="px-2 text-sm font-bold text-muted-foreground tabular">{position}</span>
              <button type="button" onClick={() => goTo(nextId)} disabled={!nextId} className={stepButton} aria-label="Next image (→)" title="Next image (→)">
                <ChevronRight size={17} aria-hidden="true" />
              </button>
            </div>
          </nav>
        )}

        <div className="grid grid-cols-1 items-start gap-8 lg:grid-cols-[minmax(0,1fr)_23rem] xl:grid-cols-[minmax(0,1fr)_25rem] xl:gap-12">
          <div
            className="sleeve relative flex min-h-72 items-center justify-center overflow-hidden p-3 sm:p-6 lg:p-8"
            style={tint ? { backgroundColor: `color-mix(in oklab, ${tint} 22%, var(--sleeve))` } : undefined}
          >
            <div
              onPointerMove={holo.onPointerMove}
              onPointerLeave={holo.onPointerLeave}
              className="holo-card group relative max-w-full rounded-[1.4rem] bg-card p-2 shadow-lift sm:p-2.5 dark:ring-1 dark:ring-white/8"
              style={{ width: `min(100%, calc(min(76vh, 62rem) * ${aspect} + 1.25rem))` }}
            >
              <div
                className="relative overflow-hidden rounded-[1rem]"
                style={{ aspectRatio: `${image.width} / ${image.height}`, backgroundColor: tint ?? undefined } as CSSProperties}
              >
                {needsConsent ? null : isBroken ? (
                  <span className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-sleeve text-sm font-bold text-muted-foreground">
                    <ImageOff size={24} aria-hidden="true" /> Image unavailable
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() => openViewer(false)}
                    className="absolute inset-0 block size-full cursor-zoom-in outline-none"
                    aria-label="Open full screen (F)"
                    title="Open full screen (F)"
                  >
                    {showLowRes && (
                      <img src={lowResSrc} alt="" aria-hidden="true" decoding="async" className="absolute inset-0 size-full scale-105 object-cover blur-md" />
                    )}
                    <img
                        key={previewSrc}
                        src={previewSrc}
                        srcSet={imgPreviewSrcSet(image.url, image.width)}
                        sizes={DETAIL_SIZES}
                        alt={altText}
                        width={image.width}
                        height={image.height}
                        decoding="async"
                        fetchPriority="high"
                        onLoad={() => setLoadedSrc(previewSrc)}
                        onError={() => setBrokenSrc(previewSrc)}
                        className="relative block size-full object-cover"
                    />
                  </button>
                )}
              </div>
              <span className="holo-foil" aria-hidden="true" />
              {!needsConsent && !isBroken && (
                <span className="pointer-events-none absolute bottom-4 right-4 grid size-9 place-items-center rounded-full bg-zinc-950/55 text-white opacity-0 backdrop-blur-sm transition-opacity group-hover:opacity-100 group-focus-within:opacity-100" aria-hidden="true">
                  <Maximize2 size={16} />
                </span>
              )}
            </div>

            {needsConsent && (
                <NsfwWarning
                    variant="overlay"
                    onConsent={consent}
                    onDecline={() => window.history.length > 1 ? navigate(-1) : navigate('/gallery')}
                />
            )}
          </div>

          <div className="space-y-7 lg:sticky lg:top-6">
            <div>
              <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                <h1 className="text-4xl font-black tabular">Image #{image.id}</h1>
                {image.reviewStatus !== ReviewStatus.Accepted && <StatusBadge label={status.label} tone={status.tone} />}
              </div>
              {artistNames && <p className="mt-2 text-muted-foreground">by <span className="font-bold text-foreground">{artistNames}</span></p>}
            </div>

            <div className="flex gap-2">
              <button
                  type="button"
                  onClick={handleLike}
                  data-sfx="like"
                  disabled={isLiking}
                  aria-pressed={!!image.likedAt}
                  aria-label={`${image.likedAt ? 'Remove from favorites' : 'Add to favorites'} (${image.favorites} favorites)`}
                  className={`btn ${image.likedAt ? 'bg-brand-soft text-brand-ink ring-1 ring-brand/30 hover:bg-brand-soft' : 'btn-secondary'}`}
              >
                <Heart size={17} aria-hidden="true" className={image.likedAt ? 'fill-current' : ''} />
                <span aria-hidden="true" className="tabular">{image.favorites}</span>
              </button>

              <button
                  type="button"
                  onClick={() => {
                      if (!checkAuth('You must be logged in to add images to albums.')) return;
                      setIsAlbumModalOpen(true);
                  }}
                  aria-label="Add to Album"
                  className="btn btn-primary min-w-0 flex-1 px-4"
              >
                  <FolderPlus size={17} className="shrink-0" aria-hidden="true" />
                  <span className="truncate" aria-hidden="true"><span className="min-[400px]:hidden">Album</span><span className="hidden min-[400px]:inline">Add to Album</span></span>
              </button>

              <button
                  type="button"
                  onClick={handleDownload}
                  disabled={isDownloading}
                  aria-label="Download the original image"
                  title="Download the original image"
                  className={secondaryAction}
              >
                  <Download size={17} className={isDownloading ? 'animate-pulse' : ''} aria-hidden="true" />
              </button>

              <Dropdown
                  width="w-60"
                  label="Share this image"
                  trigger={
                      <DropdownTrigger className={secondaryAction}>
                          {copiedKey ? <Check size={17} className="text-success" aria-hidden="true" /> : <Share2 size={17} aria-hidden="true" />}
                      </DropdownTrigger>
                  }
              >
                  <DropdownItem onClick={() => copy('link', pageUrl, 'Link copied to clipboard')} icon={<Link2 size={16} />}>Copy page link</DropdownItem>
                  <DropdownItem onClick={() => copy('url', image.url, 'Image URL copied to clipboard')} icon={<Image size={16} />}>Copy image URL</DropdownItem>
                  {canCopyImage && (
                      <DropdownItem onClick={handleCopyImage} icon={<Copy size={16} />}>Copy image</DropdownItem>
                  )}
                  <DropdownSeparator />
                  <DropdownItem onClick={() => copy('md', `[![${altText}](${image.url})](${pageUrl})`, 'Markdown copied to clipboard')} icon={<Code2 size={16} />}>Copy as Markdown</DropdownItem>
                  <DropdownItem onClick={() => copy('bb', `[url=${pageUrl}][img]${image.url}[/img][/url]`, 'BBCode copied to clipboard')} icon={<MessageSquareCode size={16} />}>Copy as BBCode</DropdownItem>
                  {canShare && (
                      <>
                          <DropdownSeparator />
                          <DropdownItem onClick={handleNativeShare} icon={<Send size={16} />}>Share…</DropdownItem>
                      </>
                  )}
              </Dropdown>

              <button
                  type="button"
                  onClick={() => setIsApiOpen(true)}
                  aria-label="Use this image in the API"
                  title="Use this image in the API"
                  className={secondaryAction}
              >
                  <Braces size={17} aria-hidden="true" />
              </button>
            </div>

            <div className="panel px-5 text-sm">
              <dl className="grid grid-cols-2 gap-4 pb-4 pt-5">
                <div className="flex flex-col-reverse gap-0.5">
                  <dt className="text-muted-foreground">Resolution</dt>
                  <dd className="text-2xl font-black tabular">{image.width} × {image.height}</dd>
                </div>
                <div className="flex flex-col-reverse gap-0.5">
                  <dt className="text-muted-foreground">Favorites</dt>
                  <dd className="flex items-center gap-1.5 text-2xl font-black tabular">
                    <Heart size={19} aria-hidden="true" className="fill-brand text-brand" />{image.favorites}
                  </dd>
                </div>
              </dl>
              <dl className="divide-y-2 divide-dashed divide-border border-t-2 border-dashed border-border">
                <StatRow label="File">{format} · {formatBytes(image.byteSize)}</StatRow>
                <StatRow label="Uploaded">{new Date(image.uploadedAt).toLocaleDateString()}</StatRow>
                {tint && (
                  <StatRow label="Dominant color">
                    <button
                      type="button"
                      onClick={() => copy('color', tint, `${tint} copied to clipboard`)}
                      title="Copy the color"
                      className="inline-flex items-center gap-2 rounded-full px-1.5 py-0.5 font-mono transition-colors hover:bg-secondary"
                    >
                      <span className="size-3.5 rounded-full ring-1 ring-border" style={{ backgroundColor: tint }} aria-hidden="true" />
                      {tint}
                    </button>
                    {normalizeHex(tint) && (
                      <Link
                        to={`/gallery?${new URLSearchParams({ color: normalizeHex(tint)!, ...(image.isNsfw && consented ? { isNsfw: NsfwMode.All } : {}) })}`}
                        title="Find images with a similar color"
                        aria-label="Find images with a similar color"
                        className="ml-1 inline-grid size-7 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                      >
                        <Palette size={15} aria-hidden="true" />
                      </Link>
                    )}
                  </StatRow>
                )}
                {!needsConsent && <ColorPalette url={image.url} allowNsfw={image.isNsfw && consented} />}
                <StatRow label="Status"><StatusBadge label={status.label} tone={status.tone} /></StatRow>
                {image.source && (
                  <StatRow label="Source">
                    <ExternalLink
                        href={image.source}
                        title={image.source}
                        className="block truncate text-brand-ink underline-offset-2 hover:underline"
                        fallback={<span className="break-all">{image.source}</span>}
                    >
                      {displayUrl(image.source)}
                    </ExternalLink>
                  </StatRow>
                )}
              </dl>
            </div>

            <section aria-labelledby="image-artists">
              <h2 id="image-artists" className="mb-2.5 text-sm font-bold">
                Artist{image.artists.length + addedArtists.length === 1 ? '' : 's'}
              </h2>
              <div className="flex flex-wrap items-center gap-2">
                {image.artists.map(artist => {
                    const removed = removedArtistIds.has(artist.id);
                    const pending = artist.reviewStatus === ReviewStatus.Pending;
                    return entityChip({
                      key: `artist-${artist.id}`,
                      name: artist.name,
                      entityHref: `/artists/${artist.id}`,
                      galleryHref: `/gallery?includedArtists=${artist.id}`,
                      galleryTitle: 'View images by this artist',
                      removed, pending,
                      title: removed ? 'You proposed removing this artist (awaiting review)' : pending ? 'Your artist (awaiting review)' : undefined,
                    });
                })}
                {addedArtists.map(a => entityChip({
                    key: `add-artist-${a.id}`,
                    name: a.name,
                    entityHref: `/artists/${a.id}`,
                    galleryHref: `/gallery?includedArtists=${a.id}`,
                    galleryTitle: 'View images by this artist',
                    added: true,
                    addedPending: a.reviewStatus === ReviewStatus.Pending,
                    title: a.reviewStatus === ReviewStatus.Pending
                        ? 'You proposed adding this artist (also awaiting review)'
                        : 'You proposed adding this artist (awaiting review)',
                }))}
                {image.artists.length + addedArtists.length === 0 && <p className="text-sm text-muted-foreground">No artist credited yet.</p>}
              </div>
            </section>

            <section aria-labelledby="image-tags">
              <h2 id="image-tags" className="mb-2.5 text-sm font-bold">Tags</h2>
              <div className="flex flex-wrap items-center gap-2">
                {image.tags.map(tag => {
                    const removed = removedTagIds.has(tag.id);
                    const pending = tag.reviewStatus === ReviewStatus.Pending;
                    return entityChip({
                      key: `tag-${tag.id}`,
                      name: tag.name,
                      entityHref: `/tags/${encodeURIComponent(tag.slug)}`,
                      galleryHref: `/gallery?includedTags=${encodeURIComponent(tag.slug)}`,
                      galleryTitle: 'View images with this tag',
                      removed, pending,
                      title: removed ? 'You proposed removing this tag (awaiting review)' : pending ? 'Your tag (awaiting review)' : undefined,
                    });
                })}
                {addedTags.map(t => entityChip({
                    key: `add-tag-${t.id}`,
                    name: t.name,
                    entityHref: `/tags/${encodeURIComponent(t.slug)}`,
                    galleryHref: `/gallery?includedTags=${encodeURIComponent(t.slug)}`,
                    galleryTitle: 'View images with this tag',
                    added: true,
                    addedPending: t.reviewStatus === ReviewStatus.Pending,
                    title: t.reviewStatus === ReviewStatus.Pending
                        ? 'You proposed adding this tag (also awaiting review)'
                        : 'You proposed adding this tag (awaiting review)',
                }))}
                {image.tags.length + addedTags.length === 0 && <p className="text-sm text-muted-foreground">No tags yet.</p>}
              </div>
            </section>

            <div className="flex flex-wrap gap-1 border-t-2 border-dashed border-border pt-5">
              <button
                  type="button"
                  onClick={() => {
                      if (!checkAuth('You must be logged in to report images.')) return;
                      setIsReportModalOpen(true);
                  }}
                  className="btn btn-ghost btn-sm"
                  aria-label="Report this image"
              >
                  <Flag size={15} aria-hidden="true" /> Report
              </button>

              {user && !isAdminOrModerator && (
                  <button type="button" onClick={() => setShowSuggestModal(true)} className="btn btn-ghost btn-sm" aria-label="Suggest an edit">
                      <GitPullRequest size={15} aria-hidden="true" /> Suggest edit
                  </button>
              )}

              {isAdminOrModerator && (
                  <>
                    <button type="button" onClick={() => setShowEditModal(true)} className="btn btn-ghost btn-sm" aria-label="Edit image">
                      <Edit size={15} aria-hidden="true" /> Edit
                    </button>
                    <button type="button" onClick={() => setIsDeleteModalOpen(true)} className="btn btn-danger-soft btn-sm" aria-label="Delete image">
                      <Trash2 size={15} aria-hidden="true" /> Delete
                    </button>
                  </>
              )}
            </div>
          </div>
        </div>

        {!needsConsent && <RelatedImages image={image} allowNsfw={consented} />}
        {!needsConsent && <SimilarImages image={image} allowNsfw={consented} />}

        {isViewerOpen && !needsConsent && (
            <Lightbox
                url={image.url}
                alt={altText}
                width={image.width}
                height={image.height}
                placeholder={tint ?? undefined}
                position={position}
                autoPlay={autoPlay}
                canContinue={!!nav?.moreNext}
                onClose={() => setIsViewerOpen(false)}
                onPrev={prevId ? () => goTo(prevId) : undefined}
                onNext={nextId ? () => goTo(nextId) : undefined}
            />
        )}

        <ApiRequestDialog
            isOpen={isApiOpen}
            onClose={() => setIsApiOpen(false)}
            path={`/images/${image.id}`}
            description="Fetch this image's record – URL, size, artists and tags – from the API."
        />

        <ConfirmModal
            isOpen={isDeleteModalOpen}
            onClose={() => setIsDeleteModalOpen(false)}
            onConfirm={handleDelete}
            title="Delete Image"
            message="Permanently delete this image? This cannot be undone."
            confirmText="Delete"
            variant="destructive"
            zIndex={70}
        />

        <ImageModal
            isOpen={showEditModal}
            onClose={() => setShowEditModal(false)}
            initialData={image}
            onSubmit={handleSaveEdit}
            onDelete={() => setIsDeleteModalOpen(true)}
        />

        {showSuggestModal && (
            <ImageModal
                isOpen={showSuggestModal}
                onClose={() => {
                    setShowSuggestModal(false);
                    queryClient.invalidateQueries({ queryKey: ['my-pending-image-edits'] });
                    invalidateNavBadges(queryClient);
                }}
                initialData={image}
                onSubmit={() => {}}
                suggestContext={{ targetType: ReviewableContentType.Image, targetId: image.id, current: image }}
            />
        )}

        <ReasonModal
            isOpen={isReportModalOpen}
            onClose={() => setIsReportModalOpen(false)}
            onSubmit={handleReport}
            title="Report Image"
            description="Please describe why you are reporting this image."
            placeholder="Reason for reporting..."
            submitText="Submit Report"
        />

        <AlbumSelectionModal
            isOpen={isAlbumModalOpen}
            onClose={() => setIsAlbumModalOpen(false)}
            image={image}
            onUpdate={setImage}
        />
      </div>
  );
};

export default ImagePage;
