import { useState, useEffect, useEffectEvent, useRef } from 'react';
import api from '../services/api';
import { ApiKeyDto } from '../types';
import { useNotification } from '../context/NotificationContext';
import Modal from '../components/Modal';
import ConfirmModal from '../components/modals/ConfirmModal';
import { Key, Plus, Trash2, Copy, Calendar, Shield, Clock, Edit2, TriangleAlert } from 'lucide-react';
import { getEnv } from '../utils/env';
import { useRequireAuth } from '../hooks/useRequireAuth';
import { MetaTags } from '../hooks/useMetaTags';

const DESCRIPTION_MAX = 100;

const pad = (n: number) => String(n).padStart(2, '0');

function toDateInput(date: Date): string {
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

function endOfDayIso(value: string): string {
    return new Date(`${value}T23:59:59`).toISOString();
}

function tomorrow(): string {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    return toDateInput(d);
}

const ApiKeys = () => {
    const user = useRequireAuth();
    const { showNotification } = useNotification();
    const [keys, setKeys] = useState<ApiKeyDto[]>([]);
    const [loading, setLoading] = useState(true);

    const [isCreateOpen, setIsCreateOpen] = useState(false);
    const [isEditOpen, setIsEditOpen] = useState(false);
    const [keyFormData, setKeyFormData] = useState({ description: '', expirationDate: '' });
    const [createdKey, setCreatedKey] = useState<string | null>(null);
    const copyButtonRef = useRef<HTMLButtonElement>(null);
    useEffect(() => {
        if (createdKey) copyButtonRef.current?.focus();
    }, [createdKey]);
    const [isSaving, setIsSaving] = useState(false);

    const [isRevokeOpen, setIsRevokeOpen] = useState(false);
    const [now] = useState(() => Date.now());
    const [selectedKey, setSelectedKey] = useState<ApiKeyDto | null>(null);

    const fetchKeys = async () => {
        if (!user) return;
        setLoading(true);
        try {
            const { data } = await api.get<ApiKeyDto[]>('/auth/api-keys');
            setKeys(data);
        } catch {
            return;
        } finally { setLoading(false); }
    };

    const loadKeys = useEffectEvent(() => { fetchKeys(); });

    useEffect(() => {
        if (user) loadKeys();
    }, [user]);

    const descriptionLength = [...keyFormData.description.trim()].length;
    const descriptionTooLong = descriptionLength > DESCRIPTION_MAX;
    const descriptionValid = descriptionLength >= 3 && !descriptionTooLong;

    const handleCreate = async () => {
        if (isSaving || !descriptionValid) return;
        setIsSaving(true);
        try {
            const payload: { description: string; expirationDate?: string } = { description: keyFormData.description.trim() };
            if (keyFormData.expirationDate) payload.expirationDate = endOfDayIso(keyFormData.expirationDate);

            const { data } = await api.post<ApiKeyDto & { key?: string, token?: string }>('/auth/api-keys', payload);
            setCreatedKey(data.key || data.token || "Error: Token not received");
            fetchKeys();
            setKeyFormData({ description: '', expirationDate: '' });
        } catch {
        } finally {
            setIsSaving(false);
        }
    };

    const handleEdit = async () => {
        if (!selectedKey || isSaving || !descriptionValid) return;
        setIsSaving(true);
        try {
            const payload = {
                description: keyFormData.description.trim(),
                expirationDate: keyFormData.expirationDate ? endOfDayIso(keyFormData.expirationDate) : null,
            };

            await api.patch(`/auth/api-keys/${selectedKey.id}`, payload);
            showNotification('success', 'API Key updated');
            setIsEditOpen(false);
            fetchKeys();
        } catch {
        } finally {
            setIsSaving(false);
        }
    };

    const handleRevoke = async () => {
        if (!selectedKey) return;
        try {
            await api.delete(`/auth/api-keys/${selectedKey.id}`);
            showNotification('success', 'API Key revoked');
            setIsRevokeOpen(false);
            fetchKeys();
        } catch {
        }
    };

    const copyToClipboard = async (text: string) => {
        if (!text) return;
        try {
            await navigator.clipboard.writeText(text);
            showNotification('success', 'Copied to clipboard');
        } catch {
            showNotification('warning', 'Could not access the clipboard. Please select the key and copy it manually.');
        }
    };

    const openEdit = (key: ApiKeyDto) => {
        setSelectedKey(key);
        const dateStr = key.expirationDate ? toDateInput(new Date(key.expirationDate)) : '';
        setKeyFormData({ description: key.description, expirationDate: dateStr });
        setIsEditOpen(true);
    };

    if (!user) return null;

    const rawBase = getEnv('VITE_API_URL') || '/api';
    const apiBase = (rawBase.startsWith('/') ? `${window.location.origin}${rawBase}` : rawBase).replace(/\/$/, '');

    return (
        <div className="page">
            <MetaTags title="API Keys" description="Manage your API keys." />
            <div className="page-header">
                <div>
                    <h1 className="page-title">API Keys</h1>
                    <p className="page-lede">Keys let your bots and apps call the API in your name. Send one in the <code className="whitespace-nowrap rounded-md bg-secondary px-1.5 py-0.5 font-mono text-[0.85em] text-foreground">X-Api-Key</code> header.</p>
                </div>
                <button
                    type="button"
                    aria-label="Generate API key"
                    onClick={() => { setCreatedKey(null); setKeyFormData({ description: '', expirationDate: '' }); setIsCreateOpen(true); }}
                    className="btn btn-primary shrink-0 self-start md:self-auto"
                >
                    <Plus size={18} aria-hidden="true" /> <span className="hidden sm:inline">Generate Key</span>
                </button>
            </div>

            <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
                <section aria-label="Your API keys" className="panel overflow-hidden">
                    {loading ? (
                        <div className="divide-y divide-border">
                            {[...Array(3)].map((_, i) => <div key={i} className="h-20 animate-pulse bg-secondary/50" />)}
                        </div>
                    ) : keys.length === 0 ? (
                        <div className="flex flex-col items-center px-6 py-14 text-center">
                            <span className="mb-4 grid size-12 place-items-center rounded-2xl bg-brand-soft text-brand-ink" aria-hidden="true">
                                <Key size={22} />
                            </span>
                            <p className="font-semibold">No API keys yet</p>
                            <p className="mt-1 max-w-sm text-sm text-muted-foreground">Generate a key to call the API from scripts, bots or other apps.</p>
                        </div>
                    ) : (
                        <ul className="divide-y divide-border">
                            {keys.map(key => {
                                const expired = !!key.expirationDate && new Date(key.expirationDate).getTime() < now;
                                return (
                                    <li key={key.id} className="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
                                        <div className="min-w-0">
                                            <p className="flex flex-wrap items-center gap-2">
                                                <span className="truncate font-semibold">{key.description}</span>
                                                {expired && <span className="inline-flex items-center rounded-full bg-destructive/10 px-2 py-0.5 text-xs font-semibold text-destructive">Expired</span>}
                                            </p>
                                            <dl className="mt-1.5 flex flex-wrap gap-x-5 gap-y-1 text-xs text-muted-foreground">
                                                <div className="flex items-center gap-1.5"><Calendar size={13} aria-hidden="true" /><dt className="sr-only">Created</dt><dd>Created {new Date(key.createdAt).toLocaleDateString()}</dd></div>
                                                <div className="flex items-center gap-1.5"><Shield size={13} aria-hidden="true" /><dt className="sr-only">Last used</dt><dd>{key.lastUsedAt ? `Last used ${new Date(key.lastUsedAt).toLocaleDateString()}` : 'Never used'}</dd></div>
                                                <div className="flex items-center gap-1.5"><Clock size={13} aria-hidden="true" /><dt className="sr-only">Expires</dt><dd>{key.expirationDate ? `${expired ? 'Expired' : 'Expires'} ${new Date(key.expirationDate).toLocaleDateString()}` : 'No expiration'}</dd></div>
                                            </dl>
                                        </div>
                                        <div className="flex shrink-0 items-center gap-1">
                                            <button type="button" onClick={() => openEdit(key)} className="btn btn-ghost btn-sm">
                                                <Edit2 size={14} aria-hidden="true" /> Edit
                                            </button>
                                            <button type="button" onClick={() => { setSelectedKey(key); setIsRevokeOpen(true); }} className="btn btn-sm text-destructive hover:bg-destructive/10">
                                                <Trash2 size={14} aria-hidden="true" /> Revoke
                                            </button>
                                        </div>
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                </section>

                <aside className="panel p-5">
                    <h2 className="section-title">Use a key</h2>
                    <p className="mt-1 text-sm text-muted-foreground">Requests with a key count towards your account and its limits.</p>
                    <pre className="mt-4 overflow-x-auto rounded-2xl bg-secondary p-4 font-mono text-[0.8125rem] leading-6 text-foreground"><code>{`curl ${apiBase}/images \\\n  -H "X-Api-Key: <your key>"`}</code></pre>
                </aside>
            </div>

            <Modal isOpen={isCreateOpen} onClose={() => { setIsCreateOpen(false); setCreatedKey(null); }} title="Generate API Key">
                {!createdKey ? (
                    <form className="space-y-5" onSubmit={(e) => { e.preventDefault(); handleCreate(); }}>
                        <div>
                            <label htmlFor="api-key-description" className="field-label">Description</label>
                            <input id="api-key-description" value={keyFormData.description} autoComplete="off" onChange={e => setKeyFormData({ ...keyFormData, description: e.target.value })} aria-invalid={descriptionTooLong} aria-describedby="api-key-description-hint" className="input" placeholder="e.g. My Bot" data-autofocus />
                            <p id="api-key-description-hint" className={`field-hint ${descriptionTooLong ? 'text-destructive' : ''}`}>3–{DESCRIPTION_MAX} characters ({descriptionLength}/{DESCRIPTION_MAX}).</p>
                        </div>
                        <div>
                            <label htmlFor="api-key-expiration" className="field-label">Expiration (optional)</label>
                            <input id="api-key-expiration" type="date" min={tomorrow()} value={keyFormData.expirationDate} onChange={e => setKeyFormData({ ...keyFormData, expirationDate: e.target.value })} className="input" />
                            <p className="field-hint">The key stays valid until the end of the selected day.</p>
                        </div>
                        <button type="submit" disabled={!descriptionValid || isSaving} aria-busy={isSaving} className="btn btn-primary w-full">{isSaving ? 'Generating…' : 'Generate'}</button>
                    </form>
                ) : (
                    <div className="space-y-4">
                        <div className="flex items-start gap-3 rounded-2xl bg-warning/12 p-4">
                            <TriangleAlert size={18} className="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
                            <div>
                                <p className="text-sm font-bold">Copy the key now</p>
                                <p className="mt-0.5 text-sm text-muted-foreground">It is shown only once. If you lose it, generate a new one and revoke this key.</p>
                            </div>
                        </div>
                        <button ref={copyButtonRef} type="button" onClick={() => copyToClipboard(createdKey)} aria-label="Copy the API key to the clipboard" className="group relative flex w-full cursor-pointer items-center gap-3 rounded-2xl border border-border bg-secondary p-4 text-left transition-colors hover:border-brand/50" data-autofocus>
                            <code className="flex-1 select-all break-all font-mono text-sm">{createdKey}</code>
                            <Copy size={17} aria-hidden="true" className="shrink-0 text-muted-foreground group-hover:text-foreground" />
                        </button>
                        <button type="button" onClick={() => { setIsCreateOpen(false); setCreatedKey(null); }} className="btn btn-primary w-full">Done</button>
                    </div>
                )}
            </Modal>

            <Modal isOpen={isEditOpen} onClose={() => setIsEditOpen(false)} title="Edit API Key">
                <form className="space-y-5" onSubmit={(e) => { e.preventDefault(); handleEdit(); }}>
                    <div>
                        <label htmlFor="api-key-edit-description" className="field-label">Description</label>
                        <input id="api-key-edit-description" value={keyFormData.description} autoComplete="off" onChange={e => setKeyFormData({ ...keyFormData, description: e.target.value })} aria-invalid={descriptionTooLong} aria-describedby="api-key-edit-description-hint" className="input" data-autofocus />
                        <p id="api-key-edit-description-hint" className={`field-hint ${descriptionTooLong ? 'text-destructive' : ''}`}>3–{DESCRIPTION_MAX} characters ({descriptionLength}/{DESCRIPTION_MAX}).</p>
                    </div>
                    <div>
                        <label htmlFor="api-key-edit-expiration" className="field-label">Expiration (optional)</label>
                        <input id="api-key-edit-expiration" type="date" value={keyFormData.expirationDate} onChange={e => setKeyFormData({ ...keyFormData, expirationDate: e.target.value })} className="input" />
                        <p className="field-hint">Clear the date to make the key non-expiring.</p>
                    </div>
                    <button type="submit" disabled={!descriptionValid || isSaving} aria-busy={isSaving} className="btn btn-primary w-full">{isSaving ? 'Saving…' : 'Save Changes'}</button>
                </form>
            </Modal>

            <ConfirmModal isOpen={isRevokeOpen} onClose={() => setIsRevokeOpen(false)} onConfirm={handleRevoke} title="Revoke API Key" message={<p>Revoke <strong className="text-foreground">{selectedKey?.description}</strong>? Apps using this key stop working immediately.</p>} confirmText="Revoke" />
        </div>
    );
};

export default ApiKeys;
