import { useSyncExternalStore } from 'react';
import { getSavedSearches, SavedSearch, subscribeSavedSearches } from '../utils/savedSearches';

const EMPTY: SavedSearch[] = [];
const serverSnapshot = () => EMPTY;

export const useSavedSearches = () => useSyncExternalStore(subscribeSavedSearches, getSavedSearches, serverSnapshot);
