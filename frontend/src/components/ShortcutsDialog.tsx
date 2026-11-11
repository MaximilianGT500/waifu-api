import { Fragment } from 'react';
import Modal from './Modal';
import { modKeyLabel } from '../hooks/useHotkeys';

interface ShortcutsDialogProps {
    isOpen: boolean;
    onClose: () => void;
}

const Key = ({ children }: { children: string }) => (
    <kbd className="inline-grid min-w-7 place-items-center rounded-lg border border-border bg-card px-1.5 py-0.5 font-mono text-xs font-bold text-foreground shadow-[0_1px_0_var(--border)]">
        {children}
    </kbd>
);

const ShortcutsDialog = ({ isOpen, onClose }: ShortcutsDialogProps) => {
    const mod = modKeyLabel();
    const groups: { title: string; items: [string[], string][] }[] = [
        {
            title: 'Everywhere',
            items: [
                [[mod, 'K'], 'Quick search'],
                [['/'], 'Quick search'],
                [['?'], 'Show these shortcuts'],
            ],
        },
        {
            title: 'Gallery and albums',
            items: [
                [['←', '→'], 'Previous / next page'],
                [['F'], 'Show or hide the filters'],
                [['T'], 'Show or hide tags on the cards'],
                [['D'], 'Draw a random card'],
                [['S'], 'Slideshow of the results'],
                [['X'], 'Select several images'],
                [['R'], 'Reload the results'],
            ],
        },
        {
            title: 'Image page',
            items: [
                [['←', '→'], 'Previous / next image of the list you came from'],
                [['F'], 'Full screen'],
                [['S'], 'Slideshow'],
                [['L'], 'Add to or remove from favorites'],
                [['A'], 'Add to an album'],
                [['D'], 'Download'],
                [['C'], 'Copy the page link'],
                [['Z'], 'Original size in full screen'],
                [['Esc'], 'Close full screen'],
            ],
        },
    ];

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Keyboard shortcuts" maxWidth="max-w-lg">
            <div className="space-y-6">
                {groups.map((group) => (
                    <section key={group.title}>
                        <h3 className="mb-2 text-sm font-bold text-muted-foreground">{group.title}</h3>
                        <dl className="divide-y-2 divide-dashed divide-border">
                            {group.items.map(([keys, label]) => (
                                <div key={`${keys.join('+')}-${label}`} className="flex items-center justify-between gap-4 py-2.5">
                                    <dt className="text-sm">{label}</dt>
                                    <dd className="flex shrink-0 items-center gap-1">
                                        {keys.map((key, i) => (
                                            <Fragment key={key}>
                                                {i > 0 && keys.length === 2 && keys[0] === mod && <span className="text-xs text-muted-foreground">+</span>}
                                                <Key>{key}</Key>
                                            </Fragment>
                                        ))}
                                    </dd>
                                </div>
                            ))}
                        </dl>
                    </section>
                ))}
                <p className="text-xs text-muted-foreground">Shortcuts pause while you type in a field or a dialog is open.</p>
            </div>
        </Modal>
    );
};

export default ShortcutsDialog;
