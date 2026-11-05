import { useState, useEffect, useEffectEvent } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router';
import api from '../services/api';
import {
    ReviewTask,
    ReviewTaskStatus,
    ReviewTaskKind,
    ReviewableContentType,
    ImageEditPayload,
    TagEditPayload,
    ArtistEditPayload,
    UserMinimal,
    PaginatedList,
    Role,
    ImageDto,
    ImageFormData,
    Tag,
    Artist,
} from '../types';
import { useAuth } from '../context/AuthContext';
import { useNotification } from '../context/NotificationContext';
import { useRequireAuth } from '../hooks/useRequireAuth';
import { buildImagePayload, buildTagPayload, buildArtistPayload } from '../utils/editRequests';
import ReasonModal from '../components/modals/ReasonModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import ImageModal from '../components/modals/ImageModal';
import ArtistModal, { ArtistFormData } from '../components/modals/ArtistModal';
import TagModal, { TagFormData } from '../components/modals/TagModal';
import Pagination from '../components/Pagination';
import TagChip from '../components/TagChip';
import { StatusBadge as SharedStatusBadge, StatusTone } from '../components/StatusBadge';
import { resolveTagsBySlugs, resolveArtistsByIds } from '../utils/metadataResolve';
import { imgCard } from '../utils/cfImage';
import { safeColor } from '../utils/color';
import { invalidateNavBadges } from '../hooks/useNavBadges';
import SafeExternalLink from '../components/ExternalLink';
import { displayUrl } from '../utils/safeUrl';
import {
    Check, X, ExternalLink, Image as ImageIcon, Tag as TagIcon,
    Palette, Clock, Ban, ArrowRight, Sparkles, GitPullRequest, Edit2, Link as LinkIcon,
} from 'lucide-react';

const PAGE_SIZE = 20;

const TYPE_TABS = [ReviewableContentType.Image, ReviewableContentType.Tag, ReviewableContentType.Artist] as const;

const TARGET_LABELS: Record<ReviewableContentType, string> = {
    [ReviewableContentType.Image]: 'Image',
    [ReviewableContentType.Tag]: 'Tag',
    [ReviewableContentType.Artist]: 'Artist',
};

const targetIcon = (type: ReviewableContentType, size = 14) => {
    switch (type) {
        case ReviewableContentType.Image: return <ImageIcon size={size} />;
        case ReviewableContentType.Tag: return <TagIcon size={size} />;
        case ReviewableContentType.Artist: return <Palette size={size} />;
    }
};

const targetLink = (task: ReviewTask): string => {
    switch (task.targetType) {
        case ReviewableContentType.Image: return `/images/${task.targetId}`;
        case ReviewableContentType.Tag: return `/tags?includedIds=${task.targetId}`;
        case ReviewableContentType.Artist: return `/artists?includedIds=${task.targetId}`;
    }
};

const REVIEW_STATUS: Record<ReviewTaskStatus, { tone: StatusTone; icon: React.ReactNode }> = {
    [ReviewTaskStatus.Pending]: { tone: 'pending', icon: <Clock size={12} /> },
    [ReviewTaskStatus.Approved]: { tone: 'success', icon: <Check size={12} /> },
    [ReviewTaskStatus.Rejected]: { tone: 'danger', icon: <X size={12} /> },
    [ReviewTaskStatus.Cancelled]: { tone: 'neutral', icon: <Ban size={12} /> },
};

const StatusBadge = ({ status }: { status: ReviewTaskStatus }) => (
    <SharedStatusBadge label={status} tone={REVIEW_STATUS[status].tone} icon={REVIEW_STATUS[status].icon} />
);

const KindBadge = ({ kind }: { kind: ReviewTaskKind }) => (
    kind === ReviewTaskKind.NewContent ? (
        <span className="inline-flex items-center gap-1 rounded-full bg-brand-soft px-2.5 py-0.5 text-xs font-semibold text-brand-ink">
            <Sparkles size={11} aria-hidden="true" /> New
        </span>
    ) : (
        <span className="inline-flex items-center gap-1 rounded-full bg-info/10 px-2.5 py-0.5 text-xs font-semibold text-info">
            <GitPullRequest size={11} aria-hidden="true" /> Edit
        </span>
    )
);

const ModEditedCue = ({ at }: { at?: string }) => at ? (
    <span
        className="inline-flex items-center gap-1 rounded-full bg-warning/12 px-2.5 py-0.5 text-xs font-semibold text-warning"
        title={`A moderator edited this submission on ${new Date(at).toLocaleString()}`}
    >
        <Edit2 size={11} aria-hidden="true" /> Edited by moderator
    </span>
) : null;

const UserLink = ({ user, id, plain }: { user?: UserMinimal; id?: number; plain?: boolean }) => {
    const label = user ? user.name : id !== undefined ? `User #${id}` : 'Unknown';
    if (plain || (!user && id === undefined)) {
        return <span className="font-medium">{label}</span>;
    }
    return (
        <Link to={`/users?includedIds=${user ? user.id : id}`} className="font-semibold text-foreground hover:underline">
            {label}
        </Link>
    );
};

const ScalarDiff = ({ label, oldValue, newValue }: { label: string; oldValue?: string; newValue?: string }) => (
    <div className="text-sm">
        <span className="text-xs font-semibold text-muted-foreground">{label}</span>
        <div className="flex items-center gap-2 flex-wrap mt-0.5">
            <span className="line-through text-muted-foreground break-all">{oldValue || <em>empty</em>}</span>
            <ArrowRight size={14} className="text-muted-foreground shrink-0" />
            <span className="text-foreground font-medium break-all">{newValue || <em>empty</em>}</span>
        </div>
    </div>
);

const FieldLabel = ({ children }: { children: React.ReactNode }) => (
    <span className="text-xs font-semibold text-muted-foreground">{children}</span>
);

const Unavailable = ({ label }: { label: string }) => (
    <p className="text-sm text-muted-foreground">The submitted {label.toLowerCase()} is no longer available.</p>
);

const UnsafeUrl = ({ url }: { url: string }) => (
    <span className="flex items-center gap-2 rounded-full bg-destructive/10 px-3 py-1.5 text-xs text-destructive break-all" title="Not a valid http(s) link">
        <Ban size={14} aria-hidden="true" className="shrink-0" />
        <span>{url}</span>
    </span>
);

const SocialRow = ({ url }: { url: string }) => (
    <SafeExternalLink
        href={url}
        className="flex items-center gap-2 rounded-full bg-secondary px-3 py-1.5 text-xs transition-colors hover:bg-brand-soft hover:text-brand-ink"
        title={url}
        fallback={<UnsafeUrl url={url} />}
    >
        <LinkIcon size={14} aria-hidden="true" className="text-muted-foreground shrink-0" />
        <span className="truncate">{displayUrl(url)}</span>
    </SafeExternalLink>
);

const EditDiff = ({ task }: { task: ReviewTask }) => {
    if (task.targetType === ReviewableContentType.Tag) {
        const p = (task.payload ?? {}) as TagEditPayload;
        const current = task.target?.tag;
        return (
            <div className="space-y-2">
                {p.name !== undefined && <ScalarDiff label="Name" oldValue={current?.name} newValue={p.name} />}
                {p.description !== undefined && <ScalarDiff label="Description" oldValue={current?.description} newValue={p.description} />}
            </div>
        );
    }

    if (task.targetType === ReviewableContentType.Artist) {
        const p = (task.payload ?? {}) as ArtistEditPayload;
        const current = task.target?.artist;
        return (
            <div className="space-y-2">
                {p.name !== undefined && <ScalarDiff label="Name" oldValue={current?.name} newValue={p.name} />}
                {p.twitter !== undefined && <ScalarDiff label="Twitter" oldValue={current?.twitter} newValue={p.twitter} />}
                {p.pixiv !== undefined && <ScalarDiff label="Pixiv" oldValue={current?.pixiv} newValue={p.pixiv} />}
                {p.patreon !== undefined && <ScalarDiff label="Patreon" oldValue={current?.patreon} newValue={p.patreon} />}
                {p.deviantArt !== undefined && <ScalarDiff label="DeviantArt" oldValue={current?.deviantArt} newValue={p.deviantArt} />}
            </div>
        );
    }

    return <ImageEditDiff task={task} />;
};

const ImageEditDiff = ({ task }: { task: ReviewTask }) => {
    const p = (task.payload ?? {}) as ImageEditPayload;
    const current = task.target?.image;

    const addTagSlugs = p.addTagSlugs ?? [];
    const removeTagSlugs = p.removeTagSlugs ?? [];
    const addArtistIds = p.addArtistIds ?? [];
    const removeArtistIds = p.removeArtistIds ?? [];

    const tagBySlug = new Map((current?.tags ?? []).map(t => [t.slug, t]));
    const artistById = new Map((current?.artists ?? []).map(a => [a.id, a]));

    const [addedTags, setAddedTags] = useState<Map<string, Tag>>(new Map());
    const [addedArtists, setAddedArtists] = useState<Map<number, Artist>>(new Map());

    const tagKey = addTagSlugs.join(',');
    const artistKey = addArtistIds.join(',');
    const resolveAdded = useEffectEvent(() => Promise.all([
        resolveTagsBySlugs(addTagSlugs),
        resolveArtistsByIds(addArtistIds),
    ]));

    useEffect(() => {
        let alive = true;
        resolveAdded().then(([tags, artists]) => {
            if (!alive) return;
            setAddedTags(new Map(tags.map(t => [t.slug, t])));
            setAddedArtists(new Map(artists.map(a => [a.id, a])));
        }, () => undefined);
        return () => { alive = false; };
    }, [tagKey, artistKey]);

    const hasTagChanges = addTagSlugs.length || removeTagSlugs.length;
    const hasArtistChanges = addArtistIds.length || removeArtistIds.length;

    return (
        <div className="space-y-2">
            {p.source !== undefined && <ScalarDiff label="Source" oldValue={current?.source} newValue={p.source} />}
            {p.isNsfw !== undefined && (
                <ScalarDiff label="NSFW" oldValue={current ? String(current.isNsfw) : undefined} newValue={String(p.isNsfw)} />
            )}
            {hasTagChanges ? (
                <div>
                    <FieldLabel>Tags</FieldLabel>
                    <div className="flex flex-wrap gap-1.5 mt-1">
                        {addTagSlugs.map(slug => {
                            const t = addedTags.get(slug);
                            return <TagChip key={`at-${slug}`} tone="add" href={t ? `/tags?includedIds=${t.id}` : undefined} label={t?.name ?? slug} />;
                        })}
                        {removeTagSlugs.map(slug => {
                            const t = tagBySlug.get(slug);
                            return <TagChip key={`rt-${slug}`} tone="remove" href={t ? `/tags?includedIds=${t.id}` : undefined} label={t?.name ?? slug} />;
                        })}
                    </div>
                </div>
            ) : null}
            {hasArtistChanges ? (
                <div>
                    <FieldLabel>Artists</FieldLabel>
                    <div className="flex flex-wrap gap-1.5 mt-1">
                        {addArtistIds.map(id => {
                            const a = addedArtists.get(id) ?? artistById.get(id);
                            return <TagChip key={`aa-${id}`} tone="add" href={`/artists?includedIds=${id}`} label={a?.name ?? `#${id}`} />;
                        })}
                        {removeArtistIds.map(id => {
                            const a = artistById.get(id);
                            return <TagChip key={`ra-${id}`} tone="remove" href={`/artists?includedIds=${id}`} label={a?.name ?? `#${id}`} />;
                        })}
                    </div>
                </div>
            ) : null}
        </div>
    );
};

interface CardActions {
    isModerator: boolean;
    isOwn: boolean;
    canEdit: boolean;
    onApprove: () => void;
    onReject: () => void;
    onEdit: () => void;
    onCancel: () => void;
}

const TaskMeta = ({ task, isModerator }: { task: ReviewTask; isModerator: boolean }) => (
    <div className="min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
            <KindBadge kind={task.kind} />
            <StatusBadge status={task.status} />
            <ModEditedCue at={task.moderatorEditedAt} />
        </div>
        <Link
            to={targetLink(task)}
            className="mt-2.5 inline-flex items-center gap-1.5 text-sm font-semibold hover:underline"
        >
            {targetIcon(task.targetType)}
            {TARGET_LABELS[task.targetType]} #{task.targetId}
            <ExternalLink size={12} className="text-muted-foreground" />
        </Link>
        <p className="text-xs text-muted-foreground mt-1">
            {isModerator ? (
                <>By <UserLink user={task.submitter} id={task.submitterId} />, </>
            ) : 'Submitted '}
            {new Date(task.createdAt).toLocaleDateString()}
        </p>
        {task.reviewer && (
            <p className="text-xs text-muted-foreground">
                Reviewed by <UserLink user={task.reviewer} id={task.reviewerId} plain={!isModerator} />
            </p>
        )}
        {task.reviewerNote && (
            <p className="mt-2 rounded-2xl bg-secondary/70 px-3 py-2 text-xs text-muted-foreground wrap-break-word"><span className="font-semibold text-foreground">Moderator note: </span>{task.reviewerNote}</p>
        )}
    </div>
);

const TaskFooter = ({ task, isModerator, isOwn, canEdit, onApprove, onReject, onEdit, onCancel }: { task: ReviewTask } & CardActions) => {
    if (task.status !== ReviewTaskStatus.Pending) return null;
    const isNew = task.kind === ReviewTaskKind.NewContent;
    const showEdit = canEdit && (isModerator || isOwn);

    return (
        <div className="flex flex-wrap gap-2 border-t border-border p-3">
            {isModerator && (
                <>
                    <button type="button" onClick={onApprove} className="btn btn-soft btn-sm min-w-[5.5rem] flex-1 hover:bg-success/12 hover:text-success">
                        <Check size={15} aria-hidden="true" /> Approve
                    </button>
                    <button type="button" onClick={onReject} className="btn btn-soft btn-sm min-w-[5.5rem] flex-1 hover:bg-destructive/10 hover:text-destructive">
                        <X size={15} aria-hidden="true" /> Reject
                    </button>
                </>
            )}
            {showEdit && (
                <button
                    type="button"
                    onClick={onEdit}
                    className="btn btn-soft btn-sm min-w-[5.5rem] flex-1"
                    title={isNew ? 'Edit the submitted content' : 'Refine the proposed change'}
                >
                    <Edit2 size={15} aria-hidden="true" /> Edit
                </button>
            )}
            {!isModerator && isOwn && (
                <button type="button" onClick={onCancel} className="btn btn-soft btn-sm min-w-[5.5rem] flex-1 hover:bg-destructive/10 hover:text-destructive">
                    <Ban size={15} aria-hidden="true" /> {isNew ? 'Withdraw' : 'Cancel'}
                </button>
            )}
        </div>
    );
};

const TaskCard = ({ task, actions, banner, children }: { task: ReviewTask; actions: CardActions; banner?: React.ReactNode; children: React.ReactNode }) => (
    <article className="panel flex flex-col overflow-hidden">
        {banner}
        <div className="border-b border-border p-4">
            <TaskMeta task={task} isModerator={actions.isModerator} />
        </div>
        <div className="flex-1 space-y-3 p-4">
            {task.reason && (
                <div className="text-sm">
                    <FieldLabel>Reason</FieldLabel>
                    <p className="mt-0.5 text-foreground whitespace-pre-wrap wrap-break-word">{task.reason}</p>
                </div>
            )}
            <div className="rounded-2xl bg-secondary/50 p-3">
                {children}
            </div>
        </div>
        <TaskFooter task={task} {...actions} />
    </article>
);

const ImageTaskCard = ({ task, actions }: { task: ReviewTask; actions: CardActions }) => {
    const image = task.target?.image;
    const isNew = task.kind === ReviewTaskKind.NewContent;

    const banner = image ? (
        <Link to={`/images/${task.targetId}`} className="group block bg-secondary">
            <div className="aspect-16/10 w-full overflow-hidden" style={{ backgroundColor: safeColor(image.dominantColor) ?? undefined }}>
                <img
                    src={imgCard(image.url)}
                    alt={`Submitted upload #${task.targetId}`}
                    className={`size-full object-cover ${image.isNsfw ? 'blur-xl transition-[filter] duration-300 transform-gpu group-hover:blur-none group-focus-visible:blur-none' : ''}`}
                    loading="lazy"
                />
            </div>
        </Link>
    ) : null;

    return (
        <TaskCard task={task} actions={actions} banner={banner}>
            {!image ? (
                <Unavailable label="image" />
            ) : isNew ? (
                <div className="space-y-3">
                    <div className="flex flex-wrap gap-1.5 items-center">
                        {image.isNsfw && <span className="rounded-full bg-destructive/10 px-2.5 py-0.5 text-xs font-semibold text-destructive">NSFW</span>}
                        {image.tags?.map(t => <TagChip key={`t-${t.id}`} href={`/tags?includedIds=${t.id}`} icon={<TagIcon size={11} />} label={t.name} />)}
                        {image.artists?.map(a => <TagChip key={`a-${a.id}`} href={`/artists?includedIds=${a.id}`} icon={<Palette size={11} />} label={a.name} />)}
                        {!image.isNsfw && !image.tags?.length && !image.artists?.length && (
                            <span className="text-xs text-muted-foreground">No tags or artists</span>
                        )}
                    </div>
                    {image.source && (
                        <div>
                            <FieldLabel>Source</FieldLabel>
                            <SafeExternalLink
                                href={image.source}
                                className="mt-0.5 block text-sm font-medium text-brand-ink break-all hover:underline"
                                fallback={<div className="mt-0.5"><UnsafeUrl url={image.source} /></div>}
                            >
                                {image.source}
                            </SafeExternalLink>
                        </div>
                    )}
                </div>
            ) : (
                <EditDiff task={task} />
            )}
        </TaskCard>
    );
};

const TagTaskCard = ({ task, actions }: { task: ReviewTask; actions: CardActions }) => {
    const tag = task.target?.tag;
    const isNew = task.kind === ReviewTaskKind.NewContent;

    return (
        <TaskCard task={task} actions={actions}>
            {!tag ? (
                <Unavailable label="tag" />
            ) : isNew ? (
                <div className="space-y-2">
                    <div className="flex items-center gap-2 flex-wrap">
                        <h3 className="text-base font-bold text-foreground">{tag.name}</h3>
                        <code className="select-text font-mono text-xs text-brand-ink">{tag.slug}</code>
                    </div>
                    <p className="text-sm text-muted-foreground whitespace-pre-wrap wrap-break-word">{tag.description || 'No description'}</p>
                </div>
            ) : (
                <EditDiff task={task} />
            )}
        </TaskCard>
    );
};

const ArtistTaskCard = ({ task, actions }: { task: ReviewTask; actions: CardActions }) => {
    const artist = task.target?.artist;
    const isNew = task.kind === ReviewTaskKind.NewContent;
    const hasSocials = artist && (artist.twitter || artist.pixiv || artist.deviantArt || artist.patreon);

    return (
        <TaskCard task={task} actions={actions}>
            {!artist ? (
                <Unavailable label="artist" />
            ) : isNew ? (
                <div className="space-y-2">
                    <h3 className="font-bold text-base text-foreground">{artist.name}</h3>
                    {hasSocials ? (
                        <div className="flex flex-col gap-1.5">
                            {artist.twitter && <SocialRow url={artist.twitter} />}
                            {artist.pixiv && <SocialRow url={artist.pixiv} />}
                            {artist.deviantArt && <SocialRow url={artist.deviantArt} />}
                            {artist.patreon && <SocialRow url={artist.patreon} />}
                        </div>
                    ) : (
                        <span className="text-xs text-muted-foreground">No links</span>
                    )}
                </div>
            ) : (
                <EditDiff task={task} />
            )}
        </TaskCard>
    );
};

const imageInitial = (task: ReviewTask): ImageDto | undefined => {
    const image = task.target?.image;
    if (!image) return undefined;
    if (task.kind === ReviewTaskKind.NewContent) return image;
    const p = (task.payload ?? {}) as ImageEditPayload;
    const removeTagSlugs = new Set(p.removeTagSlugs ?? []);
    const removeArtistIds = new Set(p.removeArtistIds ?? []);
    const tags = [
        ...(image.tags ?? []).filter(t => !removeTagSlugs.has(t.slug)),
        ...(p.addTagSlugs ?? []).map((slug, i) => ({ id: -1 - i, name: slug, slug, description: '' })),
    ];
    const artists = [
        ...(image.artists ?? []).filter(a => !removeArtistIds.has(a.id)),
        ...(p.addArtistIds ?? []).map(id => ({ id, name: `#${id}` })),
    ];
    return { ...image, source: p.source ?? image.source, isNsfw: p.isNsfw ?? image.isNsfw, tags, artists };
};

const tagInitial = (task: ReviewTask): (Partial<TagFormData> & { id?: number }) | undefined => {
    const tag = task.target?.tag;
    if (!tag) return undefined;
    if (task.kind === ReviewTaskKind.NewContent) return { id: tag.id, name: tag.name, slug: tag.slug, description: tag.description };
    const p = (task.payload ?? {}) as TagEditPayload;
    return { id: tag.id, name: p.name ?? tag.name, slug: tag.slug, description: p.description ?? tag.description };
};

const artistInitial = (task: ReviewTask): (Partial<ArtistFormData> & { id?: number }) | undefined => {
    const artist = task.target?.artist;
    if (!artist) return undefined;
    if (task.kind === ReviewTaskKind.NewContent) {
        return { id: artist.id, name: artist.name, twitter: artist.twitter, pixiv: artist.pixiv, patreon: artist.patreon, deviantArt: artist.deviantArt };
    }
    const p = (task.payload ?? {}) as ArtistEditPayload;
    return {
        id: artist.id,
        name: p.name ?? artist.name,
        twitter: p.twitter ?? artist.twitter,
        pixiv: p.pixiv ?? artist.pixiv,
        patreon: p.patreon ?? artist.patreon,
        deviantArt: p.deviantArt ?? artist.deviantArt,
    };
};

const editTitle = (task: ReviewTask) =>
    `${task.kind === ReviewTaskKind.NewContent ? 'Edit submission' : 'Refine suggestion'}: ${TARGET_LABELS[task.targetType]} #${task.targetId}`;

const Review = () => {
    const user = useRequireAuth();
    const { user: authUser } = useAuth();
    const { showNotification } = useNotification();

    const isModerator = !!authUser && (authUser.role === Role.Moderator || authUser.role === Role.Admin);

    const [tasks, setTasks] = useState<ReviewTask[]>([]);
    const [loading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);
    const [counts, setCounts] = useState<Record<ReviewableContentType, number>>({
        [ReviewableContentType.Image]: 0,
        [ReviewableContentType.Tag]: 0,
        [ReviewableContentType.Artist]: 0,
    });

    const [searchParams] = useSearchParams();
    const initialType = searchParams.get('type') === ReviewableContentType.Tag
        ? ReviewableContentType.Tag
        : searchParams.get('type') === ReviewableContentType.Artist
            ? ReviewableContentType.Artist
            : ReviewableContentType.Image;
    const [typeTab, setTypeTab] = useState<ReviewableContentType>(initialType);
    const [statusFilter, setStatusFilter] = useState<ReviewTaskStatus>(ReviewTaskStatus.Pending);
    const [kindFilter, setKindFilter] = useState<ReviewTaskKind | 'All'>('All');

    const [rejectTarget, setRejectTarget] = useState<ReviewTask | null>(null);
    const [cancelTarget, setCancelTarget] = useState<ReviewTask | null>(null);
    const [editingTask, setEditingTask] = useState<ReviewTask | null>(null);
    const [editingImageData, setEditingImageData] = useState<ImageDto | undefined>(undefined);
    const queryClient = useQueryClient();

    const openEditTask = async (task: ReviewTask) => {
        if (task.targetType === ReviewableContentType.Image) {
            const base = imageInitial(task);
            if (base && task.kind === ReviewTaskKind.Edit) {
                const p = (task.payload ?? {}) as ImageEditPayload;
                const [addedTags, addedArtists] = await Promise.all([
                    resolveTagsBySlugs(p.addTagSlugs ?? []),
                    resolveArtistsByIds(p.addArtistIds ?? []),
                ]);
                const bySlug = new Map(addedTags.map(t => [t.slug, t]));
                const byId = new Map(addedArtists.map(a => [a.id, a]));
                setEditingImageData({
                    ...base,
                    tags: base.tags.map(t => (t.id < 0 ? bySlug.get(t.slug) ?? t : t)),
                    artists: base.artists.map(a => byId.get(a.id) ?? a),
                });
            } else {
                setEditingImageData(base);
            }
        }
        setEditingTask(task);
    };

    const fetchTasks = async () => {
        if (!user) return;
        setLoading(true);
        try {
            const params: Record<string, string | number> = { page, pageSize: PAGE_SIZE, status: statusFilter, targetType: typeTab };
            if (kindFilter !== 'All') params.kind = kindFilter;
            const { data } = await api.get<PaginatedList<ReviewTask>>('/review/tasks', { params });
            setTasks(data.items);
            setTotalPages(data.totalPages);
        } catch {
        } finally {
            setLoading(false);
        }
    };

    const fetchCounts = async () => {
        if (!user) return;
        try {
            const results = await Promise.all(TYPE_TABS.map(t => {
                const params: Record<string, string | number> = { status: ReviewTaskStatus.Pending, targetType: t, page: 1, pageSize: 1 };
                if (kindFilter !== 'All') params.kind = kindFilter;
                return api.get<PaginatedList<ReviewTask>>('/review/tasks', { params, skipGlobalErrorHandler: true })
                    .then(r => r.data.totalCount)
                    .catch(() => 0);
            }));
            setCounts({
                [ReviewableContentType.Image]: results[0],
                [ReviewableContentType.Tag]: results[1],
                [ReviewableContentType.Artist]: results[2],
            });
        } catch {
        }
    };

    useEffect(() => {
        setPage(1);
    }, [statusFilter, kindFilter, typeTab]);

    const loadTasks = useEffectEvent(() => {
        fetchTasks();
    });

    const loadCounts = useEffectEvent(() => {
        fetchCounts();
    });

    useEffect(() => {
        if (user) loadTasks();
    }, [page, user, statusFilter, kindFilter, typeTab]);

    useEffect(() => {
        if (user) loadCounts();
    }, [user, kindFilter]);

    const handleApprove = async (task: ReviewTask) => {
        try {
            await api.patch(`/review/tasks/${task.id}/approve`);
            showNotification('success', task.kind === ReviewTaskKind.NewContent ? 'Content published' : 'Edit applied');
            setTasks(prev => prev.filter(t => t.id !== task.id));
            fetchCounts();
            invalidateNavBadges(queryClient);
        } catch {
        }
    };

    const handleReject = async (reason: string) => {
        if (!rejectTarget) return;
        try {
            await api.patch(`/review/tasks/${rejectTarget.id}/reject`, { reason: reason || undefined });
            showNotification('success', rejectTarget.kind === ReviewTaskKind.NewContent ? 'Submission rejected' : 'Edit declined');
            setTasks(prev => prev.filter(t => t.id !== rejectTarget.id));
            setRejectTarget(null);
            fetchCounts();
            invalidateNavBadges(queryClient);
        } catch {
        }
    };

    const handleCancel = async () => {
        if (!cancelTarget) return;
        try {
            await api.patch(`/review/tasks/${cancelTarget.id}/cancel`);
            showNotification('success', 'Task cancelled');
            setTasks(prev => prev.filter(t => t.id !== cancelTarget.id));
            setCancelTarget(null);
            fetchCounts();
            invalidateNavBadges(queryClient);
        } catch {
        }
    };

    const patchTaskContent = async (task: ReviewTask, content: Record<string, unknown>) => {
        await api.patch(`/review/tasks/${task.id}`, { content });
        showNotification('success', task.kind === ReviewTaskKind.NewContent ? 'Submission updated' : 'Suggestion updated');
        setEditingTask(null);
        fetchTasks();
        fetchCounts();
    };

    const submitImageEdit = async (task: ReviewTask, data: ImageFormData) => {
        const current = task.target?.image;
        if (!current) return;
        if (task.kind === ReviewTaskKind.NewContent) {
            await patchTaskContent(task, { source: data.source ?? '', isNsfw: data.isNsfw, tags: data.tags, artists: data.artists });
            return;
        }
        const { payload, isEmpty } = buildImagePayload(current, {
            source: data.source,
            isNsfw: data.isNsfw,
            tagSlugs: data.tags,
            artistIds: data.artists,
        });
        if (isEmpty) { showNotification('warning', 'No changes to save'); return; }
        await patchTaskContent(task, payload as unknown as Record<string, unknown>);
    };

    const submitTagEdit = async (task: ReviewTask, data: TagFormData) => {
        const current = task.target?.tag;
        if (!current) return;
        if (task.kind === ReviewTaskKind.NewContent) {
            await patchTaskContent(task, { name: data.name, description: data.description });
            return;
        }
        const { payload, isEmpty } = buildTagPayload(current, { name: data.name, description: data.description });
        if (isEmpty) { showNotification('warning', 'No changes to save'); return; }
        await patchTaskContent(task, payload as unknown as Record<string, unknown>);
    };

    const submitArtistEdit = async (task: ReviewTask, data: ArtistFormData) => {
        const current = task.target?.artist;
        if (!current) return;
        if (task.kind === ReviewTaskKind.NewContent) {
            await patchTaskContent(task, {
                name: data.name,
                patreon: data.patreon ?? '',
                pixiv: data.pixiv ?? '',
                twitter: data.twitter ?? '',
                deviantArt: data.deviantArt ?? '',
            });
            return;
        }
        const { payload, isEmpty } = buildArtistPayload(current, {
            name: data.name,
            patreon: data.patreon,
            pixiv: data.pixiv,
            twitter: data.twitter,
            deviantArt: data.deviantArt,
        });
        if (isEmpty) { showNotification('warning', 'No changes to save'); return; }
        await patchTaskContent(task, payload as unknown as Record<string, unknown>);
    };

    if (!user) return null;

    const kindLabel = (k: ReviewTaskKind | 'All') =>
        k === 'All' ? 'All' : k === ReviewTaskKind.NewContent ? 'New content' : 'Edits';

    const emptyTitle = isModerator ? 'All caught up!' : 'Nothing here yet';
    const emptySubtitle = isModerator
        ? `No ${statusFilter.toLowerCase()} ${TARGET_LABELS[typeTab].toLowerCase()} tasks.`
        : 'Your uploads and suggestions will appear here.';

    const segment = (active: boolean) => `flex flex-1 items-center justify-center gap-2 rounded-full px-4 py-1.5 text-sm font-bold whitespace-nowrap transition-colors sm:flex-initial ${active ? 'bg-card text-foreground shadow-card' : 'text-muted-foreground hover:text-foreground'}`;

    return (
        <div className="page">
            <div className="page-header">
                <div>
                    <h1 className="page-title">{isModerator ? 'Review queue' : 'My Submissions'}</h1>
                    <p className="page-lede">
                        {isModerator
                            ? 'New uploads and proposed edits from the community, waiting for a decision.'
                            : 'Your uploads and suggested edits, and what the moderators decided.'}
                    </p>
                </div>
            </div>

            <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:flex-wrap">
                <div role="group" aria-label="Content type" className="flex rounded-full bg-sleeve p-1 shadow-[inset_0_1px_2px_rgb(9_9_11/0.1)]">
                    {TYPE_TABS.map(t => (
                        <button key={t} type="button" aria-pressed={typeTab === t} onClick={() => setTypeTab(t)} className={segment(typeTab === t)}>
                            {TARGET_LABELS[t]}s
                            {counts[t] > 0 && (
                                <span className={`min-w-5 rounded-full px-1.5 py-0.5 text-center text-[0.6875rem] font-bold leading-none tabular ${typeTab === t ? 'bg-brand text-white dark:text-zinc-950' : 'bg-brand-soft text-brand-ink'}`}>
                                    {counts[t] > 99 ? '99+' : counts[t]}
                                </span>
                            )}
                        </button>
                    ))}
                </div>
                <div role="group" aria-label="Status" className="flex rounded-full bg-sleeve p-1 shadow-[inset_0_1px_2px_rgb(9_9_11/0.1)]">
                    {[ReviewTaskStatus.Pending, ReviewTaskStatus.Approved, ReviewTaskStatus.Rejected].map(s => (
                        <button key={s} type="button" aria-pressed={statusFilter === s} onClick={() => setStatusFilter(s)} className={segment(statusFilter === s)}>
                            {s}
                        </button>
                    ))}
                </div>
                <div role="group" aria-label="Kind of change" className="flex rounded-full bg-sleeve p-1 shadow-[inset_0_1px_2px_rgb(9_9_11/0.1)]">
                    {(['All', ReviewTaskKind.NewContent, ReviewTaskKind.Edit] as const).map(k => (
                        <button key={k} type="button" aria-pressed={kindFilter === k} onClick={() => setKindFilter(k)} className={segment(kindFilter === k)}>
                            {kindLabel(k)}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? (
                <div className="py-20 text-center text-muted-foreground" role="status">Loading tasks…</div>
            ) : tasks.length === 0 ? (
                <div className="flex h-64 flex-col items-center justify-center rounded-2xl border-2 border-dashed border-border px-6 text-center text-muted-foreground">
                    <Check size={40} className="mb-3 opacity-30" aria-hidden="true" />
                    <p className="font-semibold text-foreground">{emptyTitle}</p>
                    <p className="mt-1 text-sm">{emptySubtitle}</p>
                </div>
            ) : (
                <div>
                    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 2xl:grid-cols-3">
                        {tasks.map(task => {
                            const isOwn = authUser?.id === task.submitterId;
                            const canEdit = !!task.target && (
                                (task.targetType === ReviewableContentType.Image && !!task.target.image) ||
                                (task.targetType === ReviewableContentType.Tag && !!task.target.tag) ||
                                (task.targetType === ReviewableContentType.Artist && !!task.target.artist)
                            );
                            const actions: CardActions = {
                                isModerator,
                                isOwn,
                                canEdit,
                                onApprove: () => handleApprove(task),
                                onReject: () => setRejectTarget(task),
                                onEdit: () => openEditTask(task),
                                onCancel: () => setCancelTarget(task),
                            };
                            switch (task.targetType) {
                                case ReviewableContentType.Image: return <ImageTaskCard key={task.id} task={task} actions={actions} />;
                                case ReviewableContentType.Tag: return <TagTaskCard key={task.id} task={task} actions={actions} />;
                                case ReviewableContentType.Artist: return <ArtistTaskCard key={task.id} task={task} actions={actions} />;
                            }
                        })}
                    </div>

                    {!loading && (
                        <Pagination currentPage={page} totalPages={totalPages} setPage={setPage} />
                    )}
                </div>
            )}

            <ReasonModal
                isOpen={!!rejectTarget}
                onClose={() => setRejectTarget(null)}
                onSubmit={handleReject}
                title={rejectTarget?.kind === ReviewTaskKind.NewContent ? 'Reject Submission' : 'Reject Edit'}
                description={
                    rejectTarget?.kind === ReviewTaskKind.NewContent
                        ? 'Rejecting deletes the submitted content. Optionally provide a reason for the submitter.'
                        : 'Optionally provide a reason. This will be visible to the submitter.'
                }
                placeholder="Reason for rejection..."
                submitText="Reject"
                required={false}
            />

            <ConfirmModal
                isOpen={!!cancelTarget}
                onClose={() => setCancelTarget(null)}
                onConfirm={handleCancel}
                title={cancelTarget?.kind === ReviewTaskKind.NewContent ? 'Withdraw Submission' : 'Cancel Suggestion'}
                message={
                    cancelTarget?.kind === ReviewTaskKind.NewContent
                        ? 'Withdraw this submission? Your upload will be deleted.'
                        : 'Cancel this suggestion? The moderators will no longer see it.'
                }
                confirmText={cancelTarget?.kind === ReviewTaskKind.NewContent ? 'Withdraw' : 'Cancel Suggestion'}
                cancelText="Keep"
                variant="warning"
            />

            {editingTask?.targetType === ReviewableContentType.Image && (
                <ImageModal
                    isOpen={true}
                    onClose={() => setEditingTask(null)}
                    contentOnly
                    title={editTitle(editingTask)}
                    initialData={editingImageData}
                    onSubmit={(data) => submitImageEdit(editingTask, data)}
                />
            )}
            {editingTask?.targetType === ReviewableContentType.Tag && (
                <TagModal
                    isOpen={true}
                    onClose={() => setEditingTask(null)}
                    contentOnly
                    title={editTitle(editingTask)}
                    initialData={tagInitial(editingTask)}
                    onSubmit={(data) => submitTagEdit(editingTask, data)}
                />
            )}
            {editingTask?.targetType === ReviewableContentType.Artist && (
                <ArtistModal
                    isOpen={true}
                    onClose={() => setEditingTask(null)}
                    contentOnly
                    title={editTitle(editingTask)}
                    initialData={artistInitial(editingTask)}
                    onSubmit={(data) => submitArtistEdit(editingTask, data)}
                />
            )}
        </div>
    );
};

export default Review;
