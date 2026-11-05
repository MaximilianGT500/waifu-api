import { useState, useEffect, useRef } from 'react';
import Modal from '../Modal';
import { Info, Loader2, Check, Clock, ChevronDown, Trash2, GitPullRequest } from 'lucide-react';
import { ReviewStatus, Role, PaginatedList, User, Artist, ReviewableContentType, SubmitEditBody } from '../../types';
import { Dropdown, DropdownTrigger, DropdownItem } from '../Dropdown';
import { useAuth } from '../../context/AuthContext';
import { useNotification } from '../../context/NotificationContext';
import SearchableSelect, { Option } from '../SearchableSelect';
import api from '../../services/api';
import { buildArtistPayload } from '../../utils/editRequests';
import AcceptSubmissionNote from '../AcceptSubmissionNote';
import { safeExternalUrl } from '../../utils/safeUrl';

const SOCIAL_FIELDS = ['twitter', 'pixiv', 'patreon', 'deviantArt'] as const;
type SocialField = typeof SOCIAL_FIELDS[number];
const SOCIAL_LABELS: Record<SocialField, string> = { twitter: 'Twitter / X', pixiv: 'Pixiv', patreon: 'Patreon', deviantArt: 'DeviantArt' };
const NAME_MAX = 30;
const LINK_MAX = 200;

function linkError(value: string | undefined): string | null {
    const v = (value ?? '').trim();
    if (!v) return null;
    if ([...v].length > LINK_MAX) return `Must not exceed ${LINK_MAX} characters.`;
    return safeExternalUrl(v) ? null : 'Must be a valid http(s) URL.';
}

export interface ArtistFormData {
    name: string;
    twitter?: string;
    pixiv?: string;
    patreon?: string;
    deviantArt?: string;
    reviewStatus?: ReviewStatus;
    creatorId?: number;
}

interface ArtistModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSubmit: (data: ArtistFormData) => Promise<void> | void;
    initialData?: Partial<ArtistFormData> & { id?: number; creatorId?: number };
    title: string;
    isReviewMode?: boolean;
    submitLabel?: string;
    onDelete?: () => void;
    suggestContext?: { targetType: ReviewableContentType; targetId: number; current: Artist };
    contentOnly?: boolean;
}

const ArtistModal = ({ isOpen, onClose, onSubmit, initialData, title, isReviewMode, submitLabel = "Save", onDelete, suggestContext, contentOnly }: ArtistModalProps) => {
    const { user } = useAuth();
    const { showNotification } = useNotification();
    const [formData, setFormData] = useState<ArtistFormData>({
        name: '',
        twitter: '',
        pixiv: '',
        patreon: '',
        deviantArt: '',
        reviewStatus: ReviewStatus.Pending
    });
    const [selectedCreator, setSelectedCreator] = useState<Option | null>(null);
    const [reason, setReason] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);
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

        setFormData({
            name: initialData?.name || '',
            twitter: initialData?.twitter || '',
            pixiv: initialData?.pixiv || '',
            patreon: initialData?.patreon || '',
            deviantArt: initialData?.deviantArt || '',
            reviewStatus: initialData?.reviewStatus ?? ReviewStatus.Pending
        });
        setIsSubmitting(false);
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

    const nameLength = [...formData.name.trim()].length;
    const nameError = !formData.name.trim()
        ? 'A name is required.'
        : nameLength > NAME_MAX ? `Must not exceed ${NAME_MAX} characters.` : null;
    const linkChanged = (f: SocialField) =>
        !isSuggestMode || (formData[f] ?? '').trim() !== (suggestContext?.current[f] ?? '').trim();
    const socialErrors = Object.fromEntries(
        SOCIAL_FIELDS.map(f => [f, linkChanged(f) ? linkError(formData[f]) : null])
    ) as Record<SocialField, string | null>;
    const hasErrors = !!nameError || SOCIAL_FIELDS.some(f => socialErrors[f]);

    const handleSubmit = async () => {
        if (isSubmitting) return;
        if (hasErrors) {
            setShowErrors(true);
            return;
        }

        if (isSuggestMode && suggestContext) {
            const { payload, isEmpty } = buildArtistPayload(suggestContext.current, {
                name: formData.name,
                patreon: formData.patreon,
                pixiv: formData.pixiv,
                twitter: formData.twitter,
                deviantArt: formData.deviantArt,
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
                                New artists must be approved by a moderator before appearing in public lists.
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
                    <label htmlFor="artist-name" className="field-label">Name</label>
                    <input
                        id="artist-name"
                        value={formData.name}
                        onChange={e => setFormData({...formData, name: e.target.value})}
                        className="input"
                        placeholder="Artist Name"
                        autoComplete="off"
                        aria-invalid={showErrors && !!nameError}
                        aria-describedby={showErrors && nameError ? 'artist-name-error' : undefined}
                        disabled={isSubmitting}
                        data-autofocus
                    />
                    <div className="mt-1.5 flex justify-between gap-3 text-xs">
                        <span id="artist-name-error" className="text-destructive">{showErrors ? nameError : null}</span>
                        <span className={`shrink-0 tabular ${nameLength > NAME_MAX ? 'text-destructive' : 'text-muted-foreground'}`}>{nameLength}/{NAME_MAX}</span>
                    </div>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    {SOCIAL_FIELDS.map((social) => {
                        const error = showErrors ? socialErrors[social] : null;
                        return (
                            <div key={social}>
                                <label htmlFor={`artist-${social}`} className="field-label">{SOCIAL_LABELS[social]}</label>
                                <input
                                    id={`artist-${social}`}
                                    type="url"
                                    inputMode="url"
                                    autoComplete="off"
                                    spellCheck={false}
                                    placeholder={`https://${social.toLowerCase()}.com/...`}
                                    value={formData[social] ?? ''}
                                    onChange={e => setFormData({...formData, [social]: e.target.value})}
                                    aria-invalid={!!error}
                                    aria-describedby={error ? `artist-${social}-error` : undefined}
                                    className="input"
                                    disabled={isSubmitting}
                                />
                                {error && <p id={`artist-${social}-error`} className="mt-1 text-xs font-medium text-destructive">{error}</p>}
                            </div>
                        );
                    })}
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
                            aria-label="Delete artist"
                            title="Delete artist"
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

export default ArtistModal;