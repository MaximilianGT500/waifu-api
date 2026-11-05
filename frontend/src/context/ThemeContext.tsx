import React, { createContext, useContext, useEffect, useMemo, useState } from 'react';
import { readLocal, writeLocal } from '../utils/storage';
import { applyThemeColor } from '../utils/design';
import { MOTION_KEY, MotionPreference, applyMotion, parseMotionPreference, systemPrefersReducedMotion, watchSystemMotion } from '../utils/motion';
import { ACCENT_KEY, Accent, applyAccent, parseAccent } from '../utils/accent';

type Theme = 'light' | 'dark' | 'system';

interface ThemeContextType {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  resolvedTheme: 'light' | 'dark';
  motionEnabled: boolean;
  setMotionEnabled: (enabled: boolean) => void;
  accent: Accent;
  setAccent: (accent: Accent) => void;
}

const ThemeContext = createContext<ThemeContextType | null>(null);

export const ThemeProvider = ({ children }: { children: React.ReactNode }) => {
  const [theme, setThemeState] = useState<Theme>(() => {
    const saved = readLocal('theme');
    return (saved === 'light' || saved === 'dark' || saved === 'system') ? (saved as Theme) : 'system';
  });

  const [resolvedTheme, setResolvedTheme] = useState<'light' | 'dark'>(() =>
    document.documentElement.classList.contains('dark') ? 'dark' : 'light'
  );

  const [motion, setMotion] = useState<MotionPreference>(() => parseMotionPreference(readLocal(MOTION_KEY)));
  const [systemReduced, setSystemReduced] = useState(systemPrefersReducedMotion);
  const motionEnabled = motion === 'system' ? !systemReduced : motion === 'on';

  useEffect(() => watchSystemMotion(setSystemReduced), []);

  const [accent, setAccentState] = useState<Accent>(() => parseAccent(readLocal(ACCENT_KEY)));

  useEffect(() => {
    applyAccent(accent);
  }, [accent]);

  useEffect(() => {
    applyMotion(motionEnabled);
  }, [motionEnabled]);

  useEffect(() => {
    const root = window.document.documentElement;
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');

    const applyTheme = () => {
      let targetTheme = theme;
      if (theme === 'system') {
        targetTheme = mediaQuery.matches ? 'dark' : 'light';
      }

      setResolvedTheme(targetTheme as 'light' | 'dark');

      root.classList.toggle('dark', targetTheme === 'dark');
      applyThemeColor(targetTheme === 'dark');
    };

    applyTheme();

    const listener = () => {
      if (theme === 'system') applyTheme();
    };

    mediaQuery.addEventListener('change', listener);
    writeLocal('theme', theme);

    return () => mediaQuery.removeEventListener('change', listener);
  }, [theme]);

  const value = useMemo(() => ({
    theme,
    setTheme: setThemeState,
    resolvedTheme,
    motionEnabled,
    setMotionEnabled: (enabled: boolean) => {
      const next = enabled ? 'on' : 'off';
      writeLocal(MOTION_KEY, next);
      setMotion(next);
    },
    accent,
    setAccent: (next: Accent) => {
      writeLocal(ACCENT_KEY, next);
      setAccentState(next);
    },
  }), [theme, resolvedTheme, motionEnabled, accent]);

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  );
};

export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (!context) throw new Error('useTheme must be used within a ThemeProvider');
  return context;
};
