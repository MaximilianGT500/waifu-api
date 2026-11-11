import { Link } from 'react-router';
import { History, X } from 'lucide-react';
import { useRecentImages } from '../hooks/useRecentImages';
import { clearRecentImages, removeRecentImage } from '../utils/recentImages';
import { imgCard } from '../utils/cfImage';
import { readLocal } from '../utils/storage';
import { saveBrowseContext } from '../utils/browseContext';

const SHOWN = 12;

const RecentlyViewed = () => {
    const recent = useRecentImages();
    const nsfwAllowed = readLocal('nsfw-consent') === 'true';
    const items = recent.filter((item) => nsfwAllowed || !item.isNsfw).slice(0, SHOWN);
    if (items.length === 0) return null;

    const remember = () => saveBrowseContext({ ids: items.map((item) => item.id), from: '/' });

    return (
        <section aria-labelledby="recently-viewed" className="mt-14">
            <div className="mb-4 flex items-center justify-between gap-3">
                <h2 id="recently-viewed" className="flex items-center gap-2 text-xl font-black">
                    <History size={19} aria-hidden="true" className="text-muted-foreground" /> Recently viewed
                </h2>
                <button type="button" onClick={clearRecentImages} className="btn btn-ghost btn-sm">Clear</button>
            </div>
            <ul className="-mx-1 flex snap-x gap-3 overflow-x-auto overscroll-x-contain px-1 pb-3 pt-1 [scrollbar-width:thin]">
                {items.map((item) => (
                    <li key={item.id} className="group relative w-24 shrink-0 snap-start sm:w-28">
                        <Link
                            to={`/images/${item.id}`}
                            onClick={remember}
                            data-nsfw={item.isNsfw ? '' : undefined}
                            className="block rounded-[0.9rem] bg-card p-1 shadow-card transition-shadow hover:shadow-lift dark:ring-1 dark:ring-white/6"
                        >
                            <span className="block aspect-[5/7] overflow-hidden rounded-[0.65rem]" style={{ backgroundColor: item.dominantColor ?? undefined }}>
                                <img
                                    src={imgCard(item.url)}
                                    alt={`Artwork #${item.id}`}
                                    width={item.width || undefined}
                                    height={item.height || undefined}
                                    loading="lazy"
                                    decoding="async"
                                    className="size-full object-cover object-[50%_28%]"
                                />
                            </span>
                            <span className="block px-1 pb-0.5 pt-1 text-xs font-black tabular">#{item.id}</span>
                        </Link>
                        <button
                            type="button"
                            onClick={() => removeRecentImage(item.id)}
                            aria-label={`Remove #${item.id} from recently viewed`}
                            title="Remove"
                            className="absolute right-2 top-2 grid size-6 place-items-center rounded-full bg-zinc-950/60 text-white opacity-0 backdrop-blur-sm transition-opacity focus-visible:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100"
                        >
                            <X size={13} aria-hidden="true" />
                        </button>
                    </li>
                ))}
            </ul>
        </section>
    );
};

export default RecentlyViewed;
