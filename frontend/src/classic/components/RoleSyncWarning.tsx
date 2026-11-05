import { useEffect } from 'react';
import { useAuth } from '../../context/AuthContext';
import { useNotification } from '../../context/NotificationContext';
import { readLocal, writeLocal } from '../../utils/storage';

const NOTICE_KEY = 'role_change_notice';

const ROLE_LABELS: Record<string, string> = {
  User: 'User',
  TrustedUser: 'Trusted User',
  Moderator: 'Moderator',
  Admin: 'Admin',
};

export const RoleSyncWarning = () => {
  const { roleOutOfSync, user } = useAuth();
  const { showNotification } = useNotification();

  useEffect(() => {
    if (!roleOutOfSync || !user) return;
    const marker = `${user.id}:${user.role}`;
    if (readLocal(NOTICE_KEY) === marker) return;
    writeLocal(NOTICE_KEY, marker);
    showNotification('info', `Your role has been changed to ${ROLE_LABELS[user.role] ?? user.role}. The new permissions are already active.`);
  }, [roleOutOfSync, user, showNotification]);

  return null;
};
