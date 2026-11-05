import { useState, useEffect } from 'react';
import { RefreshCw } from 'lucide-react';

interface FloatingRefreshButtonProps {
    onRefresh: () => void;
}

const FloatingRefreshButton = ({ onRefresh }: FloatingRefreshButtonProps) => {
    const [bottomOffset, setBottomOffset] = useState(24);

    useEffect(() => {
        const vv = window.visualViewport;
        if (!vv) return;

        const update = () => {
            const toolbarHeight = window.innerHeight - vv.height - vv.offsetTop;
            setBottomOffset(Math.max(24, toolbarHeight + 24));
        };

        vv.addEventListener('resize', update);
        vv.addEventListener('scroll', update);
        update();

        return () => {
            vv.removeEventListener('resize', update);
            vv.removeEventListener('scroll', update);
        };
    }, []);

    return (
        <button
            type="button"
            onClick={onRefresh}
            className="fixed right-5 z-40 grid size-12 place-items-center rounded-full bg-primary text-primary-foreground shadow-float transition-[transform,background-color] hover:bg-primary/90 active:scale-95 sm:right-6"
            style={{ bottom: `${bottomOffset}px` }}
            title="Refresh"
            aria-label="Refresh images"
        >
            <RefreshCw size={20} aria-hidden="true" />
        </button>
    );
};

export default FloatingRefreshButton;
