import React from 'react';
import { Search, Hash } from 'lucide-react';

interface SearchInputProps {
    value: string;
    onChange: (value: string) => void;
    searchType: 'name' | 'id';
    onSearchTypeChange: (type: 'name' | 'id') => void;
    className?: string;
}

const SearchInput: React.FC<SearchInputProps> = ({
    value,
    onChange,
    searchType,
    onSearchTypeChange,
    className = 'w-full md:w-72',
}) => {
    const handleToggle = () => {
        const nextType = searchType === 'name' ? 'id' : 'name';
        if (nextType === 'id' && value && !/^\d+$/.test(value)) {
            onChange('');
        }
        onSearchTypeChange(nextType);
    };

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const val = e.target.value;
        if (searchType === 'id' && val && !/^\d*$/.test(val)) {
            return;
        }
        onChange(val);
    };

    return (
        <div className={`group relative flex items-center rounded-full border-[1.5px] border-input bg-card transition-[border-color,box-shadow] hover:border-foreground/60 focus-within:border-brand focus-within:ring-4 focus-within:ring-brand/15 ${className}`}>
            <span className="pointer-events-none absolute left-3.5 text-muted-foreground" aria-hidden="true">
                {searchType === 'id' ? <Hash size={17} /> : <Search size={17} />}
            </span>

            <input
                type="text"
                inputMode={searchType === 'id' ? 'numeric' : 'search'}
                placeholder={searchType === 'id' ? 'Search by ID' : 'Search by name'}
                aria-label={searchType === 'id' ? 'Search by ID' : 'Search by name'}
                value={value}
                onChange={handleChange}
                className="w-full min-w-0 bg-transparent py-2.5 pl-10 pr-2 text-sm outline-none placeholder:text-muted-foreground/70"
            />

            <button
                type="button"
                onClick={handleToggle}
                aria-label={`Switch to ${searchType === 'name' ? 'ID' : 'name'} search`}
                title={`Switch to ${searchType === 'name' ? 'ID' : 'name'} search`}
                className="mr-1 shrink-0 rounded-full bg-secondary px-3 py-1.5 text-xs font-bold text-muted-foreground transition-colors hover:bg-sleeve hover:text-foreground"
            >
                {searchType === 'name' ? 'Name' : 'ID'}
            </button>
        </div>
    );
};

export default SearchInput;
