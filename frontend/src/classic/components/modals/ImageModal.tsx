import { useState, useEffect, useRef, useEffectEvent } from 'react';
import Modal from '../Modal';
import { ImageDto, ImageFormData, PaginatedList, User, ReviewStatus, ReviewStatusFilter, Role, ReviewableContentType, SubmitEditBody } from '../../../types';
import SearchableSelect, { Option } from '../SearchableSelect';
import api from '../../../services/api';
import TagModal from './TagModal';
import ArtistModal from './ArtistModal';
import { useMetadata } from '../../../hooks/useMetadata';
import { Dropdown, DropdownTrigger, DropdownItem } from '../Dropdown';
import { Check, Clock, ChevronDown, Trash2, Upload, Loader2, GitPullRequest, X } from 'lucide-react';
import { useAuth } from '../../../context/AuthContext';
import { useNotification } from '../../../context/NotificationContext';
import { buildImagePayload } from '../../../utils/editRequests';
import { imgCard } from '../../../utils/cfImage';
import AcceptSubmissionNote from '../AcceptSubmissionNote';
import { safeExternalUrl } from '../../../utils/safeUrl';

interface ImageModalProps {
    isOpen: boolean;
    onClose: () => void;
    initialData?: ImageDto;
    onSubmit: (data: ImageFormData) => Promise<void> | void;
    onDelete?: () => void;
    suggestContext?: { targetType: ReviewableContentType; targetId: number; current: ImageDto };
    contentOnly?: boolean;
    title?: string;
}

const ImageModal = ({ isOpen, onClose, initialData, onSubmit, onDelete, suggestContext, contentOnly, title }: ImageModalProps) => {
    const usersPageSize = 30;
    const { user } = useAuth();
    const { showNotification } = useNotification();
    const [source, setSource] = useState('');
    const [isNsfw, setIsNsfw] = useState(false);
    const [selectedUser, setSelectedUser] = useState<Option | null>(null);
    const [reviewStatus, setReviewStatus] = useState<ReviewStatus>(ReviewStatus.Pending);
    const [selectedFile, setSelectedFile] = useState<File | null>(null);
    const [previewUrl, setPreviewUrl] = useState<string | null>(null);
    const [reason, setReason] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);

    const isSuggestMode = !!suggestContext;
    const readOnlyChrome = isSuggestMode || contentOnly;

    const {
        selectedTags, setSelectedTags,
        selectedArtists, setSelectedArtists,
        showCreateTagModal, setShowCreateTagModal,
        newTagName, setNewTagName,
        showCreateArtistModal, setShowCreateArtistModal,
        newArtistName, setNewArtistName,
        loadTags, loadArtists,
        handleCreateTag, handleCreateArtist
    } = useMetadata({ reviewStatus: (isSuggestMode || contentOnly) ? ReviewStatusFilter.Accepted : ReviewStatusFilter.All });

    const isAdmin = user?.role === Role.Admin;
    const hasInitialized = useRef(false);

    const initialize = useEffectEvent(() => {

        if (initialData) {
            setSource(initialData.source || '');
            setIsNsfw(initialData.isNsfw);
            setSelectedTags(initialData.tags ? initialData.tags.map(t => ({ id: t.id, name: t.name, slug: t.slug, description: t.description, reviewStatus: t.reviewStatus })) : []);
            setSelectedArtists(initialData.artists ? initialData.artists.map(a => ({ id: a.id, name: a.name, reviewStatus: a.reviewStatus })) : []);
            setReviewStatus(initialData.reviewStatus ?? ReviewStatus.Pending);
            setSelectedFile(null);
            setPreviewUrl(null);
            setReason('');
            setIsSubmitting(false);

            if (initialData.uploaderId && !readOnlyChrome) {
                const uploaderId = initialData.uploaderId;
                api.get<User>(`/users/${uploaderId}`, { skipGlobalErrorHandler: true })
                    .then(res => {
                         setSelectedUser({ id: res.data.id, name: res.data.name });
                    })
                    .catch(() => {
                        setSelectedUser({ id: uploaderId, name: `User #${uploaderId}` });
                    });
            } else {
                setSelectedUser(null);
            }
        } else {
            setSource('');
            setIsNsfw(false);
            setSelectedTags([]);
            setSelectedArtists([]);
            setSelectedUser(null);
            setReviewStatus(ReviewStatus.Pending);
            setSelectedFile(null);
            setPreviewUrl(null);
            setReason('');
            setIsSubmitting(false);
        }
    });

    useEffect(() => {
        if (!isOpen) {
            hasInitialized.current = false;
            return;
        }
        if (hasInitialized.current) return;
        hasInitialized.current = true;
        initialize();
    }, [isOpen]);

    useEffect(() => {
        if (selectedFile) {
            const url = URL.createObjectURL(selectedFile);
            setPreviewUrl(url);
            return () => URL.revokeObjectURL(url);
        } else {
            setPreviewUrl(null);
        }
    }, [selectedFile]);

    const loadUsers = async (query: string, page: number = 1) => {
        const { data } = await api.get<PaginatedList<User>>('/users', { params: { name: query, pageSize: usersPageSize, page } });
        return data.items.map(u => ({ id: u.id, name: u.name }));
    };

    const handleSubmit = async (e?: React.FormEvent) => {
        if (e) e.preventDefault();
        if (isSubmitting) return;
        const trimmedSource = source.trim();
        if (trimmedSource && ([...trimmedSource].length > 500 || !safeExternalUrl(trimmedSource))) {
            showNotification('warning', 'The source must be a valid http(s) URL with at most 500 characters.');
            return;
        }

        if (isSuggestMode && suggestContext) {
            const { payload, isEmpty } = buildImagePayload(suggestContext.current, {
                source: trimmedSource || undefined,
                isNsfw,
                tagSlugs: selectedTags.map(t => t.slug!),
                artistIds: selectedArtists.map(a => Number(a.id)),
            });
            if (isEmpty) {
                showNotification('warning', 'No changes to suggest');
                return;
            }
            setIsSubmitting(true);
            try {
                const body: SubmitEditBody = {
                    targetType: suggestContext.targetType,
                    targetId: suggestContext.targetId,
                    reason: reason.trim() || undefined,
                    payload,
                };
                await api.post('/review/tasks', body);
                showNotification('success', 'Suggestion submitted for review');
                onClose();
            } catch {
            } finally {
                setIsSubmitting(false);
            }
            return;
        }

        setIsSubmitting(true);
        try {
            await onSubmit({
                file: selectedFile || undefined,
                source: trimmedSource || undefined,
                isNsfw,
                tags: selectedTags.map(t => t.slug!),
                artists: selectedArtists.map(a => Number(a.id)),
                userId: selectedUser ? Number(selectedUser.id) : undefined,
                reviewStatus: reviewStatus
            });
            onClose();
        } catch {
            return;
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (file) {
            setSelectedFile(file);
        }
    };

    const clearSelectedFile = () => {
        setSelectedFile(null);
    };

    const getStatusBadge = (status?: ReviewStatus) => {
        switch (status) {
            case ReviewStatus.Pending: return <span className="inline-flex items-center gap-1 rounded-full bg-warning/12 px-2.5 py-0.5 text-xs font-semibold text-warning"><Clock size={12}/> Pending</span>;
            case ReviewStatus.Accepted: return <span className="inline-flex items-center gap-1 rounded-full bg-success/12 px-2.5 py-0.5 text-xs font-semibold text-success"><Check size={12}/> Accepted</span>;
            default: return <span className="text-muted-foreground text-xs">Select Status</span>;
        }
    };

    const renderStatusDropdown = () => {
        return (
            <Dropdown
                trigger={
                    <DropdownTrigger className="input flex cursor-pointer items-center justify-between text-left">
                        {getStatusBadge(reviewStatus)}
                        <ChevronDown size={16} aria-hidden="true" className="text-muted-foreground" />
                    </DropdownTrigger>
                }
                label={`Review status: ${reviewStatus === ReviewStatus.Accepted ? 'Accepted' : reviewStatus === ReviewStatus.Pending ? 'Pending' : 'not set'}`}
                align="left"
                width="w-full"
            >
                <DropdownItem
                    onClick={() => setReviewStatus(ReviewStatus.Pending)}
                    active={reviewStatus === ReviewStatus.Pending}
                    icon={<Clock size={14}/>}
                >
                    Pending
                </DropdownItem>
                <DropdownItem
                    onClick={() => setReviewStatus(ReviewStatus.Accepted)}
                    active={reviewStatus === ReviewStatus.Accepted}
                    icon={<Check size={14}/>}
                >
                    Accepted
                </DropdownItem>
            </Dropdown>
        );
    };

    return (
        <>
            <Modal
                isOpen={isOpen}
                onClose={onClose}
                title={title ?? (isSuggestMode ? `Suggest changes to Image #${suggestContext!.targetId}` : (initialData ? `Edit Image #${initialData.id}` : 'Edit Image'))}
            >
                <div className="space-y-5">
                    {isSuggestMode && (
                        <div className="flex items-start gap-3 rounded-2xl bg-info/10 p-4">
                            <GitPullRequest className="mt-0.5 shrink-0 text-info" size={18} aria-hidden="true" />
                            <div>
                                <p className="text-sm font-bold">Suggesting an edit</p>
                                <p className="mt-0.5 text-sm text-muted-foreground">
                                    Your changes will be submitted to the moderators for review.
                                </p>
                            </div>
                        </div>
                    )}

                                        {readOnlyChrome ? (
                        <div>
                            <p className="field-label">Current file</p>
                            <div className="h-48 w-full overflow-hidden rounded-xl bg-secondary">
                                {initialData?.url && (
                                    <img src={imgCard(initialData.url)} className="size-full object-contain" alt={`Upload #${initialData.id}`} />
                                )}
                            </div>
                        </div>
                    ) : (
                        <div>
                            <p className="field-label">File</p>
                            <div className="relative">
                                <label className="group block h-48 w-full cursor-pointer overflow-hidden rounded-xl border-2 border-dashed border-input transition-colors hover:border-brand/60 focus-within:ring-4 focus-within:ring-brand/25">
                                    <span className="sr-only">Replace the file</span>
                                    {previewUrl ? (
                                        <div className="relative w-full h-full">
                                            <img src={previewUrl} className="size-full object-contain" alt="Preview of the new file" />
                                            <div className="absolute inset-0 flex items-center justify-center bg-zinc-950/60 font-medium text-white opacity-0 transition-opacity group-hover:opacity-100">
                                                Click to change
                                            </div>
                                        </div>
                                    ) : initialData?.url ? (
                                        <div className="relative w-full h-full">
                                            <img src={imgCard(initialData.url)} className="size-full object-contain" alt={`Upload #${initialData.id}`} />
                                            <div className="absolute inset-0 flex items-center justify-center bg-zinc-950/60 font-medium text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
                                                <Upload size={18} className="mr-2" aria-hidden="true" /> Replace file
                                            </div>
                                        </div>
                                    ) : (
                                        <div className="flex h-full flex-col items-center justify-center text-muted-foreground">
                                            <Upload size={22} className="mb-2" aria-hidden="true" />
                                            <span className="text-sm">Choose a file</span>
                                        </div>
                                    )}
                                    <input
                                        type="file"
                                        accept="image/jpeg,image/png,image/gif,image/webp"
                                        onChange={handleFileChange}
                                        className="sr-only"
                                    />
                                </label>
                                {selectedFile && (
                                    <button
                                        type="button"
                                        onClick={clearSelectedFile}
                                        className="absolute right-2 top-2 grid size-8 place-items-center rounded-full bg-zinc-950/65 text-white backdrop-blur-md transition-colors hover:bg-zinc-950/85"
                                        title="Discard the new file"
                                        aria-label="Discard the new file"
                                    >
                                        <X size={15} aria-hidden="true" />
                                    </button>
                                )}
                            </div>
                            {selectedFile && (
                                <p className="field-hint">
                                    New file: <span className="font-semibold text-foreground">{selectedFile.name}</span>, {(selectedFile.size / (1024 * 1024)).toFixed(2)} MB
                                </p>
                            )}
                        </div>
                    )}

                    {!readOnlyChrome && (
                        <div>
                            <p className="field-label">Review status</p>
                            {renderStatusDropdown()}
                            <AcceptSubmissionNote current={initialData?.reviewStatus} selected={reviewStatus} />
                        </div>
                    )}

                    <SearchableSelect
                        label="Artists"
                        placeholder="Search artists…"
                        loadOptions={loadArtists}
                        selectedOptions={selectedArtists}
                        onSelect={(o) => setSelectedArtists(p => p.some(a => a.id === o.id) ? p : [...p, o])}
                        onRemove={(o) => setSelectedArtists(p => p.filter(a => a.id !== o.id))}
                        onCreate={(name) => { setNewArtistName(name); setShowCreateArtistModal(true); }}
                        isMulti={true}
                    />
                    <SearchableSelect
                        label="Tags"
                        placeholder="Add tags…"
                        loadOptions={loadTags}
                        selectedOptions={selectedTags}
                        onSelect={(o) => setSelectedTags(p => p.some(t => t.id === o.id) ? p : [...p, o])}
                        onRemove={(o) => setSelectedTags(p => p.filter(t => t.id !== o.id))}
                        onCreate={(name) => { setNewTagName(name); setShowCreateTagModal(true); }}
                        isMulti={true}
                    />
                    <div>
                        <label htmlFor="image-source" className="field-label">Source URL</label>
                        <input
                            id="image-source"
                            type="url"
                            inputMode="url"
                            autoComplete="off"
                            spellCheck={false}
                            value={source}
                            onChange={e => setSource(e.target.value)}
                            className="input"
                            placeholder="https://…"
                        />
                    </div>

                    <label className="grid cursor-pointer grid-cols-[auto_1fr] items-start gap-x-3 rounded-xl border border-border p-4 transition-colors hover:bg-secondary/60 has-[:checked]:border-destructive/40 has-[:checked]:bg-destructive/5">
                        <input
                            type="checkbox"
                            id="isNsfw"
                            checked={isNsfw}
                            onChange={e => setIsNsfw(e.target.checked)}
                            className="row-span-2 mt-0.5 size-[1.125rem] shrink-0 accent-rose-600"
                        />
                        <span className="text-sm font-semibold">NSFW Content</span>
                        <span className="col-start-2 text-sm text-muted-foreground">Contains adult material</span>
                    </label>

                    {!readOnlyChrome && (
                        <SearchableSelect
                            label="Uploader"
                            placeholder="Search users…"
                            loadOptions={loadUsers}
                            selectedOptions={selectedUser ? [selectedUser] : []}
                            onSelect={(o) => setSelectedUser(o)}
                            onRemove={() => setSelectedUser(null)}
                            isMulti={false}
                        />
                    )}

                    {isSuggestMode && (
                        <div>
                            <label htmlFor="suggest-reason" className="field-label">Reason <span className="font-normal text-muted-foreground">(optional)</span></label>
                        <textarea
                            id="suggest-reason"
                            value={reason}
                                onChange={e => setReason(e.target.value)}
                                className="input h-20 resize-none"
                                placeholder="Why are you suggesting this change?"
                                disabled={isSubmitting}
                            />
                        </div>
                    )}

                    <div className="flex gap-2 pt-1">
                        <button
                            type="button"
                            onClick={() => handleSubmit()}
                            disabled={isSubmitting}
                            aria-busy={isSubmitting}
                            className="btn btn-primary flex-1"
                        >
                            {isSubmitting ? (
                                <>
                                    <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                                    {isSuggestMode ? 'Submitting…' : 'Saving…'}
                                </>
                            ) : (
                                isSuggestMode ? 'Submit suggestion' : 'Save Changes'
                            )}
                        </button>
                        {!isSuggestMode && onDelete && isAdmin && (
                            <button
                                type="button"
                                onClick={onDelete}
                                disabled={isSubmitting}
                                aria-label="Delete image"
                                title="Delete image"
                                className="btn btn-danger-soft btn-icon px-3.5"
                            >
                                <Trash2 size={17} aria-hidden="true" />
                            </button>
                        )}
                    </div>
                </div>
            </Modal>

            <TagModal
                isOpen={showCreateTagModal}
                onClose={() => setShowCreateTagModal(false)}
                onSubmit={handleCreateTag}
                initialData={{ name: newTagName }}
                title="New Tag"
                submitLabel="Create"
            />

            <ArtistModal
                isOpen={showCreateArtistModal}
                onClose={() => setShowCreateArtistModal(false)}
                onSubmit={handleCreateArtist}
                initialData={{ name: newArtistName }}
                title="New Artist"
                submitLabel="Create"
            />
        </>
    );
};

export default ImageModal;