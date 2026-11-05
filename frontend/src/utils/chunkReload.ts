const KEY = 'chunk_reload_at';
const MIN_INTERVAL_MS = 30_000;

export function isChunkLoadError(error: unknown): boolean {
    const message = error instanceof Error ? `${error.name} ${error.message}` : String(error ?? '');
    return /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module|ChunkLoadError|Unable to preload CSS/i.test(message);
}

export function reloadOnceForNewBuild(): boolean {
    try {
        const last = Number(sessionStorage.getItem(KEY) || 0);
        if (Date.now() - last < MIN_INTERVAL_MS) return false;
        sessionStorage.setItem(KEY, String(Date.now()));
    } catch {
        return false;
    }
    window.location.reload();
    return true;
}
