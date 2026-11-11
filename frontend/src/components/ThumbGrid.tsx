import { Link, useLocation } from 'react-router';
import { ImageDto } from '../types';
import { imgCard, imgCardSrcSet } from '../utils/cfImage';
import { safeColor } from '../utils/color';
import { saveBrowseContext } from '../utils/browseContext';

const SIZES = '(min-width: 1280px) 11rem, (min-width: 640px) 16vw, 31vw';

const describe = (item: ImageDto) => {
    const names = (item.artists ?? []).map((a) => a.name).join(', ');
    return names ? `Artwork #${item.id} by ${names}` : `Artwork #${item.id}`;
};

interface ThumbGridProps {
    items: ImageDto[];
    loading?: boolean;
    count: number;
    caption?: (item: ImageDto) => string | null;
}

const ThumbGrid = ({ items, loading = false, count, caption }: ThumbGridProps) => {
    const location = useLocation();
    const remember = () => saveBrowseContext({ ids: items.map((i) => i.id), from: `${location.pathname}${location.search}` });

    return (
        <div className="grid grid-cols-3 gap-3 sm:grid-cols-6">
            {loading
                ? Array.from({ length: count }, (_, i) => (
                    <div key={i} className="aspect-[5/7] animate-pulse rounded-[1rem] bg-sleeve" aria-hidden="true" />
                ))
                : items.map((item) => {
                    const note = caption?.(item);
                    return (
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
                            <span className="flex items-baseline justify-between gap-1 px-1.5 pb-0.5 pt-1.5 text-xs">
                                <span className="font-black tabular">#{item.id}</span>
                                {note && <span className="truncate text-muted-foreground tabular">{note}</span>}
                            </span>
                        </Link>
                    );
                })}
        </div>
    );
};

export default ThumbGrid;
