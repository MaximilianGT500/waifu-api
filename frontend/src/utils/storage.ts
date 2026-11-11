export function readLocal(key: string): string | null {
    try {
        return localStorage.getItem(key);
    } catch {
        return null;
    }
}

export function writeLocal(key: string, value: string): void {
    try {
        localStorage.setItem(key, value);
    } catch {
    }
}

export function removeLocal(key: string): void {
    try {
        localStorage.removeItem(key);
    } catch {
    }
}
