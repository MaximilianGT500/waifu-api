import { useEffect, useRef, useState } from 'react';
import { X, CircleCheck, CircleAlert, Info, TriangleAlert } from 'lucide-react';

export type NotificationType = 'success' | 'error' | 'info' | 'warning';

export interface NotificationProps {
  id: string;
  type: NotificationType;
  message: string;
  onClose: (id: string) => void;
  duration?: number;
}

const DEFAULT_DURATION: Record<NotificationType, number> = {
  success: 4000,
  info: 6000,
  warning: 7000,
  error: 8000,
};

const tones: Record<NotificationType, { icon: typeof Info; className: string }> = {
  success: { icon: CircleCheck, className: 'bg-success/12 text-success' },
  error: { icon: CircleAlert, className: 'bg-destructive/12 text-destructive' },
  warning: { icon: TriangleAlert, className: 'bg-warning/14 text-warning' },
  info: { icon: Info, className: 'bg-info/12 text-info' },
};

const Notification = ({ id, type, message, onClose, duration }: NotificationProps) => {
  const [paused, setPaused] = useState(false);
  const remaining = useRef(duration ?? DEFAULT_DURATION[type]);

  useEffect(() => {
    if (paused) return;
    const startedAt = Date.now();
    const timer = window.setTimeout(() => onClose(id), remaining.current);
    return () => {
      window.clearTimeout(timer);
      remaining.current = Math.max(1000, remaining.current - (Date.now() - startedAt));
    };
  }, [id, onClose, paused]);

  const isUrgent = type === 'error' || type === 'warning';
  const { icon: Icon, className } = tones[type];

  return (
    <div
      role={isUrgent ? 'alert' : 'status'}
      aria-live={isUrgent ? 'assertive' : 'polite'}
      aria-atomic="true"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      className="flex w-full items-start gap-3 rounded-[1.25rem] bg-popover p-3 pr-2.5 text-popover-foreground shadow-float ring-1 ring-border animate-in fade-in slide-in-from-bottom-4 duration-300 sm:w-[22rem] dark:ring-white/8"
    >
      <span className={`grid size-8 shrink-0 place-items-center rounded-full ${className}`} aria-hidden="true">
        <Icon size={17} strokeWidth={2.2} />
      </span>
      <p className="min-w-0 flex-1 self-center py-0.5 text-sm font-bold leading-snug wrap-break-word">{message}</p>
      <button
        type="button"
        onClick={() => onClose(id)}
        aria-label="Dismiss notification"
        className="grid size-7 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
      >
        <X size={15} aria-hidden="true" />
      </button>
    </div>
  );
};

export default Notification;
