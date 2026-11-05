import { useState, useCallback } from 'react';

const TAG_VIEW_KEY = 'gallery-tag-view';

export const useTagView = () => {
    const [showTags, setShowTags] = useState<boolean>(() => {
        try { return localStorage.getItem(TAG_VIEW_KEY) === 'true'; } catch { return false; }
    });

    const toggle = useCallback(() => {
        setShowTags(prev => {
            const next = !prev;
            try { localStorage.setItem(TAG_VIEW_KEY, String(next)); } catch { }
            return next;
        });
    }, []);

    return { showTags, toggle };
};
