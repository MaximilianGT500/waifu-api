export function StatusTabs<T extends string>({ value, onChange, options, label = 'Filter by status' }: {
    value: T;
    onChange: (v: T) => void;
    options: { value: T; label: string }[];
    label?: string;
}) {
    return (
        <div role="group" aria-label={label} className="flex w-fit max-w-full overflow-x-auto rounded-xl bg-secondary p-1">
            {options.map(o => (
                <button
                    key={o.value}
                    type="button"
                    aria-pressed={value === o.value}
                    onClick={() => onChange(o.value)}
                    className={`shrink-0 whitespace-nowrap rounded-lg px-3.5 py-1.5 text-sm font-semibold transition-colors ${value === o.value ? 'bg-card text-foreground shadow-[0_1px_2px_rgb(0_0_0/0.08)]' : 'text-muted-foreground hover:text-foreground'}`}
                >
                    {o.label}
                </button>
            ))}
        </div>
    );
}

export default StatusTabs;
