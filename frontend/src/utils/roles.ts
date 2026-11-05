import { Role, User } from '../types';

export const canModerate = (user?: User | null): boolean =>
    !!user && (user.role === Role.Moderator || user.role === Role.Admin);
