import { useState, useEffect, useCallback } from 'react';
import { readLocal, writeLocal } from '../utils/storage';

const NSFW_CONSENT_KEY = 'nsfw-consent';

export const useNsfwConsent = (shouldCheck: boolean = true) => {
    const [showWarning, setShowWarning] = useState(false);

    useEffect(() => {
        if (shouldCheck && !readLocal(NSFW_CONSENT_KEY)) {
            setShowWarning(true);
        }
    }, [shouldCheck]);

    const grantConsent = useCallback(() => {
        writeLocal(NSFW_CONSENT_KEY, 'true');
        setShowWarning(false);
    }, []);

    const dismissWarning = useCallback(() => {
        setShowWarning(false);
    }, []);

    const hasConsent = useCallback(() => {
        return !!readLocal(NSFW_CONSENT_KEY);
    }, []);

    return {
        showWarning,
        setShowWarning,
        grantConsent,
        dismissWarning,
        hasConsent
    };
};
