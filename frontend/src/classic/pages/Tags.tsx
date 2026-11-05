import { useState } from 'react';
import { Link } from 'react-router';
import { Tag, Role, ReviewableContentType, ReviewStatus } from '../../types';
import { useAuth } from '../../context/AuthContext';
import Modal from '../components/Modal';
import TagModal, { TagFormData } from '../components/modals/TagModal';
import { Plus, Edit2, Trash2, Info, Image, GitPullRequest, Clock } from 'lucide-react';
import ConfirmModal from '../components/modals/ConfirmModal';
import SearchInput from '../components/SearchInput';
import Pagination from '../components/Pagination';
import { useResource } from '../../hooks/useResource';
import { useAuthGuard } from '../../hooks/useRequireAuth';
import { MetaTags } from '../../hooks/useMetaTags';

const Tags = () => {
    const { user } = useAuth();
    const checkAuth = useAuthGuard();

    const {
        items: tags,
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
    } = useResource<Tag>('/tags');

    const [isCreateOpen, setIsCreateOpen] = useState(false);
    const [isEditOpen, setIsEditOpen] = useState(false);
    const [isDeleteOpen, setIsDeleteOpen] = useState(false);
    const [suggestTag, setSuggestTag] = useState<Tag | null>(null);

    const [selectedTag, setSelectedTag] = useState<Tag | null>(null);
    const [infoTag, setInfoTag] = useState<Tag | null>(null);
    const [formData, setFormData] = useState<TagFormData>({ name: '', slug: '', description: '' });

    const handleOpenCreate = () => {
        if (!checkAuth('Log in to create tags.')) return;
        setFormData({ name: '', slug: '', description: '' });
        setIsCreateOpen(true);
    };

    const handleCreate = async (data: TagFormData) => {
        const success = await createItem(data, 'Tag created');
        if (success) {
            setIsCreateOpen(false);
            setFormData({ name: '', slug: '', description: '' });
        }
    };

    const handleEdit = async (data: TagFormData) => {
        if (!selectedTag) return;
        const success = await updateItem(selectedTag.id, data, 'Tag updated');
        if (success) setIsEditOpen(false);
    };

    const handleDelete = async () => {
        if (!selectedTag) return;
        const success = await deleteItem(selectedTag.id, 'Tag deleted');
        if (success) setIsDeleteOpen(false);
    };

    const openEdit = (e: React.MouseEvent, tag: Tag) => {
        e.preventDefault();
        e.stopPropagation();
        setSelectedTag(tag);
        setFormData({ name: tag.name, slug: tag.slug, description: tag.description, reviewStatus: tag.reviewStatus, creatorId: tag.creatorId });
        setIsEditOpen(true);
    };

    const openDelete = (e: React.MouseEvent, tag: Tag) => {
        e.preventDefault();
        e.stopPropagation();
        setSelectedTag(tag);
        setIsDeleteOpen(true);
    };

    const isAdmin = user?.role === Role.Admin;
    const isModerator = user?.role === Role.Moderator;
    const canEdit = isAdmin || isModerator;
    const isReviewMode = !(canEdit);

    const iconButton = 'relative z-10 grid size-8 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground';

    return (
        <div className="page">
            <MetaTags
              title="Tags"
              description="Browse and manage image tags."
            />
            <div className="page-header">
                <div>
                    <h1 className="page-title">Tags</h1>
                    <p className="page-lede">Every tag you can filter the gallery and the API by.</p>
                </div>

                <div className="flex w-full gap-2 md:w-auto">
                    <SearchInput
                        value={search}
                        onChange={setSearch}
                        searchType={searchType}
                        onSearchTypeChange={setSearchType}
                    />
                    <button type="button" onClick={handleOpenCreate} aria-label="Create tag" className="btn btn-primary shrink-0">
                        <Plus size={18} aria-hidden="true" /> <span className="hidden sm:inline">New tag</span>
                    </button>
                </div>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
                {loading ? (
                    [...Array(8)].map((_, i) => <div key={i} className="h-40 animate-pulse rounded-2xl bg-secondary" />)
                ) : tags.length === 0 ? (
                    <p className="col-span-full rounded-2xl border border-dashed border-border px-6 py-12 text-center text-muted-foreground">
                        No tags match your search.
                    </p>
                ) : (
                    tags.map(tag => (
                        <article key={tag.id} className="panel group relative flex flex-col p-5 transition-[border-color,box-shadow] hover:border-brand/40 hover:shadow-float">
                            <div className="flex min-w-0 items-start justify-between gap-2">
                                <h2 className="min-w-0 text-lg font-bold leading-snug">
                                    <Link
                                        to={`/gallery?includedTags=${encodeURIComponent(tag.slug)}`}
                                        className="block truncate outline-none after:absolute after:inset-0 after:rounded-2xl focus-visible:after:ring-2 focus-visible:after:ring-ring"
                                        title={`View images tagged ${tag.name}`}
                                    >
                                        {tag.name}
                                    </Link>
                                </h2>
                                {tag.reviewStatus === ReviewStatus.Pending && (
                                    <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-warning/12 px-2 py-0.5 text-xs font-semibold text-warning" title="Awaiting review">
                                        <Clock size={11} aria-hidden="true" /> Pending
                                    </span>
                                )}
                            </div>
                            <div className="mt-1 flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                                <code className="truncate font-mono text-brand-ink" title="API slug">{tag.slug}</code>
                                <span className="tabular">#{tag.id}</span>
                            </div>

                            <p className="mt-3 line-clamp-2 flex-1 text-sm text-muted-foreground">{tag.description || 'No description'}</p>

                            <div className="mt-4 flex items-center justify-between gap-2">
                                <span className="inline-flex items-center gap-1.5 text-sm font-semibold">
                                    <Image size={15} className="text-muted-foreground" aria-hidden="true" />
                                    <span className="tabular">{tag.imageCount?.toLocaleString() ?? 0}</span>
                                    <span className="font-normal text-muted-foreground">images</span>
                                </span>
                                <div className="flex shrink-0 gap-0.5">
                                    <button type="button" onClick={() => setInfoTag(tag)} className={iconButton} title="Details" aria-label={`Details for ${tag.name}`}>
                                        <Info size={16} aria-hidden="true" />
                                    </button>
                                    {canEdit ? (
                                        <>
                                            <button type="button" onClick={(e) => openEdit(e, tag)} className={iconButton} title="Edit" aria-label={`Edit ${tag.name}`}>
                                                <Edit2 size={15} aria-hidden="true" />
                                            </button>
                                            {isAdmin && (
                                                <button type="button" onClick={(e) => openDelete(e, tag)} className={`${iconButton} hover:bg-destructive/10 hover:text-destructive`} title="Delete" aria-label={`Delete ${tag.name}`}>
                                                    <Trash2 size={15} aria-hidden="true" />
                                                </button>
                                            )}
                                        </>
                                    ) : user && (
                                        <button
                                            type="button"
                                            onClick={(e) => { e.preventDefault(); e.stopPropagation(); setSuggestTag(tag); }}
                                            className={iconButton}
                                            title="Suggest edit"
                                            aria-label={`Suggest an edit for ${tag.name}`}
                                        >
                                            <GitPullRequest size={15} aria-hidden="true" />
                                        </button>
                                    )}
                                </div>
                            </div>
                        </article>
                    ))
                )}
            </div>

            {!loading && (
                <Pagination currentPage={page} totalPages={totalPages} setPage={setPage} />
            )}

            <TagModal
                isOpen={isCreateOpen}
                onClose={() => setIsCreateOpen(false)}
                onSubmit={handleCreate}
                initialData={{ name: '', slug: '', description: '' }}
                title="New Tag"
                isReviewMode={isReviewMode}
                submitLabel="Create"
            />

            <TagModal
                isOpen={isEditOpen}
                onClose={() => setIsEditOpen(false)}
                onSubmit={handleEdit}
                initialData={selectedTag ? { ...formData, id: selectedTag.id } : formData}
                title="Edit Tag"
                submitLabel="Save"
                onDelete={isAdmin ? () => setIsDeleteOpen(true) : undefined}
            />

            {suggestTag && (
                <TagModal
                    isOpen={!!suggestTag}
                    onClose={() => setSuggestTag(null)}
                    onSubmit={() => {}}
                    initialData={{ name: suggestTag.name, slug: suggestTag.slug, description: suggestTag.description }}
                    title="Suggest changes"
                    suggestContext={{ targetType: ReviewableContentType.Tag, targetId: suggestTag.id, current: suggestTag }}
                />
            )}

            <ConfirmModal
                isOpen={isDeleteOpen}
                onClose={() => setIsDeleteOpen(false)}
                onConfirm={handleDelete}
                title="Delete Tag"
                message={<p>Delete the tag <strong className="text-foreground">{selectedTag?.name}</strong>? Images keep existing but lose this tag.</p>}
                confirmText="Delete"
                variant="destructive"
                zIndex={70}
            />

            <Modal
                isOpen={!!infoTag}
                onClose={() => setInfoTag(null)}
                title="Tag Details"
                footer={infoTag && (
                    <div className="flex justify-end">
                        <Link to={`/gallery?includedTags=${encodeURIComponent(infoTag.slug)}`} className="btn btn-primary">
                            View images
                        </Link>
                    </div>
                )}
            >
                {infoTag && (
                    <dl className="space-y-4">
                        <div>
                            <dt className="text-sm text-muted-foreground">Name</dt>
                            <dd className="mt-0.5 text-lg font-semibold wrap-break-word">{infoTag.name}</dd>
                        </div>
                        <div>
                            <dt className="text-sm text-muted-foreground">Slug</dt>
                            <dd className="mt-1"><code className="rounded-md bg-secondary px-2 py-1 font-mono text-sm break-all">{infoTag.slug}</code></dd>
                        </div>
                        <div>
                            <dt className="text-sm text-muted-foreground">Description</dt>
                            <dd className="mt-0.5 whitespace-pre-wrap wrap-break-word">{infoTag.description || 'No description available.'}</dd>
                        </div>
                    </dl>
                )}
            </Modal>
        </div>
    );
};

export default Tags;