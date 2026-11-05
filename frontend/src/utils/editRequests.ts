import {
    ImageDto,
    Tag,
    Artist,
    ImageEditPayload,
    TagEditPayload,
    ArtistEditPayload,
} from '../types';

export interface PayloadResult<T> {
    payload: T;
    isEmpty: boolean;
}

const normalize = (value?: string): string | undefined => {
    const trimmed = (value ?? '').trim();
    return trimmed.length > 0 ? trimmed : undefined;
};

const diffScalar = (current?: string, proposed?: string): string | undefined => {
    const next = normalize(proposed);
    const prev = normalize(current);
    if (next === undefined) return undefined;
    if (next === prev) return undefined;
    return next;
};

export interface ProposedImage {
    source?: string;
    isNsfw: boolean;
    tagSlugs: string[];
    artistIds: number[];
}

export function buildImagePayload(
    current: ImageDto,
    proposed: ProposedImage,
): PayloadResult<ImageEditPayload> {
    const payload: ImageEditPayload = {};

    const source = diffScalar(current.source, proposed.source);
    if (source !== undefined) payload.source = source;

    if (proposed.isNsfw !== current.isNsfw) payload.isNsfw = proposed.isNsfw;

    const currentTagSlugs = (current.tags ?? []).map(t => t.slug);
    const proposedTagSlugs = proposed.tagSlugs;
    const addTagSlugs = proposedTagSlugs.filter(s => !currentTagSlugs.includes(s));
    const removeTagSlugs = currentTagSlugs.filter(s => !proposedTagSlugs.includes(s));
    if (addTagSlugs.length > 0) payload.addTagSlugs = addTagSlugs;
    if (removeTagSlugs.length > 0) payload.removeTagSlugs = removeTagSlugs;

    const currentArtistIds = (current.artists ?? []).map(a => a.id);
    const proposedArtistIds = proposed.artistIds;
    const addArtistIds = proposedArtistIds.filter(id => !currentArtistIds.includes(id));
    const removeArtistIds = currentArtistIds.filter(id => !proposedArtistIds.includes(id));
    if (addArtistIds.length > 0) payload.addArtistIds = addArtistIds;
    if (removeArtistIds.length > 0) payload.removeArtistIds = removeArtistIds;

    return { payload, isEmpty: Object.keys(payload).length === 0 };
}

export interface ProposedTag {
    name?: string;
    description?: string;
}

export function buildTagPayload(
    current: Tag,
    proposed: ProposedTag,
): PayloadResult<TagEditPayload> {
    const payload: TagEditPayload = {};

    const name = diffScalar(current.name, proposed.name);
    if (name !== undefined) payload.name = name;

    const description = diffScalar(current.description, proposed.description);
    if (description !== undefined) payload.description = description;

    return { payload, isEmpty: Object.keys(payload).length === 0 };
}

export interface ProposedArtist {
    name?: string;
    patreon?: string;
    pixiv?: string;
    twitter?: string;
    deviantArt?: string;
}

export function buildArtistPayload(
    current: Artist,
    proposed: ProposedArtist,
): PayloadResult<ArtistEditPayload> {
    const payload: ArtistEditPayload = {};

    const name = diffScalar(current.name, proposed.name);
    if (name !== undefined) payload.name = name;

    const patreon = diffScalar(current.patreon, proposed.patreon);
    if (patreon !== undefined) payload.patreon = patreon;

    const pixiv = diffScalar(current.pixiv, proposed.pixiv);
    if (pixiv !== undefined) payload.pixiv = pixiv;

    const twitter = diffScalar(current.twitter, proposed.twitter);
    if (twitter !== undefined) payload.twitter = twitter;

    const deviantArt = diffScalar(current.deviantArt, proposed.deviantArt);
    if (deviantArt !== undefined) payload.deviantArt = deviantArt;

    return { payload, isEmpty: Object.keys(payload).length === 0 };
}
