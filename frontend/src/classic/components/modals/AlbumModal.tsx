import { useForm } from 'react-hook-form';
import Modal from '../Modal';
import { useEffect, useId } from 'react';

export interface AlbumFormData {
    name: string;
    description?: string;
}

interface AlbumModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSubmit: (data: AlbumFormData) => void;
    initialData?: AlbumFormData;
    title?: string;
    submitLabel?: string;
}

const AlbumModal = ({ isOpen, onClose, onSubmit, initialData, title = 'Create Album', submitLabel = 'Create' }: AlbumModalProps) => {
    const { register, handleSubmit, reset, formState: { errors } } = useForm<AlbumFormData>();
    const id = useId();

    useEffect(() => {
        if (isOpen) {
            reset(initialData || { name: '', description: '' });
        }
    }, [isOpen, initialData, reset]);

    return (
        <Modal isOpen={isOpen} onClose={onClose} title={title} maxWidth="max-w-md">
            <form onSubmit={handleSubmit(onSubmit)} className="space-y-5">
                <div>
                    <label htmlFor={`${id}-name`} className="field-label">Name</label>
                    <input
                        id={`${id}-name`}
                        {...register('name', { required: 'Name is required' })}
                        aria-invalid={!!errors.name}
                        aria-describedby={errors.name ? `${id}-name-error` : undefined}
                        className="input"
                        placeholder="e.g. Rainy evenings"
                        data-autofocus
                    />
                    {errors.name && <p id={`${id}-name-error`} className="field-error">{errors.name.message}</p>}
                </div>

                <div>
                    <label htmlFor={`${id}-description`} className="field-label">Description</label>
                    <textarea
                        id={`${id}-description`}
                        {...register('description')}
                        className="input min-h-24 resize-y"
                        placeholder="Optional"
                    />
                </div>

                <div className="flex justify-end gap-2 pt-1">
                    <button type="button" onClick={onClose} className="btn btn-secondary">Cancel</button>
                    <button type="submit" className="btn btn-primary">{submitLabel}</button>
                </div>
            </form>
        </Modal>
    );
};

export default AlbumModal;
