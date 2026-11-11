import { Link } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight } from 'lucide-react';
import api from '../services/api';
import { ImageDto, ImageOrderBy, NsfwMode, PaginatedList, ReviewStatus } from '../types';
import ThumbGrid from './ThumbGrid';

const COUNT = 6;


interface RelatedImagesProps {
    image: ImageDto;
    allowNsfw: boolean;
}

const RelatedImages = ({ image, allowNsfw }: RelatedImagesProps) => {
    const artist = image.artists.find((a) => a.reviewStatus !== ReviewStatus.Pending);
    const tag = artist ? undefined : image.tags.find((t) => t.reviewStatus !== ReviewStatus.Pending);
    const source = artist
        ? { key: 'includedArtists', value: String(artist.id), title: <>More by <span className="text-brand-ink">{artist.name}</span></>, label: `More by ${artist.name}` }
        : tag
            ? { key: 'includedTags', value: tag.slug, title: <>More tagged <span className="text-brand-ink">{tag.name}</span></>, label: `More tagged ${tag.name}` }
            : null;
    const isNsfw = allowNsfw && image.isNsfw ? NsfwMode.All : NsfwMode.False;

    const related = useQuery({
        queryKey: ['related', source?.key, source?.value, isNsfw, image.id],
        enabled: !!source,
        staleTime: 5 * 60_000,
        queryFn: async ({ signal }) => {
            const { data } = await api.get<PaginatedList<ImageDto>>('/images', {
                params: { [source!.key]: source!.value, orderBy: ImageOrderBy.Random, pageSize: COUNT + 1, isNsfw },
                skipGlobalErrorHandler: true,
                signal,
            });
            return data.items.filter((item) => item.id !== image.id).slice(0, COUNT);
        },
    });

    if (!source || related.isError || (related.isSuccess && related.data.length === 0)) return null;

    const entityPath = source.key === 'includedArtists' ? `/artists/${source.value}` : `/tags/${encodeURIComponent(source.value)}`;
    const galleryHref = isNsfw !== NsfwMode.False ? `${entityPath}?isNsfw=${isNsfw}` : entityPath;
    const items = related.data ?? [];

    return (
        <section aria-label={source.label} className="mt-14 border-t-2 border-dashed border-border pt-8">
            <div className="mb-5 flex flex-wrap items-baseline justify-between gap-3">
                <h2 className="text-xl font-black">{source.title}</h2>
                <Link to={galleryHref} className="inline-flex items-center gap-1.5 text-sm font-bold text-muted-foreground transition-colors hover:text-foreground">
                    See all <ArrowRight size={15} aria-hidden="true" />
                </Link>
            </div>
            <ThumbGrid items={items} loading={related.isPending} count={COUNT} />
        </section>
    );
};

export default RelatedImages;
