import { SlidersHorizontal } from 'lucide-react';

interface FilterToggleButtonProps {
    isOpen: boolean;
    onToggle: () => void;
    className?: string;
}

const FilterToggleButton = ({ isOpen, onToggle, className = '' }: FilterToggleButtonProps) => (
    <button
        type="button"
        onClick={onToggle}
        aria-expanded={isOpen}
        aria-label={isOpen ? 'Hide filters' : 'Show filters'}
        className={`btn ${isOpen ? 'bg-brand-soft text-brand-ink hover:bg-brand-soft' : 'btn-secondary'} ${className}`}
    >
        <SlidersHorizontal size={16} aria-hidden="true" />
        <span className="hidden sm:inline">Filters</span>
    </button>
);

export default FilterToggleButton;
