import { useState } from 'react';
import { Link } from 'react-router';
import { Artist, Role, ReviewableContentType, ReviewStatus } from '../types';
import { useAuth } from '../context/AuthContext';
import { Plus, Edit2, Trash2, Info, Link as LinkIcon, Image, GitPullRequest, Clock } from 'lucide-react';
import ArtistModal, { ArtistFormData } from '../components/modals/ArtistModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import Modal from '../components/Modal';
import SearchInput from '../components/SearchInput';
import Pagination from '../components/Pagination';
import { useResource } from '../hooks/useResource';
import { useAuthGuard } from '../hooks/useRequireAuth';
import { MetaTags } from '../hooks/useMetaTags';
import SafeExternalLink from '../components/ExternalLink';
import { displayUrl } from '../utils/safeUrl';

const ARTIST_LINKS = [
    { key: 'twitter', label: 'Twitter' },
    { key: 'pixiv', label: 'Pixiv' },
    { key: 'deviantArt', label: 'DeviantArt' },
    { key: 'patreon', label: 'Patreon' },
] as const;

const Artists = () => {
    const { user } = useAuth();
    const checkAuth = useAuthGuard();

    const {
        items: artists,
        loading,
        page,
        setPage,
        totalPages,
        search,
        setSearch,
        searchType,
        setSearchType,
        createItem,
        updateItem,
        deleteItem
    } = useResource<Artist>('/artists');

    const [isCreateOpen, setIsCreateOpen] = useState(false);
    const [isEditOpen, setIsEditOpen] = useState(false);
    const [isDeleteOpen, setIsDeleteOpen] = useState(false);
    const [suggestArtist, setSuggestArtist] = useState<Artist | null>(null);
    const [selectedArtist, setSelectedArtist] = useState<Artist | null>(null);
    const [infoArtist, setInfoArtist] = useState<Artist | null>(null);
    const [formData, setFormData] = useState<ArtistFormData>({ name: '', twitter: '', pixiv: '', patreon: '', deviantArt: '' });

    const handleOpenCreate = () => {
        if (!checkAuth('Log in to create artists.')) return;
        setFormData({ name: '', twitter: '', pixiv: '', patreon: '', deviantArt: '' });
        setIsCreateOpen(true);
    };

    const handleCreate = async (data: ArtistFormData) => {
        const isMod = user?.role === Role.Moderator || user?.role === Role.Admin;
        const success = await createItem(data, isMod ? 'Artist created' : 'Artist submitted for review');
        if (success) {
            setIsCreateOpen(false);
            setFormData({ name: '', twitter: '', pixiv: '', patreon: '', deviantArt: '' });
        }
    };

    const handleEdit = async (data: ArtistFormData) => {
        if (!selectedArtist) return;
        const success = await updateItem(selectedArtist.id, data, 'Artist updated');
        if (success) setIsEditOpen(false);
    };

    const handleDelete = async () => {
        if (!selectedArtist) return;
        const success = await deleteItem(selectedArtist.id, 'Artist deleted');
        if (success) setIsDeleteOpen(false);
    };

    const openEdit = (e: React.MouseEvent, artist: Artist) => {
        e.preventDefault();
        e.stopPropagation();
        setSelectedArtist(artist);
        setFormData({
            name: artist.name,
            twitter: artist.twitter,
            pixiv: artist.pixiv,
            patreon: artist.patreon,
            deviantArt: artist.deviantArt,
            reviewStatus: artist.reviewStatus,
            creatorId: artist.creatorId
        });
        setIsEditOpen(true);
    };

    const openDelete = (e: React.MouseEvent, artist: Artist) => {
        e.preventDefault();
        e.stopPropagation();
        setSelectedArtist(artist);
        setIsDeleteOpen(true);
    };

    const canManage = user && (user.role === Role.Admin || user.role === Role.Moderator);
    const isAdmin = user?.role === Role.Admin;
    const isReviewMode = !(canManage);

    const iconButton = 'relative z-10 grid size-8 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground';

    return (
        <div className="page">
            <MetaTags
              title="Artists"
              description="Browse and manage artists."
            />
            <div className="page-header">
                <div>
                    <h1 className="page-title">Artists</h1>
                    <p className="page-lede">The people behind the illustrations, with links to their pages.</p>
                </div>

                <div className="flex w-full gap-2 md:w-auto">
                    <SearchInput
                        value={search}
                        onChange={setSearch}
                        searchType={searchType}
                        onSearchTypeChange={setSearchType}
                    />
                    <button type="button" onClick={handleOpenCreate} aria-label="Create artist" className="btn btn-primary shrink-0">
                        <Plus size={18} aria-hidden="true" /> <span className="hidden sm:inline">New artist</span>
                    </button>
                </div>
            </div>

            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
                {loading ? [...Array(8)].map((_, i) => <div key={i} className="h-44 animate-pulse rounded-[var(--radius-panel)] bg-sleeve" />) : artists.length === 0 ? (
                    <p className="sleeve col-span-full px-6 py-14 text-center font-bold text-muted-foreground">
                        No artists match your search.
                    </p>
                ) : artists.map(artist => {
                    const links = ARTIST_LINKS.filter(({ key }) => !!artist[key]);
                    return (
                    <article key={artist.id} className="panel group relative flex flex-col p-5 transition-shadow duration-200 hover:shadow-lift has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-ring">
                        <div className="flex min-w-0 items-center gap-3">
                            <span className="card-back grid aspect-[5/7] w-10 shrink-0 place-items-center rounded-[0.7rem] text-lg font-black text-zinc-50 shadow-card" aria-hidden="true">
                                {[...artist.name.trim()][0]?.toUpperCase() ?? '?'}
                            </span>
                            <div className="min-w-0 flex-1">
                                <h2 className="text-lg font-black leading-snug">
                                    <Link
                                        to={`/gallery?includedArtists=${artist.id}`}
                                        className="block truncate outline-none after:absolute after:inset-0"
                                        title={`View images by ${artist.name}`}
                                    >
                                        {artist.name}
                                    </Link>
                                </h2>
                                <p className="flex items-center gap-2 text-xs text-muted-foreground">
                                    <span className="tabular">#{artist.id}</span>
                                    {artist.reviewStatus === ReviewStatus.Pending && (
                                        <span className="inline-flex items-center gap-1 rounded-full bg-warning/12 px-2 py-0.5 font-bold text-warning" title="Awaiting review">
                                            <Clock size={11} aria-hidden="true" /> Pending
                                        </span>
                                    )}
                                </p>
                            </div>
                        </div>

                        <div className="mt-4 flex flex-1 flex-wrap content-start gap-1.5">
                            {links.length === 0 && <span className="text-sm text-muted-foreground">No links yet</span>}
                            {links.map(({ key, label }) => {
                                const url = artist[key] ?? '';
                                return (
                                    <SafeExternalLink
                                        key={key}
                                        href={url}
                                        title={url}
                                        aria-label={`${label}: ${displayUrl(url)} (opens in a new tab)`}
                                        className="sticker relative z-10 px-2.5 py-1 text-xs transition-colors hover:bg-brand-soft hover:text-brand-ink"
                                    >
                                        <LinkIcon size={12} aria-hidden="true" className="shrink-0" />
                                        {label}
                                    </SafeExternalLink>
                                );
                            })}
                        </div>

                        <div className="mt-4 flex items-center justify-between gap-2">
                            <span className="inline-flex items-center gap-1.5 text-sm font-bold">
                                <Image size={15} className="text-muted-foreground" aria-hidden="true" />
                                <span className="tabular">{artist.imageCount?.toLocaleString() ?? 0}</span>
                                <span className="font-normal text-muted-foreground">images</span>
                            </span>
                            <div className="flex shrink-0 gap-0.5">
                                <button type="button" onClick={() => setInfoArtist(artist)} className={iconButton} title="Details" aria-label={`Details for ${artist.name}`}>
                                    <Info size={16} aria-hidden="true" />
                                </button>
                                {canManage ? (
                                    <>
                                        <button type="button" onClick={(e) => openEdit(e, artist)} className={iconButton} title="Edit" aria-label={`Edit ${artist.name}`}>
                                            <Edit2 size={15} aria-hidden="true" />
                                        </button>
                                        {isAdmin && (
                                            <button type="button" onClick={(e) => openDelete(e, artist)} className={`${iconButton} hover:bg-destructive/10 hover:text-destructive`} title="Delete" aria-label={`Delete ${artist.name}`}>
                                                <Trash2 size={15} aria-hidden="true" />
                                            </button>
                                        )}
                                    </>
                                ) : user && (
                                    <button
                                        type="button"
                                        onClick={(e) => { e.preventDefault(); e.stopPropagation(); setSuggestArtist(artist); }}
                                        className={iconButton}
                                        title="Suggest edit"
                                        aria-label={`Suggest an edit for ${artist.name}`}
                                    >
                                        <GitPullRequest size={15} aria-hidden="true" />
                                    </button>
                                )}
                            </div>
                        </div>
                    </article>
                    );
                })}
            </div>

            {!loading && (
                <Pagination currentPage={page} totalPages={totalPages} setPage={setPage} />
            )}

            <ArtistModal isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} onSubmit={handleCreate} title="New Artist" isReviewMode={isReviewMode} submitLabel="Create" />

            <ArtistModal
                isOpen={isEditOpen}
                onClose={() => setIsEditOpen(false)}
                onSubmit={handleEdit}
                initialData={selectedArtist ? { ...formData, id: selectedArtist.id } : formData}
                title="Edit Artist"
                submitLabel="Save"
                onDelete={isAdmin ? () => setIsDeleteOpen(true) : undefined}
            />

            {suggestArtist && (
                <ArtistModal
                    isOpen={!!suggestArtist}
                    onClose={() => setSuggestArtist(null)}
                    onSubmit={() => {}}
                    initialData={{
                        name: suggestArtist.name,
                        twitter: suggestArtist.twitter,
                        pixiv: suggestArtist.pixiv,
                        patreon: suggestArtist.patreon,
                        deviantArt: suggestArtist.deviantArt,
                    }}
                    title="Suggest changes"
                    suggestContext={{ targetType: ReviewableContentType.Artist, targetId: suggestArtist.id, current: suggestArtist }}
                />
            )}

            <ConfirmModal isOpen={isDeleteOpen} onClose={() => setIsDeleteOpen(false)} onConfirm={handleDelete} title="Delete Artist" message={<p>Delete the artist <strong className="text-foreground">{selectedArtist?.name}</strong>? Their images stay in the gallery.</p>} zIndex={70} />

            <Modal
                isOpen={!!infoArtist}
                onClose={() => setInfoArtist(null)}
                title="Artist Details"
                footer={infoArtist && (
                    <div className="flex justify-end">
                        <Link to={`/gallery?includedArtists=${infoArtist.id}`} className="btn btn-primary">
                            View images
                        </Link>
                    </div>
                )}
            >
                {infoArtist && (
                    <dl className="space-y-4">
                        <div>
                            <dt className="text-sm text-muted-foreground">Name</dt>
                            <dd className="mt-0.5 text-lg font-semibold wrap-break-word">{infoArtist.name}</dd>
                        </div>
                        {ARTIST_LINKS.map(({ key, label }) => {
                            const url = infoArtist[key];
                            if (!url) return null;
                            return (
                                <div key={key}>
                                    <dt className="text-sm text-muted-foreground">{label}</dt>
                                    <dd className="mt-0.5">
                                        <SafeExternalLink
                                            href={url}
                                            className="font-medium text-brand-ink break-all hover:underline"
                                            fallback={<span className="break-all text-muted-foreground">{url}</span>}
                                        >
                                            {url}
                                        </SafeExternalLink>
                                    </dd>
                                </div>
                            );
                        })}
                    </dl>
                )}
            </Modal>
        </div>
    );
};

export default Artists;