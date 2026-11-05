import { useEffect, useRef } from 'react';
import { ShieldAlert } from 'lucide-react';

interface NsfwWarningProps {
    variant: 'overlay' | 'modal';
    onConsent: () => void;
    onDecline: () => void;
}

const NsfwWarning = ({ variant, onConsent, onDecline }: NsfwWarningProps) => {
    const consentRef = useRef<HTMLButtonElement>(null);

    useEffect(() => {
        if (variant === 'modal') consentRef.current?.focus({ preventScroll: true });
    }, [variant]);

    if (variant === 'overlay') {
        return (
            <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/90 p-6 backdrop-blur-2xl">
                <div className="max-w-sm text-center">
                    <span className="mx-auto mb-4 grid size-14 place-items-center rounded-2xl bg-destructive/12 text-destructive" aria-hidden="true">
                        <ShieldAlert size={28} strokeWidth={1.8} />
                    </span>
                    <h2 className="text-2xl font-extrabold tracking-[-0.02em]">NSFW Content</h2>
                    <p className="mt-2 text-muted-foreground">This image is marked as 18+.</p>
                    <div className="mt-6 flex justify-center gap-3">
                        <button type="button" onClick={onConsent} className="btn btn-primary">Show Content</button>
                        <button type="button" onClick={onDecline} className="btn btn-secondary">Go Back</button>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="fixed inset-0 z-60 flex items-center justify-center bg-zinc-950/85 p-4 backdrop-blur-xl">
            <div role="alertdialog" aria-modal="true" aria-labelledby="nsfw-title" aria-describedby="nsfw-text" className="w-full max-w-sm rounded-[1.25rem] border border-border bg-card p-7 text-center shadow-float">
                <span className="mx-auto mb-4 grid size-14 place-items-center rounded-2xl bg-destructive/12 text-destructive" aria-hidden="true">
                    <ShieldAlert size={28} strokeWidth={1.8} />
                </span>
                <h2 id="nsfw-title" className="text-2xl font-extrabold tracking-[-0.02em]">Age Restricted</h2>
                <p id="nsfw-text" className="mt-2 text-muted-foreground">This content is intended for adults only.</p>
                <div className="mt-6 grid gap-2">
                    <button ref={consentRef} type="button" onClick={onConsent} className="btn btn-primary btn-lg">I am 18+</button>
                    <button type="button" onClick={onDecline} className="btn btn-ghost btn-lg">Go Back</button>
                </div>
            </div>
        </div>
    );
};

export default NsfwWarning;
