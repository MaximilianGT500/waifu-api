import { Link } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { extractPalette } from '../utils/palette';
import { imgCard } from '../utils/cfImage';
import { NsfwMode } from '../types';

interface ColorPaletteProps {
    url: string;
    allowNsfw: boolean;
}

const ColorPalette = ({ url, allowNsfw }: ColorPaletteProps) => {
    const palette = useQuery({
        queryKey: ['palette', url],
        staleTime: Infinity,
        gcTime: 10 * 60_000,
        retry: false,
        queryFn: () => extractPalette(imgCard(url)),
    });
    if (!palette.data || palette.data.length < 2) return null;

    return (
        <div className="flex min-w-0 items-center justify-between gap-4 py-3">
            <dt className="shrink-0 text-muted-foreground">Palette</dt>
            <dd className="flex min-w-0 flex-wrap justify-end gap-1.5">
                {palette.data.map((hex) => {
                    const params = new URLSearchParams({ color: hex.slice(1) });
                    if (allowNsfw) params.set('isNsfw', NsfwMode.All);
                    return (
                        <Link
                            key={hex}
                            to={`/gallery?${params}`}
                            title={`Find images in ${hex}`}
                            aria-label={`Find images in ${hex}`}
                            style={{ backgroundColor: hex }}
                            className="palette-swatch size-6 rounded-full ring-1 ring-foreground/15 transition-transform hover:scale-110 motion-reduce:hover:scale-100"
                        />
                    );
                })}
            </dd>
        </div>
    );
};

export default ColorPalette;
