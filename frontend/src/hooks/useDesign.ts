import { useSyncExternalStore } from 'react';
import { DESIGN_SWITCHED_EVENT, Design, currentDesign } from '../utils/design';

const subscribe = (onChange: () => void) => {
    window.addEventListener(DESIGN_SWITCHED_EVENT, onChange);
    return () => window.removeEventListener(DESIGN_SWITCHED_EVENT, onChange);
};

const serverDesign = (): Design => 'collector';

export const useDesign = () => useSyncExternalStore(subscribe, currentDesign, serverDesign);
