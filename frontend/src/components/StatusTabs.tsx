export function StatusTabs<T extends string>({ value, onChange, options, label = 'Filter by status' }: {
    value: T;
    onChange: (v: T) => void;
    options: { value: T; label: string }[];
    label?: string;
}) {
    return (
        <div role="group" aria-label={label} className="flex w-fit max-w-full overflow-x-auto rounded-full bg-sleeve p-1 shadow-[inset_0_1px_2px_rgb(9_9_11/0.1)]">
            {options.map(o => (
                <button
                    key={o.value}
                    type="button"
                    aria-pressed={value === o.value}
                    onClick={() => onChange(o.value)}
                    className={`shrink-0 whitespace-nowrap rounded-full px-4 py-1.5 text-sm font-bold transition-colors ${value === o.value ? 'bg-card text-foreground shadow-card' : 'text-muted-foreground hover:text-foreground'}`}
                >
                    {o.label}
                </button>
            ))}
        </div>
    );
}

export default StatusTabs;
