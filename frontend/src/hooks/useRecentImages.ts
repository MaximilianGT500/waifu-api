import { useSyncExternalStore } from 'react';
import { getRecentImages, RecentImage, subscribeRecentImages } from '../utils/recentImages';

const EMPTY: RecentImage[] = [];
const serverSnapshot = () => EMPTY;

export const useRecentImages = () => useSyncExternalStore(subscribeRecentImages, getRecentImages, serverSnapshot);
