import { useState, useEffect, useRef } from 'react';
import Modal from '../Modal';
import { Info, Loader2, Link as LinkIcon, Check, Clock, ChevronDown, Trash2, GitPullRequest } from 'lucide-react';
import { ReviewStatus, Role, PaginatedList, User, Tag, ReviewableContentType, SubmitEditBody } from '../../../types';
import { Dropdown, DropdownTrigger, DropdownItem } from '../Dropdown';
import { useAuth } from '../../../context/AuthContext';
import { useNotification } from '../../../context/NotificationContext';
import SearchableSelect, { Option } from '../SearchableSelect';
import api from '../../../services/api';
import { buildTagPayload } from '../../../utils/editRequests';
import AcceptSubmissionNote from '../AcceptSubmissionNote';
import { slugify, SLUG_MAX, SLUG_PATTERN } from '../../../utils/slug';

const NAME_MAX = 30;
const DESCRIPTION_MAX = 150;
const length = (v: string) => [...v.trim()].length;

export interface TagFormData {
    name: string;
    slug: string;
    description: string;
    reviewStatus?: ReviewStatus;
    creatorId?: number;
}

interface TagModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSubmit: (data: TagFormData) => Promise<void> | void;
    initialData?: Partial<TagFormData> & { id?: number; creatorId?: number };
    title: string;
    isReviewMode?: boolean;
    submitLabel?: string;
    onDelete?: () => void;
    suggestContext?: { targetType: ReviewableContentType; targetId: number; current: Tag };
    contentOnly?: boolean;
}

const TagModal = ({ isOpen, onClose, onSubmit, initialData, title, isReviewMode, submitLabel = "Save", onDelete, suggestContext, contentOnly }: TagModalProps) => {
    const { user } = useAuth();
    const { showNotification } = useNotification();
    const [formData, setFormData] = useState<TagFormData>({
        name: '',
        slug: '',
        description: '',
        reviewStatus: ReviewStatus.Pending
    });
    const [selectedCreator, setSelectedCreator] = useState<Option | null>(null);
    const [reason, setReason] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [isSlugManuallyEdited, setIsSlugManuallyEdited] = useState(false);
    const [showErrors, setShowErrors] = useState(false);
    const hasInitialized = useRef(false);

    const isSuggestMode = !!suggestContext;
    const isEditMode = !!initialData?.id && !isSuggestMode && !contentOnly;
    const isAdmin = user?.role === Role.Admin;

    useEffect(() => {
        if (!isOpen) {
            hasInitialized.current = false;
            return;
        }
        if (hasInitialized.current) return;
        hasInitialized.current = true;

        const initialName = initialData?.name || '';
        const initialSlug = initialData?.slug || '';

        const derivedSlug = initialSlug || (initialName ? slugify(initialName) : '');

        setFormData({
            name: initialName,
            slug: derivedSlug,
            description: initialData?.description || '',
            reviewStatus: initialData?.reviewStatus ?? ReviewStatus.Pending
        });
        setIsSubmitting(false);
        setIsSlugManuallyEdited(!!initialSlug);
        setShowErrors(false);
        setReason('');

        if (initialData?.creatorId) {
            const creatorId = initialData.creatorId;
            api.get<User>(`/users/${creatorId}`, { skipGlobalErrorHandler: true })
                .then(res => setSelectedCreator({ id: res.data.id, name: res.data.name }))
                .catch(() => setSelectedCreator({ id: creatorId, name: `User #${creatorId}` }));
        } else {
            setSelectedCreator(null);
        }
    }, [isOpen, initialData]);

    const loadUsers = async (query: string, page: number = 1) => {
        const { data } = await api.get<PaginatedList<User>>('/users', { params: { name: query, pageSize: 30, page } });
        return data.items.map(u => ({ id: u.id, name: u.name }));
    };

    const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const name = e.target.value;
        setFormData(prev => ({
            ...prev,
            name,
            slug: isSlugManuallyEdited ? prev.slug : slugify(name)
        }));
    };

    const handleSlugChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        setFormData(prev => ({ ...prev, slug: e.target.value }));
        setIsSlugManuallyEdited(true);
    };

    const slugEditable = !isSuggestMode && !contentOnly;
    const nameError = !formData.name.trim()
        ? 'A name is required.'
        : length(formData.name) > NAME_MAX ? `Must not exceed ${NAME_MAX} characters.` : null;
    const descriptionError = !formData.description.trim() && !isSuggestMode
        ? 'A description is required.'
        : length(formData.description) > DESCRIPTION_MAX ? `Must not exceed ${DESCRIPTION_MAX} characters.` : null;
    const slugError = !slugEditable
        ? null
        : !formData.slug
            ? 'Please enter a slug (lowercase letters, numbers and hyphens), e.g. for names in non-Latin scripts.'
            : formData.slug.length > SLUG_MAX
                ? `Must not exceed ${SLUG_MAX} characters.`
                : !SLUG_PATTERN.test(formData.slug) ? 'Only lowercase letters, numbers and hyphens are allowed.' : null;
    const hasErrors = !!nameError || !!descriptionError || !!slugError;

    const handleSubmit = async () => {
        if (isSubmitting) return;
        if (hasErrors) {
            setShowErrors(true);
            return;
        }

        if (isSuggestMode && suggestContext) {
            const { payload, isEmpty } = buildTagPayload(suggestContext.current, {
                name: formData.name,
                description: formData.description,
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
                ...formData,
                creatorId: selectedCreator ? Number(selectedCreator.id) : undefined
            });
        } catch {
        } finally {
            setIsSubmitting(false);
        }
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
                        {getStatusBadge(formData.reviewStatus)}
                        <ChevronDown size={16} aria-hidden="true" className="text-muted-foreground" />
                    </DropdownTrigger>
                }
                label={`Review status: ${formData.reviewStatus === ReviewStatus.Accepted ? 'Accepted' : formData.reviewStatus === ReviewStatus.Pending ? 'Pending' : 'not set'}`}
                align="left"
                width="w-full"
            >
                <DropdownItem
                    onClick={() => setFormData({...formData, reviewStatus: ReviewStatus.Pending})}
                    active={formData.reviewStatus === ReviewStatus.Pending}
                    icon={<Clock size={14}/>}
                >
                    Pending
                </DropdownItem>
                <DropdownItem
                    onClick={() => setFormData({...formData, reviewStatus: ReviewStatus.Accepted})}
                    active={formData.reviewStatus === ReviewStatus.Accepted}
                    icon={<Check size={14}/>}
                >
                    Accepted
                </DropdownItem>
            </Dropdown>
        );
    };

    return (
        <Modal
            isOpen={isOpen}
            onClose={onClose}
            title={title}
        >
            <div className="space-y-4">
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

                {isReviewMode && !isEditMode && !isSuggestMode && (
                    <div className="flex items-start gap-3 rounded-2xl bg-info/10 p-4">
                        <Info className="mt-0.5 shrink-0 text-info" size={18} aria-hidden="true" />
                        <div>
                            <p className="text-sm font-bold">Review required</p>
                            <p className="mt-0.5 text-sm text-muted-foreground">
                                New tags must be approved by a moderator before appearing in public lists.
                            </p>
                        </div>
                    </div>
                )}

                                {isEditMode && (
                    <div>
                        <p className="field-label">Review status</p>
                        {renderStatusDropdown()}
                        <AcceptSubmissionNote current={initialData?.reviewStatus} selected={formData.reviewStatus} />
                    </div>
                )}

                {isEditMode && (
                    <SearchableSelect
                        label="Creator"
                        placeholder="Search users…"
                        loadOptions={loadUsers}
                        selectedOptions={selectedCreator ? [selectedCreator] : []}
                        onSelect={(o) => setSelectedCreator(o)}
                        onRemove={() => setSelectedCreator(null)}
                        isMulti={false}
                    />
                )}

                <div>
                    <label htmlFor="tag-name" className="field-label">Name</label>
                    <input
                        id="tag-name"
                        value={formData.name}
                        onChange={handleNameChange}
                        className="input"
                        placeholder="Tag name"
                        autoComplete="off"
                        aria-invalid={showErrors && !!nameError}
                        disabled={isSubmitting}
                        data-autofocus
                    />
                    <div className="mt-1.5 flex justify-between gap-3 text-xs">
                        <span className="text-destructive">{showErrors ? nameError : null}</span>
                        <span className={`shrink-0 tabular ${length(formData.name) > NAME_MAX ? 'text-destructive' : 'text-muted-foreground'}`}>{length(formData.name)}/{NAME_MAX}</span>
                    </div>
                </div>

                {slugEditable && (
                    <div>
                        <label htmlFor="tag-slug" className="field-label">
                            Slug <span className="font-normal text-muted-foreground">used in URLs and the API</span>
                        </label>
                        <div className="relative">
                            <LinkIcon size={15} aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-muted-foreground" />
                            <input
                                id="tag-slug"
                                value={formData.slug}
                                onChange={handleSlugChange}
                                className="input pl-9 font-mono"
                                placeholder="tag-slug"
                                maxLength={SLUG_MAX}
                                autoComplete="off"
                                spellCheck={false}
                                aria-invalid={showErrors && !!slugError}
                                disabled={isSubmitting}
                            />
                        </div>
                        {showErrors && slugError && <p className="mt-1 text-xs font-medium text-destructive">{slugError}</p>}
                    </div>
                )}

                <div>
                    <label htmlFor="tag-description" className="field-label">Description</label>
                    <textarea
                        id="tag-description"
                        value={formData.description}
                        onChange={e => setFormData({...formData, description: e.target.value})}
                        className="input h-24 resize-none"
                        placeholder="What the tag describes"
                        aria-invalid={showErrors && !!descriptionError}
                        disabled={isSubmitting}
                    />
                    <div className="mt-1.5 flex justify-between gap-3 text-xs">
                        <span className="text-destructive">{showErrors ? descriptionError : null}</span>
                        <span className={`shrink-0 tabular ${length(formData.description) > DESCRIPTION_MAX ? 'text-destructive' : 'text-muted-foreground'}`}>{length(formData.description)}/{DESCRIPTION_MAX}</span>
                    </div>
                </div>

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
                        onClick={handleSubmit}
                        disabled={isSubmitting}
                        aria-busy={isSubmitting}
                        className="btn btn-primary flex-1"
                    >
                        {isSubmitting && <Loader2 size={16} className="animate-spin" aria-hidden="true" />}
                        {isSuggestMode ? 'Submit suggestion' : submitLabel}
                    </button>
                    {onDelete && isAdmin && (
                        <button
                            type="button"
                            onClick={onDelete}
                            disabled={isSubmitting}
                            aria-label="Delete tag"
                            title="Delete tag"
                            className="btn btn-danger-soft btn-icon px-3.5"
                        >
                            <Trash2 size={17} aria-hidden="true" />
                        </button>
                    )}
                </div>
            </div>
        </Modal>
    );
};

export default TagModal;