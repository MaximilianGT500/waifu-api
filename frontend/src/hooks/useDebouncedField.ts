import { ChangeEvent, KeyboardEvent, useEffect, useRef, useState } from 'react';

export function useDebouncedField(value: string, commit: (next: string) => void, delay = 450) {
    const [draft, setDraft] = useState(value);
    const [synced, setSynced] = useState(value);
    const [committed, setCommitted] = useState<string | null>(null);
    const timer = useRef<number | undefined>(undefined);

    if (synced !== value) {
        setSynced(value);
        setCommitted(null);
        if (value !== committed) setDraft(value);
    }

    useEffect(() => () => window.clearTimeout(timer.current), []);

    const flush = (next: string) => {
        window.clearTimeout(timer.current);
        const trimmed = next.trim();
        if (trimmed === value) return;
        setCommitted(trimmed);
        commit(trimmed);
    };

    return {
        value: draft,
        onChange: (e: ChangeEvent<HTMLInputElement>) => {
            const next = e.target.value;
            setDraft(next);
            window.clearTimeout(timer.current);
            timer.current = window.setTimeout(() => flush(next), delay);
        },
        onBlur: () => flush(draft),
        onKeyDown: (e: KeyboardEvent<HTMLInputElement>) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                flush(draft);
            }
        },
    };
}
