import { Link } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight } from 'lucide-react';
import api from '../services/api';
import { PaginatedList, Tag } from '../types';
import Skeleton from './Skeleton';

const COUNT = 14;

const PopularTags = () => {
    const tags = useQuery({
        queryKey: ['popular-tags', COUNT],
        staleTime: 5 * 60_000,
        queryFn: async ({ signal }) => {
            const { data } = await api.get<PaginatedList<Tag>>('/tags', { params: { pageSize: COUNT }, skipGlobalErrorHandler: true, signal });
            return data.items.filter((tag) => (tag.imageCount ?? 0) > 0);
        },
    });

    if (tags.isError || (tags.isSuccess && tags.data.length === 0)) return null;

    return (
        <section aria-labelledby="popular-tags" className="mt-14">
            <div className="mb-4 flex items-baseline justify-between gap-3">
                <h2 id="popular-tags" className="text-xl font-black">Browse by tag</h2>
                <Link to="/tags" className="inline-flex items-center gap-1.5 text-sm font-bold text-muted-foreground transition-colors hover:text-foreground">
                    All tags <ArrowRight size={15} aria-hidden="true" />
                </Link>
            </div>
            <ul className="flex flex-wrap gap-2.5">
                {tags.isPending
                    ? Array.from({ length: 8 }, (_, i) => (
                        <li key={i}><Skeleton className="h-9 rounded-full" style={{ width: `${5 + (i % 4) * 1.5}rem` }} /></li>
                    ))
                    : tags.data.map((tag) => (
                        <li key={tag.id}>
                            <Link
                                to={`/tags/${encodeURIComponent(tag.slug)}`}
                                title={tag.description || `Images tagged ${tag.name}`}
                                className="sticker gap-2 py-1.5 pl-3.5 pr-1.5 text-sm transition-[rotate,background-color] duration-200 hover:-rotate-2 hover:bg-secondary motion-reduce:hover:rotate-0"
                            >
                                {tag.name}
                                <span className="rounded-full bg-sleeve px-2 py-px text-xs text-muted-foreground tabular">{(tag.imageCount ?? 0).toLocaleString()}</span>
                            </Link>
                        </li>
                    ))}
            </ul>
        </section>
    );
};

export default PopularTags;
