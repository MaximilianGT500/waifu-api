const HEX_COLOR = /^#(?:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$/i;

export function safeColor(value: string | null | undefined): string | null {
    if (!value) return null;
    const trimmed = value.trim();
    return HEX_COLOR.test(trimmed) ? trimmed.toLowerCase() : null;
}
