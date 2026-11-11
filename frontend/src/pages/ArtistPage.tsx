import { useParams } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { ExternalLink as ExternalLinkIcon } from 'lucide-react';
import api, { httpStatus } from '../services/api';
import { Artist, ReviewStatus } from '../types';
import { isId } from '../utils/ids';
import { safeExternalUrl, displayUrl } from '../utils/safeUrl';
import { MetaTags } from '../hooks/useMetaTags';
import EntityGallery from '../components/layout/EntityGallery';
import EntityHeader from '../components/EntityHeader';
import ExternalLink from '../components/ExternalLink';
import NotFound from './NotFound';

const PROFILES: { key: keyof Pick<Artist, 'pixiv' | 'twitter' | 'patreon' | 'deviantArt'>; label: string }[] = [
    { key: 'pixiv', label: 'Pixiv' },
    { key: 'twitter', label: 'X / Twitter' },
    { key: 'patreon', label: 'Patreon' },
    { key: 'deviantArt', label: 'DeviantArt' },
];

const ArtistPage = () => {
    const { id } = useParams<{ id: string }>();
    if (!isId(id)) return <NotFound />;
    return <ArtistView id={id} />;
};

const ArtistView = ({ id }: { id: string }) => {
    const artist = useQuery({
        queryKey: ['artist', id],
        staleTime: 60_000,
        retry: false,
        queryFn: async ({ signal }) => (await api.get<Artist>(`/artists/${id}`, { skipGlobalErrorHandler: true, signal })).data,
    });

    if (httpStatus(artist.error) === 404) return <NotFound />;

    const data = artist.data;
    const profiles = data ? PROFILES.filter(({ key }) => safeExternalUrl(data[key])) : [];

    const header = (
        <EntityHeader kind="Artist" backTo="/artists" backLabel="All artists" title={data?.name} pending={data?.reviewStatus === ReviewStatus.Pending}>
            {profiles.length > 0 && (
                <ul className="mt-4 flex flex-wrap gap-2">
                    {profiles.map(({ key, label }) => (
                        <li key={key}>
                            <ExternalLink
                                href={data![key]}
                                title={displayUrl(data![key] as string)}
                                className="sticker gap-1.5 py-1.5 pl-3.5 pr-3 text-sm transition-colors hover:bg-secondary"
                            >
                                {label} <ExternalLinkIcon size={13} aria-hidden="true" className="text-muted-foreground" />
                            </ExternalLink>
                        </li>
                    ))}
                </ul>
            )}
        </EntityHeader>
    );

    return (
        <>
            <MetaTags title={data ? data.name : 'Artist'} description={data ? `Artworks by ${data.name}.` : undefined} />
            <EntityGallery filterKey="includedArtists" filterValue={id} header={header} />
        </>
    );
};

export default ArtistPage;
