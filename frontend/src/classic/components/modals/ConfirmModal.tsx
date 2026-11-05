import { ReactNode, useEffect, useRef, useState } from 'react';
import Modal from '../Modal';
import { Loader2, Trash2, TriangleAlert, LucideIcon } from 'lucide-react';

interface ConfirmModalProps {
    isOpen: boolean;
    onClose: () => void;
    onConfirm: () => void | Promise<unknown>;
    title: string;
    message: ReactNode;
    confirmText?: string;
    cancelText?: string;
    variant?: 'destructive' | 'warning' | 'info';
    icon?: LucideIcon;
    zIndex?: number;
}

const ConfirmModal = ({
    isOpen,
    onClose,
    onConfirm,
    title,
    message,
    confirmText = 'Confirm',
    cancelText = 'Cancel',
    variant = 'destructive',
    icon: Icon,
    zIndex,
}: ConfirmModalProps) => {
    const [isPending, setIsPending] = useState(false);
    const [wasOpen, setWasOpen] = useState(isOpen);
    const pendingRef = useRef(false);
    const mounted = useRef(true);

    if (wasOpen !== isOpen) {
        setWasOpen(isOpen);
        if (!isOpen) setIsPending(false);
    }

    useEffect(() => {
        mounted.current = true;
        return () => { mounted.current = false; };
    }, []);

    useEffect(() => {
        if (!isOpen) pendingRef.current = false;
    }, [isOpen]);

    const handleConfirm = async () => {
        if (pendingRef.current) return;
        pendingRef.current = true;
        setIsPending(true);
        try {
            await onConfirm();
        } finally {
            pendingRef.current = false;
            if (mounted.current) setIsPending(false);
        }
    };

    const DisplayIcon = Icon || (variant === 'destructive' ? Trash2 : TriangleAlert);

    const colorClass = variant === 'destructive'
        ? 'bg-destructive/10 text-destructive'
        : variant === 'warning'
            ? 'bg-warning/12 text-warning'
            : 'bg-info/10 text-info';

    const buttonClass = variant === 'destructive' ? 'btn-danger' : 'btn-primary';

    return (
        <Modal
            isOpen={isOpen}
            onClose={isPending ? () => {} : onClose}
            title={title}
            zIndex={zIndex}
            footer={
                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                    <button
                        type="button"
                        onClick={onClose}
                        disabled={isPending}
                        data-autofocus
                        className="btn btn-secondary"
                    >
                        {cancelText}
                    </button>
                    <button
                        type="button"
                        onClick={handleConfirm}
                        disabled={isPending}
                        aria-busy={isPending}
                        className={`btn ${buttonClass} disabled:cursor-wait`}
                    >
                        {isPending && <Loader2 size={16} className="animate-spin" aria-hidden="true" />}
                        {confirmText}
                    </button>
                </div>
            }
        >
            <div className="flex items-start gap-4">
                <span className={`grid size-11 shrink-0 place-items-center rounded-2xl ${colorClass}`} aria-hidden="true">
                    <DisplayIcon size={20} />
                </span>
                <div className="min-w-0 pt-1 text-muted-foreground">{message}</div>
            </div>
        </Modal>
    );
};

export default ConfirmModal;
