import { useSyncExternalStore } from 'react';
import { getBrowseContext, subscribeBrowseContext } from '../utils/browseContext';

const serverSnapshot = () => null;

export const useBrowseContext = () => useSyncExternalStore(subscribeBrowseContext, getBrowseContext, serverSnapshot);
