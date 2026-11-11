import { useId, useState, KeyboardEvent } from 'react';
import { Check, Copy, ExternalLink as ExternalLinkIcon, KeyRound } from 'lucide-react';
import { Link } from 'react-router';
import Modal from '../Modal';
import ExternalLink from '../ExternalLink';
import { apiUrl, snippet, SNIPPET_LANGUAGES, SnippetLanguage } from '../../utils/apiSnippets';
import { copyText } from '../../utils/clipboard';
import { getEnv } from '../../utils/env';
import { readLocal, writeLocal } from '../../utils/storage';
import { useNotification } from '../../context/NotificationContext';

interface ApiRequestDialogProps {
    isOpen: boolean;
    onClose: () => void;
    path: string;
    params?: URLSearchParams;
    needsAuth?: boolean;
    description?: string;
}

const LANGUAGE_KEY = 'api-snippet-language';

const readLanguage = (): SnippetLanguage => {
    const saved = readLocal(LANGUAGE_KEY);
    return SNIPPET_LANGUAGES.some((l) => l.id === saved) ? (saved as SnippetLanguage) : 'curl';
};

const ApiRequestDialog = ({ isOpen, onClose, path, params, needsAuth = false, description }: ApiRequestDialogProps) => {
    const { showNotification } = useNotification();
    const [language, setLanguage] = useState<SnippetLanguage>(readLanguage);
    const [copied, setCopied] = useState(false);
    const tabsId = useId();
    const url = apiUrl(path, params);
    const code = snippet(language, url, needsAuth);
    const docsUrl = getEnv('VITE_DOCS_URL');

    const choose = (next: SnippetLanguage) => {
        setLanguage(next);
        setCopied(false);
        writeLocal(LANGUAGE_KEY, next);
    };

    const onTabKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
        if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
        e.preventDefault();
        const index = SNIPPET_LANGUAGES.findIndex((l) => l.id === language);
        const step = e.key === 'ArrowRight' ? 1 : -1;
        const next = SNIPPET_LANGUAGES[(index + step + SNIPPET_LANGUAGES.length) % SNIPPET_LANGUAGES.length];
        choose(next.id);
        document.getElementById(`${tabsId}-${next.id}`)?.focus();
    };

    const copy = async () => {
        if (await copyText(code)) {
            setCopied(true);
            showNotification('success', 'Copied to clipboard');
            window.setTimeout(() => setCopied(false), 2000);
        } else {
            showNotification('error', 'Could not copy to the clipboard');
        }
    };

    return (
        <Modal isOpen={isOpen} onClose={onClose} title="Use this in the API" maxWidth="max-w-2xl">
            <p className="text-sm text-muted-foreground">
                {description ?? 'The same request the page just made, ready for your code.'}
            </p>

            <div role="tablist" aria-label="Snippet language" className="mt-5 inline-flex flex-wrap gap-1 rounded-full bg-sleeve p-1">
                {SNIPPET_LANGUAGES.map(({ id, label }) => (
                    <button
                        key={id}
                        id={`${tabsId}-${id}`}
                        type="button"
                        role="tab"
                        aria-selected={language === id}
                        aria-controls={`${tabsId}-panel`}
                        tabIndex={language === id ? 0 : -1}
                        onClick={() => choose(id)}
                        onKeyDown={onTabKeyDown}
                        className={`rounded-full px-3.5 py-1.5 text-[0.8125rem] font-bold transition-colors ${language === id ? 'bg-card text-foreground shadow-card' : 'text-muted-foreground hover:text-foreground'}`}
                    >
                        {label}
                    </button>
                ))}
            </div>

            <div id={`${tabsId}-panel`} role="tabpanel" aria-labelledby={`${tabsId}-${language}`} className="relative mt-3">
                <pre className="max-h-72 overflow-auto rounded-[var(--radius-control)] bg-zinc-950 p-4 pr-14 font-mono text-[0.8125rem] leading-6 text-zinc-100 dark:ring-1 dark:ring-white/8">
                    <code className="whitespace-pre-wrap break-all">{code}</code>
                </pre>
                <button
                    type="button"
                    onClick={copy}
                    aria-label={copied ? 'Copied' : 'Copy to clipboard'}
                    title="Copy to clipboard"
                    className="absolute right-2.5 top-2.5 grid size-9 place-items-center rounded-full bg-white/10 text-zinc-100 transition-colors hover:bg-white/20"
                >
                    {copied ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}
                </button>
            </div>

            {needsAuth && (
                <p className="mt-4 flex items-start gap-2 text-sm text-muted-foreground">
                    <KeyRound size={16} aria-hidden="true" className="mt-0.5 shrink-0 text-brand-ink" />
                    <span>
                        This request needs your account. Create a key under{' '}
                        <Link to="/api-keys" onClick={onClose} className="font-bold text-brand-ink underline-offset-2 hover:underline">API Keys</Link>{' '}
                        and send it in the <code className="font-mono text-foreground">X-Api-Key</code> header.
                    </span>
                </p>
            )}

            <div className="mt-5 flex flex-wrap items-center gap-2">
                {!needsAuth && (
                    <a href={url} target="_blank" rel="noopener noreferrer" className="btn btn-secondary btn-sm">
                        <ExternalLinkIcon size={14} aria-hidden="true" /> Open the JSON
                    </a>
                )}
                <ExternalLink href={docsUrl} trusted className="btn btn-ghost btn-sm">
                    Read the docs
                </ExternalLink>
            </div>
        </Modal>
    );
};

export default ApiRequestDialog;
