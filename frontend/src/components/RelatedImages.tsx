import { Link, useLocation } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight } from 'lucide-react';
import api from '../services/api';
import { ImageDto, ImageOrderBy, NsfwMode, PaginatedList, ReviewStatus } from '../types';
import { imgCard, imgCardSrcSet } from '../utils/cfImage';
import { safeColor } from '../utils/color';
import { saveBrowseContext } from '../utils/browseContext';

const COUNT = 6;

const describe = (item: ImageDto) => {
    const names = (item.artists ?? []).map((a) => a.name).join(', ');
    return names ? `Artwork #${item.id} by ${names}` : `Artwork #${item.id}`;
};
const SIZES = '(min-width: 1280px) 11rem, (min-width: 640px) 16vw, 31vw';

interface RelatedImagesProps {
    image: ImageDto;
    allowNsfw: boolean;
}

const RelatedImages = ({ image, allowNsfw }: RelatedImagesProps) => {
    const location = useLocation();
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
    const remember = () => saveBrowseContext({ ids: items.map((i) => i.id), from: `${location.pathname}${location.search}` });

    return (
        <section aria-label={source.label} className="mt-14 border-t-2 border-dashed border-border pt-8">
            <div className="mb-5 flex flex-wrap items-baseline justify-between gap-3">
                <h2 className="text-xl font-black">{source.title}</h2>
                <Link to={galleryHref} className="inline-flex items-center gap-1.5 text-sm font-bold text-muted-foreground transition-colors hover:text-foreground">
                    See all <ArrowRight size={15} aria-hidden="true" />
                </Link>
            </div>
            <div className="grid grid-cols-3 gap-3 sm:grid-cols-6">
                {related.isPending
                    ? Array.from({ length: COUNT }, (_, i) => (
                        <div key={i} className="aspect-[5/7] animate-pulse rounded-[1rem] bg-sleeve" aria-hidden="true" />
                    ))
                    : items.map((item) => (
                        <Link
                            key={item.id}
                            to={`/images/${item.id}`}
                            onClick={remember}
                            data-nsfw={item.isNsfw ? '' : undefined}
                            className="group relative block overflow-hidden rounded-[1rem] bg-card p-1 shadow-card transition-shadow hover:shadow-lift dark:ring-1 dark:ring-white/6"
                        >
                            <span className="block aspect-[5/7] overflow-hidden rounded-[0.75rem]" style={{ backgroundColor: safeColor(item.dominantColor) ?? undefined }}>
                                <img
                                    src={imgCard(item.url)}
                                    srcSet={imgCardSrcSet(item.url, item.width)}
                                    sizes={SIZES}
                                    alt={describe(item)}
                                    width={item.width}
                                    height={item.height}
                                    loading="lazy"
                                    decoding="async"
                                    className="size-full object-cover object-[50%_28%] transition-transform duration-500 group-hover:scale-[1.03] motion-reduce:group-hover:scale-100"
                                />
                            </span>
                            <span className="block px-1.5 pb-0.5 pt-1.5 text-xs font-black tabular">#{item.id}</span>
                        </Link>
                    ))}
            </div>
        </section>
    );
};

export default RelatedImages;
