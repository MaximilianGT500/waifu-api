import { FormEvent, useId, useState } from 'react';
import { Link } from 'react-router';
import { Check, Pencil, Trash2, X } from 'lucide-react';
import Modal from '../Modal';
import { useSavedSearches } from '../../hooks/useSavedSearches';
import { removeSavedSearch, renameSavedSearch, savedSearchHref } from '../../utils/savedSearches';

interface SaveSearchDialogProps {
    isOpen: boolean;
    onClose: () => void;
    defaultName: string;
    onSave: (name: string) => void;
}

export const SaveSearchDialog = ({ isOpen, onClose, defaultName, onSave }: SaveSearchDialogProps) => {
    const [name, setName] = useState(defaultName);
    const [synced, setSynced] = useState({ isOpen, defaultName });
    const inputId = useId();

    if (synced.isOpen !== isOpen || synced.defaultName !== defaultName) {
        setSynced({ isOpen, defaultName });
        if (isOpen) setName(defaultName);
    }

    const submit = (e: FormEvent) => {
        e.preventDefault();
        if (!name.trim()) return;
        onSave(name);
    };

    return (
        <Modal
            isOpen={isOpen}
            onClose={onClose}
            title="Save this search"
            footer={
                <div className="flex justify-end gap-2">
                    <button type="button" onClick={onClose} className="btn btn-ghost">Cancel</button>
                    <button type="submit" form={`${inputId}-form`} disabled={!name.trim()} className="btn btn-primary">Save</button>
                </div>
            }
        >
            <form id={`${inputId}-form`} onSubmit={submit}>
                <label htmlFor={inputId} className="field-label">Name</label>
                <input
                    id={inputId}
                    data-autofocus
                    value={name}
                    maxLength={80}
                    onChange={(e) => setName(e.target.value)}
                    className="input"
                    autoComplete="off"
                />
                <p className="field-hint">Saved searches stay in this browser. Open them from the gallery header or the quick search.</p>
            </form>
        </Modal>
    );
};

interface ManageSavedSearchesDialogProps {
    isOpen: boolean;
    onClose: () => void;
}

export const ManageSavedSearchesDialog = ({ isOpen, onClose }: ManageSavedSearchesDialogProps) => {
    const searches = useSavedSearches();
    const [editing, setEditing] = useState<{ id: string; name: string } | null>(null);

    const commit = (e: FormEvent) => {
        e.preventDefault();
        if (editing) renameSavedSearch(editing.id, editing.name);
        setEditing(null);
    };

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Saved searches" maxWidth="max-w-lg">
            {searches.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">No saved searches yet.</p>
            ) : (
                <ul className="divide-y-2 divide-dashed divide-border">
                    {searches.map((search) => (
                        <li key={search.id} className="flex items-center gap-2 py-2.5">
                            {editing?.id === search.id ? (
                                <form onSubmit={commit} className="flex min-w-0 flex-1 items-center gap-2">
                                    <input
                                        value={editing.name}
                                        maxLength={80}
                                        onChange={(e) => setEditing({ id: search.id, name: e.target.value })}
                                        aria-label="New name"
                                        className="input py-1.5"
                                        ref={(el) => el?.focus()}
                                    />
                                    <button type="submit" className="btn btn-secondary btn-icon" aria-label="Save the name"><Check size={15} aria-hidden="true" /></button>
                                    <button type="button" onClick={() => setEditing(null)} className="btn btn-ghost btn-icon" aria-label="Cancel renaming"><X size={15} aria-hidden="true" /></button>
                                </form>
                            ) : (
                                <>
                                    <Link to={savedSearchHref(search)} onClick={onClose} className="min-w-0 flex-1 truncate rounded-full px-2 py-1 text-sm font-bold transition-colors hover:bg-secondary">
                                        {search.name}
                                    </Link>
                                    <button type="button" onClick={() => setEditing({ id: search.id, name: search.name })} className="btn btn-ghost btn-icon" aria-label={`Rename ${search.name}`} title="Rename">
                                        <Pencil size={15} aria-hidden="true" />
                                    </button>
                                    <button type="button" onClick={() => removeSavedSearch(search.id)} className="btn btn-ghost btn-icon hover:text-destructive" aria-label={`Delete ${search.name}`} title="Delete">
                                        <Trash2 size={15} aria-hidden="true" />
                                    </button>
                                </>
                            )}
                        </li>
                    ))}
                </ul>
            )}
        </Modal>
    );
};
