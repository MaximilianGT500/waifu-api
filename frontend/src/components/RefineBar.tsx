import { useQueryClient } from '@tanstack/react-query';
import { Plus } from 'lucide-react';
import { rememberLabel } from '../hooks/useFilterLabels';
import { facetParams, useFacets } from '../hooks/useFacets';

type SetSearchParams = (params: URLSearchParams | ((prev: URLSearchParams) => URLSearchParams)) => void;

interface RefineBarProps {
    searchParams: URLSearchParams;
    setSearchParams: SetSearchParams;
    fixed?: Record<string, string>;
}

const RefineBar = ({ searchParams, setSearchParams, fixed }: RefineBarProps) => {
    const queryClient = useQueryClient();
    const facets = useFacets(facetParams(searchParams, fixed));
    const total = facets.data?.totalCount ?? 0;
    const tags = (facets.data?.tags ?? []).filter((t) => t.slug && t.count < total);
    const artists = (facets.data?.artists ?? []).filter((a) => a.count < total).slice(0, 4);
    if (total < 2 || tags.length + artists.length === 0) return null;

    const add = (key: 'includedTags' | 'includedArtists', value: string) => setSearchParams((prev) => {
        if (!prev.getAll(key).includes(value)) prev.append(key, value);
        prev.delete('page');
        return prev;
    });

    const chip = 'sticker shrink-0 gap-1.5 py-1 pl-2 pr-2.5 text-xs transition-[background-color,rotate] duration-200 hover:-rotate-1 hover:bg-secondary motion-reduce:hover:rotate-0';

    return (
        <div role="region" aria-label="Refine the results" className={`-mx-1 mb-5 flex items-center gap-2 overflow-x-auto px-1 pb-1 pt-0.5 [scrollbar-width:none] ${facets.isPlaceholderData ? 'opacity-60' : ''}`}>
            <span className="shrink-0 text-xs font-bold text-muted-foreground">Narrow down</span>
            {tags.map((tag) => (
                <button
                    key={`t-${tag.id}`}
                    type="button"
                    title={`Only images tagged ${tag.name}`}
                    onClick={() => {
                        rememberLabel(queryClient, 'tag', tag.slug!, { id: tag.id, name: tag.name, slug: tag.slug });
                        add('includedTags', tag.slug!);
                    }}
                    className={chip}
                >
                    <Plus size={12} aria-hidden="true" className="text-muted-foreground" />
                    {tag.name}
                    <span className="text-muted-foreground tabular">{tag.count.toLocaleString()}</span>
                </button>
            ))}
            {artists.map((artist) => (
                <button
                    key={`a-${artist.id}`}
                    type="button"
                    title={`Only images by ${artist.name}`}
                    onClick={() => {
                        rememberLabel(queryClient, 'artist', String(artist.id), { id: artist.id, name: artist.name });
                        add('includedArtists', String(artist.id));
                    }}
                    className={chip}
                >
                    <Plus size={12} aria-hidden="true" className="text-muted-foreground" />
                    by {artist.name}
                    <span className="text-muted-foreground tabular">{artist.count.toLocaleString()}</span>
                </button>
            ))}
        </div>
    );
};

export default RefineBar;
