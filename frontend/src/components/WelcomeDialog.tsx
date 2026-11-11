import { KeyboardEvent, useState } from 'react';
import { ArrowRight, Check, Monitor, Moon, RotateCcw, Sun, Info } from 'lucide-react';
import Modal from './Modal';
import DesignPreview from './DesignPreview';
import { useTheme } from '../context/ThemeContext';
import { ACCENTS } from '../utils/accent';
import { DESIGNS, Design, currentDesign, needsReload, previewDesign, switchDesign } from '../utils/design';
import { markWelcomeDone, requestDesignHint } from '../utils/welcome';
import { getEnv } from '../utils/env';

type Mode = 'light' | 'dark' | 'system';

const MODES: { id: Mode; label: string; icon: typeof Sun }[] = [
    { id: 'light', label: 'Light', icon: Sun },
    { id: 'dark', label: 'Dark', icon: Moon },
    { id: 'system', label: 'System', icon: Monitor },
];

interface WelcomeDialogProps {
    onDone: (changed: boolean) => void;
    revisit?: boolean;
}

const WelcomeDialog = ({ onDone, revisit = false }: WelcomeDialogProps) => {
    const { theme, setTheme, resolvedTheme, accent, setAccent } = useTheme();
    const [initial] = useState(() => ({ design: currentDesign(), theme, accent }));
    const [design, setDesign] = useState<Design>(initial.design);
    const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';
    const accentHex = ACCENTS.find((a) => a.id === accent)?.swatch ?? ACCENTS[0].swatch;
    const dark = resolvedTheme === 'dark';
    const reloads = needsReload(design, initial.design);
    const chosenLabel = DESIGNS.find((d) => d.id === design)?.label ?? 'Collector';

    const pickDesign = (next: Design) => {
        setDesign(next);
        previewDesign(needsReload(next, initial.design) ? initial.design : next);
    };

    const start = () => {
        markWelcomeDone();
        if (reloads) {
            requestDesignHint();
            switchDesign(design);
            return;
        }
        switchDesign(design);
        onDone(design !== initial.design || theme !== initial.theme || accent !== initial.accent);
    };

    const keepDefaults = () => {
        markWelcomeDone();
        previewDesign(initial.design);
        setTheme(initial.theme);
        setAccent(initial.accent);
        onDone(false);
    };

    const onKeyDownGroup = (e: KeyboardEvent<HTMLDivElement>) => {
        if (!['ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp'].includes(e.key)) return;
        e.preventDefault();
        const index = DESIGNS.findIndex((d) => d.id === design);
        const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : -1;
        const next = DESIGNS[(index + step + DESIGNS.length) % DESIGNS.length];
        pickDesign(next.id);
        e.currentTarget.querySelector<HTMLElement>(`[data-design-option="${next.id}"]`)?.focus();
    };

    return (
        <Modal
            isOpen
            onClose={keepDefaults}
            title={revisit ? 'Choose your look' : `Welcome to ${appTitle}`}
            maxWidth="max-w-3xl"
            footer={
                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <button type="button" onClick={keepDefaults} className="btn btn-ghost">
                        <RotateCcw size={16} aria-hidden="true" /> {revisit ? 'Cancel' : 'Keep the defaults'}
                    </button>
                    <button type="button" onClick={start} className="btn btn-primary btn-lg" data-autofocus>
                        {reloads ? `Switch to ${chosenLabel}` : revisit ? 'Done' : 'Start exploring'} <ArrowRight size={18} aria-hidden="true" />
                    </button>
                </div>
            }
        >
            <div className="flex flex-col">
                <p className="order-1 text-muted-foreground">
                    {revisit ? 'Try the designs side by side. Your choice is saved in this browser only.' : 'Pick the look you like. Your choice is saved in this browser only, and nothing here is final.'}
                </p>
                <div className="order-3 mt-6">
                    <h3 className="text-sm font-bold" id="welcome-design">Design</h3>
                    <div role="radiogroup" aria-labelledby="welcome-design" tabIndex={-1} onKeyDown={onKeyDownGroup} className="-mx-1 mt-2.5 grid snap-x snap-mandatory auto-cols-[minmax(13.5rem,80%)] grid-flow-col gap-3 overflow-x-auto px-1 pb-2 pt-1 outline-none [scrollbar-width:none] sm:mx-0 sm:auto-cols-auto sm:grid-flow-row sm:grid-cols-3 sm:overflow-visible sm:p-0">
                        {DESIGNS.map(({ id, label, tagline }) => {
                            const selected = id === design;
                            return (
                                <button
                                    key={id}
                                    type="button"
                                    role="radio"
                                    aria-checked={selected}
                                    tabIndex={selected ? 0 : -1}
                                    data-design-option={id}
                                    onClick={() => pickDesign(id)}
                                    className={`group relative flex snap-start flex-col gap-2.5 rounded-[1rem] p-2 text-left transition-[box-shadow,background-color] duration-150 ${selected ? 'bg-brand-soft ring-2 ring-brand' : 'bg-secondary/60 ring-1 ring-border hover:bg-secondary'}`}
                                >
                                    <DesignPreview design={id} dark={dark} accent={accentHex} />
                                    <span className="flex items-start gap-2 px-1 pb-1">
                                        <span className="min-w-0 flex-1">
                                            <span className="block font-black">{label}</span>
                                            <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{tagline}</span>
                                        </span>
                                        <span className={`mt-0.5 grid size-5 shrink-0 place-items-center rounded-full ${selected ? 'bg-brand text-white' : 'ring-1 ring-border'}`} aria-hidden="true">
                                            {selected && <Check size={13} strokeWidth={3} />}
                                        </span>
                                    </span>
                                </button>
                            );
                        })}
                    </div>
                    {reloads && (
                        <p className="mt-2 text-xs text-muted-foreground" role="status">
                            {chosenLabel} is a separate build, so the page reloads once when you continue.
                        </p>
                    )}
                </div>
                <div className="order-4 mt-6 grid gap-6 sm:grid-cols-2">
                    <div>
                        <h3 className="text-sm font-bold" id="welcome-mode">Color mode</h3>
                        <div role="radiogroup" aria-labelledby="welcome-mode" className="mt-2.5 inline-flex gap-1 rounded-full bg-secondary p-1">
                            {MODES.map(({ id, label, icon: Icon }) => (
                                <button
                                    key={id}
                                    type="button"
                                    role="radio"
                                    aria-checked={theme === id}
                                    onClick={() => setTheme(id)}
                                    className={`inline-flex items-center gap-1.5 rounded-full px-3.5 py-1.5 text-sm font-bold transition-colors ${theme === id ? 'bg-card text-foreground shadow-card' : 'text-muted-foreground hover:text-foreground'}`}
                                >
                                    <Icon size={15} aria-hidden="true" /> {label}
                                </button>
                            ))}
                        </div>
                    </div>
                    <div>
                        <h3 className="text-sm font-bold" id="welcome-accent">Accent color</h3>
                        <div role="radiogroup" aria-labelledby="welcome-accent" className="mt-2.5 flex flex-wrap gap-2">
                            {ACCENTS.map(({ id, label, swatch }) => (
                                <button
                                    key={id}
                                    type="button"
                                    role="radio"
                                    aria-checked={accent === id}
                                    aria-label={label}
                                    title={label}
                                    onClick={() => setAccent(id)}
                                    style={{ backgroundColor: swatch }}
                                    className={`grid size-9 place-items-center rounded-full text-white ring-offset-2 ring-offset-card transition-shadow ${accent === id ? 'ring-2 ring-foreground/70' : 'ring-1 ring-black/10 dark:ring-white/20'}`}
                                >
                                    {accent === id && <Check size={16} strokeWidth={3} aria-hidden="true" />}
                                </button>
                            ))}
                        </div>
                    </div>
                </div>
                <div className="order-2 mt-4 flex gap-3 rounded-[1rem] bg-brand-soft p-4 text-sm text-foreground sm:order-5 sm:mt-7">
                    <Info size={18} aria-hidden="true" className="mt-0.5 shrink-0 text-brand-ink" />
                    <p>
                        <span className="font-bold">You can change this at any time.</span>{' '}
                        Open the menu behind the{' '}
                        <span className="inline-flex translate-y-0.5 items-center gap-0.5 rounded-full bg-card px-1.5 py-0.5 align-baseline shadow-card" aria-label="sun or moon">
                            <Sun size={13} aria-hidden="true" /><span aria-hidden="true">/</span><Moon size={13} aria-hidden="true" />
                        </span>{' '}
                        button at the top right to switch the design, color mode, accent and effects.
                    </p>
                </div>
            </div>
        </Modal>
    );
};

export default WelcomeDialog;
