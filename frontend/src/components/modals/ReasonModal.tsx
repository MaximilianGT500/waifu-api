import { useState, useEffect, useId, useRef } from 'react';
import { Loader2 } from 'lucide-react';
import Modal from '../Modal';

interface ReasonModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSubmit: (reason: string) => void | Promise<unknown>;
    title: string;
    description: string;
    placeholder?: string;
    submitText?: string;
    required?: boolean;
    maxLength?: number;
    initialValue?: string;
    confirmVariant?: 'primary' | 'destructive';
}

const ReasonModal = ({ isOpen, onClose, onSubmit, title, description, placeholder = "Reason…", submitText = "Submit", required = true, maxLength = 500, initialValue = '', confirmVariant = 'destructive' }: ReasonModalProps) => {
    const [reason, setReason] = useState(initialValue);
    const [isPending, setIsPending] = useState(false);
    const pendingRef = useRef(false);
    const mounted = useRef(true);
    const descriptionId = useId();
    const counterId = useId();

    useEffect(() => {
        mounted.current = true;
        return () => { mounted.current = false; };
    }, []);

    const [synced, setSynced] = useState({ isOpen, initialValue });
    if (synced.isOpen !== isOpen || synced.initialValue !== initialValue) {
        setSynced({ isOpen, initialValue });
        if (isOpen) setReason(initialValue);
        else setIsPending(false);
    }

    useEffect(() => {
        if (!isOpen) pendingRef.current = false;
    }, [isOpen]);

    const length = [...reason].length;
    const isDisabled = (required && !reason.trim()) || isPending;

    const handleSubmit = async () => {
        if (pendingRef.current || (required && !reason.trim())) return;
        pendingRef.current = true;
        setIsPending(true);
        try {
            await onSubmit(reason.trim());
        } finally {
            pendingRef.current = false;
            if (mounted.current) setIsPending(false);
        }
    };

    return (
        <Modal isOpen={isOpen} onClose={isPending ? () => {} : onClose} title={title}>
            <form
                className="space-y-4"
                onSubmit={(e) => {
                    e.preventDefault();
                    handleSubmit();
                }}
            >
                <p id={descriptionId} className="text-sm text-muted-foreground">{description}</p>
                <div>
                    <textarea
                        value={reason}
                        onChange={(e) => setReason([...e.target.value].slice(0, maxLength).join(''))}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                                e.preventDefault();
                                handleSubmit();
                            }
                        }}
                        placeholder={placeholder}
                        aria-label={title}
                        aria-describedby={`${descriptionId} ${counterId}`}
                        aria-required={required}
                        disabled={isPending}
                        data-autofocus
                        className="input min-h-28 resize-y"
                    />
                    <p id={counterId} className="field-hint text-right tabular">
                        {length}/{maxLength}
                    </p>
                </div>
                <div className="flex justify-end gap-2">
                    <button type="button" onClick={onClose} disabled={isPending} className="btn btn-secondary">Cancel</button>
                    <button
                        type="submit"
                        disabled={isDisabled}
                        aria-busy={isPending}
                        className={`btn ${confirmVariant === 'primary' ? 'btn-primary' : 'btn-danger'}`}
                    >
                        {isPending && <Loader2 size={16} className="animate-spin" aria-hidden="true" />}
                        {submitText}
                    </button>
                </div>
            </form>
        </Modal>
    );
};

export default ReasonModal;
