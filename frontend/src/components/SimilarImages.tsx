import { useQuery } from '@tanstack/react-query';
import { ScanSearch } from 'lucide-react';
import api from '../services/api';
import { ImageDto, NsfwMode, PaginatedList } from '../types';
import ThumbGrid from './ThumbGrid';

const COUNT = 6;

interface SimilarImagesProps {
    image: ImageDto;
    allowNsfw: boolean;
}

const SimilarImages = ({ image, allowNsfw }: SimilarImagesProps) => {
    const isNsfw = allowNsfw ? NsfwMode.All : NsfwMode.False;
    const similar = useQuery({
        queryKey: ['similar', image.id, isNsfw],
        staleTime: 10 * 60_000,
        retry: false,
        queryFn: async ({ signal }) => {
            const { data } = await api.get<PaginatedList<ImageDto>>(`/images/${image.id}/similar`, {
                params: { limit: COUNT, maxDistance: 20, isNsfw },
                skipGlobalErrorHandler: true,
                signal,
            });
            return data.items.filter((item) => item.id !== image.id).slice(0, COUNT);
        },
    });

    if (similar.isError || (similar.isSuccess && similar.data.length === 0)) return null;

    return (
        <section aria-labelledby="similar-images" className="mt-12 border-t-2 border-dashed border-border pt-8">
            <div className="mb-5">
                <h2 id="similar-images" className="flex items-center gap-2 text-xl font-black">
                    <ScanSearch size={19} aria-hidden="true" className="text-muted-foreground" /> Looks alike
                </h2>
                <p className="mt-1 text-sm text-muted-foreground">Compared by the picture itself, so edits, recolors and reposts show up too.</p>
            </div>
            <ThumbGrid items={similar.data ?? []} loading={similar.isPending} count={COUNT} />
        </section>
    );
};

export default SimilarImages;
