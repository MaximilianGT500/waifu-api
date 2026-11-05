import { useState } from 'react';
import { Info, CircleCheck, TriangleAlert, OctagonAlert, X } from 'lucide-react';
import { useActiveBanner } from '../../hooks/useBanner';
import { BannerVariant } from '../../types';
import { readLocal, writeLocal } from '../../utils/storage';

const DISMISSED_KEY = 'dismissedBanner';

const bannerStyles: Record<BannerVariant, { className: string; icon: typeof Info }> = {
    [BannerVariant.Info]: { className: 'bg-info/10 text-info', icon: Info },
    [BannerVariant.Success]: { className: 'bg-success/10 text-success', icon: CircleCheck },
    [BannerVariant.Warning]: { className: 'bg-warning/12 text-warning', icon: TriangleAlert },
    [BannerVariant.Danger]: { className: 'bg-destructive/10 text-destructive', icon: OctagonAlert },
};

interface BannerViewProps {
    message: string;
    variant: BannerVariant;
    onDismiss?: () => void;
}

export const BannerView = ({ message, variant, onDismiss }: BannerViewProps) => {
    const { className, icon: Icon } = bannerStyles[variant] ?? bannerStyles[BannerVariant.Info];
    return (
        <div role="status" className={`flex items-start gap-3 border-b border-border px-4 py-3 text-sm font-medium lg:px-6 ${className}`}>
            <Icon size={18} className="mt-0.5 shrink-0" aria-hidden="true" />
            <p className="flex-1 whitespace-pre-line text-foreground wrap-break-word">{message}</p>
            {onDismiss && (
                <button
                    type="button"
                    onClick={onDismiss}
                    aria-label="Dismiss banner"
                    className="-m-1 grid size-7 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-foreground/5 hover:text-foreground"
                >
                    <X size={15} aria-hidden="true" />
                </button>
            )}
        </div>
    );
};

const SiteBanner = () => {
    const { data: banner } = useActiveBanner();
    const [dismissed, setDismissed] = useState(() => readLocal(DISMISSED_KEY));

    if (!banner || dismissed === banner.updatedAt) return null;

    const dismiss = () => {
        writeLocal(DISMISSED_KEY, banner.updatedAt);
        setDismissed(banner.updatedAt);
    };

    return <BannerView message={banner.message} variant={banner.variant} onDismiss={dismiss} />;
};

export default SiteBanner;
