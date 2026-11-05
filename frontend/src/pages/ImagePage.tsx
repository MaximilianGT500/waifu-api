import { useState, useEffect, useEffectEvent, useRef, ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useParams, useNavigate, Link } from 'react-router';
import api from '../services/api';
import { ImageDto, Role, ImageFormData, ReviewStatus, ReviewStatusFilter, ReviewableContentType } from '../types';
import { Heart, Trash2, Edit, FolderPlus, Flag, Image, GitPullRequest, Clock, Plus, Download, Link2, Check } from 'lucide-react';
import { useMyPendingImageEdits } from '../hooks/useMyPendingImageEdits';
import { invalidateNavBadges } from '../hooks/useNavBadges';
import { useAuth } from '../context/AuthContext';
import { useNotification } from '../context/NotificationContext';
import { useNsfwConsent } from '../hooks/useNsfwConsent';
import { useImageUpdate } from '../hooks/useImageUpdate';
import { useAuthGuard } from '../hooks/useRequireAuth';
import ImageModal from '../components/modals/ImageModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import ReasonModal from '../components/modals/ReasonModal';
import AlbumSelectionModal from '../components/modals/AlbumSelectionModal';
import NsfwWarning from '../components/NsfwWarning';
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

const StatRow = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex min-w-0 items-center justify-between gap-4 py-3">
    <dt className="shrink-0 text-muted-foreground">{label}</dt>
    <dd className="min-w-0 text-right font-bold tabular">{children}</dd>
  </div>
);

const ImagePage = () => {
  const { id } = useParams<{ id: string }>();
  if (!isId(id)) return <NotFound />;
  return <ImageView id={id} />;
};

const ImageView = ({ id }: { id: string }) => {
  const navigate = useNavigate();
  const { user } = useAuth();
  const { showNotification } = useNotification();
  const checkAuth = useAuthGuard();
  const queryClient = useQueryClient();
  const { data: pendingEdits } = useMyPendingImageEdits(!!user);

  const [image, setImage] = useState<ImageDto | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);

  const { showWarning: showNsfwWarning, setShowWarning: setShowNsfwWarning, grantConsent, hasConsent } = useNsfwConsent(false);
  const { updateImage } = useImageUpdate();

  const [showEditModal, setShowEditModal] = useState(false);
  const [showSuggestModal, setShowSuggestModal] = useState(false);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [isReportModalOpen, setIsReportModalOpen] = useState(false);
  const [isAlbumModalOpen, setIsAlbumModalOpen] = useState(false);
  const [linkCopied, setLinkCopied] = useState(false);
  const [isDownloading, setIsDownloading] = useState(false);
  const [isLiking, setIsLiking] = useState(false);
  const requestSeq = useRef(0);
  const holo = useHolo<HTMLDivElement>(4);

  const isAdminOrModerator = user && (user.role === Role.Admin || user.role === Role.Moderator);

  const handleCopyLink = async () => {
    try {
      await navigator.clipboard.writeText(window.location.href);
      setLinkCopied(true);
      showNotification('success', 'Link copied to clipboard');
      setTimeout(() => setLinkCopied(false), 2000);
    } catch {
      showNotification('error', 'Could not copy the link');
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

  const fetchImage = async (silent = false) => {
    const seq = ++requestSeq.current;
    try {
      if (!silent) { setIsLoading(true); setError(null); setNotFound(false); }
      const params: Record<string, string> = {};
      if (isAdminOrModerator) {
        params.childrenReviewStatus = ReviewStatusFilter.All;
      } else if (user) {
        params.includeMyPendingChildren = 'true';
      }
      const { data } = await api.get<ImageDto>(`/images/${id}`, { params, skipGlobalErrorHandler: true });
      if (seq !== requestSeq.current) return;
      setImage(data);

      if (data.isNsfw && !silent && !hasConsent() && !isBot()) {
        setShowNsfwWarning(true);
      }
    } catch (err) {
      if (seq !== requestSeq.current) return;
      const status = (err as { response?: { status?: number } })?.response?.status;
      if (status === 404 || status === 400) {
        if (!silent) setNotFound(true);
      } else {
        if (!silent) setError("Failed to load image.");
      }
    } finally {
      if (!silent && seq === requestSeq.current) setIsLoading(false);
    }
  };

  const loadImage = useEffectEvent(() => { fetchImage(); });

  useEffect(() => {
    loadImage();
  }, [id, user?.id]);

  const handleDelete = async () => {
    try {
      await api.delete(`/images/${id}`);
      showNotification('success', 'Image deleted successfully');
      navigate('/gallery');
    } catch {
    }
  };

  const handleLike = async () => {
    if (!checkAuth('You must be logged in to like images.')) return;
    if (!image || isLiking) return;
    setIsLiking(true);
    try {
      if (image.likedAt) {
        await api.delete(`/users/${user!.id}/albums/favorites/images/${image.id}`, { skipGlobalErrorHandler: true });
        setImage(prev => prev ? { ...prev, likedAt: undefined, favorites: prev.favorites - 1 } : null);
      } else {
        await api.post(`/users/${user!.id}/albums/favorites/images/${image.id}`, {}, { skipGlobalErrorHandler: true });
        setImage(prev => prev ? { ...prev, likedAt: nowIso(), favorites: prev.favorites + 1 } : null);
      }
      fetchImage(true);
    } catch {
      showNotification('error', 'Could not update your favorites. Please try again.');
    } finally {
      setIsLiking(false);
    }
  };

  const handleSaveEdit = async (data: ImageFormData) => {
    if (!isAdminOrModerator || !image) return;
    const updatedImage = await updateImage(image.id, data);
    setImage(updatedImage);
    setShowEditModal(false);
    showNotification('success', 'Image updated successfully');
    fetchImage(true);
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

  const statusInfo: Record<ReviewStatus, { label: string; tone: StatusTone }> = {
      [ReviewStatus.Pending]: { label: 'Pending review', tone: 'pending' },
      [ReviewStatus.Accepted]: { label: 'Accepted', tone: 'success' },
      [ReviewStatus.Rejected]: { label: 'Rejected', tone: 'danger' },
  };

  if (isLoading) {
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
  if (notFound) return <NotFound />;
  if (error || !image) {
    return (
      <div className="page flex min-h-[50vh] flex-col items-center justify-center text-center" role="alert">
        <p className="font-bold">{error || 'Not Found'}</p>
        <p className="mt-1 text-sm text-muted-foreground">The server did not respond. Try again in a moment.</p>
        <button type="button" onClick={() => fetchImage()} className="btn btn-secondary mt-5">
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
        <div className="grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_23rem] xl:grid-cols-[minmax(0,1fr)_25rem] xl:gap-12">
          <div
            className="sleeve relative flex min-h-72 items-center justify-center overflow-hidden p-3 sm:p-6 lg:p-8"
            style={tint ? { backgroundColor: `color-mix(in oklab, ${tint} 22%, var(--sleeve))` } : undefined}
          >
            <div
              onPointerMove={holo.onPointerMove}
              onPointerLeave={holo.onPointerLeave}
              className="holo-card relative max-w-full rounded-[1.4rem] bg-card p-2 shadow-lift sm:p-2.5 dark:ring-1 dark:ring-white/8"
              style={{ width: `min(100%, calc(min(76vh, 62rem) * ${aspect} + 1.25rem))` }}
            >
              <img
                  src={image.url}
                  alt={artistNames ? `Image #${image.id} by ${artistNames}` : `Image #${image.id}`}
                  width={image.width}
                  height={image.height}
                  decoding="async"
                  fetchPriority="high"
                  className="block h-auto w-full rounded-[1rem]"
              />
              <span className="holo-foil" aria-hidden="true" />
            </div>

            {image.isNsfw && showNsfwWarning && (
                <NsfwWarning
                    variant="overlay"
                    onConsent={grantConsent}
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

            <div className="flex flex-wrap gap-2">
              <button
                  type="button"
                  onClick={handleLike}
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
                  className="btn btn-primary flex-1"
              >
                  <FolderPlus size={17} className="shrink-0" aria-hidden="true" />
                  <span className="truncate">Add to Album</span>
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

              <button
                  type="button"
                  onClick={handleCopyLink}
                  aria-label={linkCopied ? 'Link copied' : 'Copy link to this image'}
                  title="Copy link to this image"
                  className={secondaryAction}
              >
                  {linkCopied ? <Check size={17} className="text-success" aria-hidden="true" /> : <Link2 size={17} aria-hidden="true" />}
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
                <StatRow label="File size">{(image.byteSize / (1024 * 1024)).toFixed(2)} MB</StatRow>
                <StatRow label="Uploaded">{new Date(image.uploadedAt).toLocaleDateString()}</StatRow>
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
                      entityHref: `/artists?includedIds=${artist.id}`,
                      galleryHref: `/gallery?includedArtists=${artist.id}`,
                      galleryTitle: 'View images by this artist',
                      removed, pending,
                      title: removed ? 'You proposed removing this artist (awaiting review)' : pending ? 'Your artist (awaiting review)' : undefined,
                    });
                })}
                {addedArtists.map(a => entityChip({
                    key: `add-artist-${a.id}`,
                    name: a.name,
                    entityHref: `/artists?includedIds=${a.id}`,
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
                      entityHref: `/tags?includedIds=${tag.id}`,
                      galleryHref: `/gallery?includedTags=${encodeURIComponent(tag.slug)}`,
                      galleryTitle: 'View images with this tag',
                      removed, pending,
                      title: removed ? 'You proposed removing this tag (awaiting review)' : pending ? 'Your tag (awaiting review)' : undefined,
                    });
                })}
                {addedTags.map(t => entityChip({
                    key: `add-tag-${t.id}`,
                    name: t.name,
                    entityHref: `/tags?includedIds=${t.id}`,
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
            onUpdate={(updated) => setImage(updated)}
        />
      </div>
  );
};

export default ImagePage;