import { createContext, useContext, useState, ReactNode, useCallback, useMemo, useRef, ComponentType } from 'react';
import Notification, { NotificationType } from '../components/Notification';

interface NotificationItem {
  id: string;
  type: NotificationType;
  message: string;
}

interface NotificationContextType {
  showNotification: (type: NotificationType, message: string) => void;
}

export interface ToastProps {
  id: string;
  type: NotificationType;
  message: string;
  onClose: (id: string) => void;
}

const NotificationContext = createContext<NotificationContextType | undefined>(undefined);

const MAX_NOTIFICATIONS = 4;
const DEDUPE_WINDOW_MS = 3000;

export const NotificationProvider = ({ children, toast: Toast = Notification }: { children: ReactNode; toast?: ComponentType<ToastProps> }) => {
  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const recent = useRef(new Map<string, number>());
  const counter = useRef(0);

  const showNotification = useCallback((type: NotificationType, message: string) => {
    const text = String(message ?? '').trim();
    if (!text) return;
    const key = `${type}:${text}`;
    const now = Date.now();
    const last = recent.current.get(key);
    if (last !== undefined && now - last < DEDUPE_WINDOW_MS) return;
    recent.current.set(key, now);
    if (recent.current.size > 50) {
      for (const [k, t] of recent.current) {
        if (now - t >= DEDUPE_WINDOW_MS) recent.current.delete(k);
      }
    }

    counter.current += 1;
    const id = `${now.toString(36)}-${counter.current}`;
    setNotifications(prev => [...prev, { id, type, message: text }].slice(-MAX_NOTIFICATIONS));
  }, []);

  const removeNotification = useCallback((id: string) => {
    setNotifications(prev => prev.filter(n => n.id !== id));
  }, []);

  const value = useMemo(() => ({ showNotification }), [showNotification]);

  return (
      <NotificationContext.Provider value={value}>
        {children}
        <section
          className="pointer-events-none fixed bottom-4 left-4 right-4 z-100 flex flex-col items-end gap-2 sm:left-auto"
          aria-label="Notifications"
        >
          {notifications.map(notification => (
              <div key={notification.id} className="pointer-events-auto w-full sm:w-auto">
                <Toast
                    id={notification.id}
                    type={notification.type}
                    message={notification.message}
                    onClose={removeNotification}
                />
              </div>
          ))}
        </section>
      </NotificationContext.Provider>
  );
};

export const useNotification = () => {
  const context = useContext(NotificationContext);
  if (context === undefined) {
    throw new Error('useNotification must be used within a NotificationProvider');
  }
  return context;
};
