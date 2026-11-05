import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Save } from 'lucide-react';
import api from '../services/api';
import { useRequireAuth } from '../hooks/useRequireAuth';
import { usePermissions, invalidateBanner } from '../hooks/useBanner';
import { useNotification } from '../context/NotificationContext';
import { BannerView } from '../components/SiteBanner';
import { BannerSettings, BannerVariant } from '../types';

const MAX_MESSAGE_LENGTH = 500;

const variants: { value: BannerVariant; label: string; dot: string }[] = [
    { value: BannerVariant.Info, label: 'Info', dot: 'bg-info' },
    { value: BannerVariant.Success, label: 'Success', dot: 'bg-success' },
    { value: BannerVariant.Warning, label: 'Warning', dot: 'bg-warning' },
    { value: BannerVariant.Danger, label: 'Danger', dot: 'bg-destructive' },
];

const SiteBannerSettings = () => {
    const user = useRequireAuth();
    const { canManageBanner, isLoading: permissionsLoading } = usePermissions();
    const queryClient = useQueryClient();
    const { showNotification } = useNotification();

    const settingsQuery = useQuery({
        queryKey: ['banner', 'settings'],
        enabled: canManageBanner,
        queryFn: async () => (await api.get<BannerSettings>('/banner/settings')).data,
    });

    const [message, setMessage] = useState('');
    const [variant, setVariant] = useState<BannerVariant>(BannerVariant.Info);
    const [isEnabled, setIsEnabled] = useState(false);
    const [saving, setSaving] = useState(false);

    const [syncedSettings, setSyncedSettings] = useState<BannerSettings | undefined>(undefined);
    if (settingsQuery.data && settingsQuery.data !== syncedSettings) {
        setSyncedSettings(settingsQuery.data);
        setMessage(settingsQuery.data.message);
        setVariant(settingsQuery.data.variant);
        setIsEnabled(settingsQuery.data.isEnabled);
    }

    if (!user || permissionsLoading) return <div className="page text-center text-muted-foreground" role="status">Loading…</div>;
    if (!canManageBanner) return <div className="page text-center text-muted-foreground">You do not have permission to manage the site banner.</div>;

    const saved = settingsQuery.data;
    const isDirty = !!saved && (message !== saved.message || variant !== saved.variant || isEnabled !== saved.isEnabled);
    const messageLength = [...message].length;
    const messageTooLong = messageLength > MAX_MESSAGE_LENGTH;
    const canSave = isDirty && !saving && !messageTooLong && (!isEnabled || message.trim().length > 0);

    const handleSave = async () => {
        setSaving(true);
        try {
            const { data } = await api.put<BannerSettings>('/banner/settings', { message, variant, isEnabled });
            queryClient.setQueryData(['banner', 'settings'], data);
            await invalidateBanner(queryClient);
            showNotification('success', data.isEnabled ? 'Banner published.' : 'Banner saved (hidden).');
        } catch {
            return;
        } finally {
            setSaving(false);
        }
    };

    return (
        <div className="page-narrow">
            <div className="page-header">
                <div>
                    <h1 className="page-title">Site Banner</h1>
                    <p className="page-lede">Shown at the top of every page. Visitors can dismiss it; saving a change shows it again.</p>
                </div>
            </div>

            <section className="panel space-y-7 p-5 sm:p-7">
                {settingsQuery.isLoading ? (
                    <div className="h-40 animate-pulse rounded-[var(--radius-panel)] bg-sleeve" />
                ) : (
                    <>
                        <div className="flex items-center justify-between gap-4">
                            <div>
                                <p id="banner-visible-label" className="text-sm font-semibold">Show the banner</p>
                                <p className="text-sm text-muted-foreground">{isEnabled ? 'Visible to every visitor after saving.' : 'Hidden. The message is kept for later.'}</p>
                            </div>
                            <button
                                type="button"
                                role="switch"
                                aria-checked={isEnabled}
                                aria-labelledby="banner-visible-label"
                                onClick={() => setIsEnabled(!isEnabled)}
                                className={`relative h-7 w-12 shrink-0 rounded-full transition-colors ${isEnabled ? 'bg-brand' : 'bg-input'}`}
                            >
                                <span className={`absolute left-0.5 top-0.5 size-6 rounded-full bg-white shadow-sm transition-transform ${isEnabled ? 'translate-x-5' : ''}`} />
                            </button>
                        </div>

                        <div>
                            <label htmlFor="banner-message" className="field-label">Message</label>
                            <textarea
                                id="banner-message"
                                value={message}
                                onChange={e => setMessage(e.target.value)}
                                aria-invalid={messageTooLong}
                                aria-describedby="banner-message-count"
                                rows={3}
                                placeholder="e.g. Scheduled maintenance tonight from 22:00 UTC."
                                className="input resize-y"
                            />
                            <p id="banner-message-count" className={`field-hint text-right tabular ${messageTooLong ? 'text-destructive' : ''}`}>{messageLength}/{MAX_MESSAGE_LENGTH}</p>
                        </div>

                        <fieldset>
                            <legend className="field-label">Style</legend>
                            <div className="flex flex-wrap gap-2">
                                {variants.map(v => (
                                    <button
                                        key={v.value}
                                        type="button"
                                        aria-pressed={variant === v.value}
                                        onClick={() => setVariant(v.value)}
                                        className={`btn btn-sm border ${variant === v.value ? 'border-brand/50 bg-brand-soft text-brand-ink' : 'border-border text-muted-foreground hover:bg-secondary'}`}
                                    >
                                        <span className={`size-2.5 rounded-full ${v.dot}`} aria-hidden="true" /> {v.label}
                                    </button>
                                ))}
                            </div>
                        </fieldset>

                        <div>
                            <p className="field-label">Preview</p>
                            <div className="overflow-hidden rounded-2xl border border-border">
                                {message.trim()
                                    ? <BannerView message={message} variant={variant} onDismiss={() => {}} />
                                    : <p className="p-4 text-sm text-muted-foreground">Write a message to preview the banner.</p>}
                            </div>
                        </div>

                        <div className="flex flex-col-reverse justify-between gap-4 border-t border-border pt-5 sm:flex-row sm:items-center">
                            <p className="text-xs text-muted-foreground">
                                {saved?.updatedAt
                                    ? <>Last updated {new Date(saved.updatedAt).toLocaleString()}{saved.updatedBy && <> by <span className="font-semibold text-foreground">{saved.updatedBy.name}</span></>}</>
                                    : 'Never configured.'}
                            </p>
                            <button type="button" onClick={handleSave} disabled={!canSave} className="btn btn-primary">
                                <Save size={16} aria-hidden="true" /> {saving ? 'Saving…' : 'Save'}
                            </button>
                        </div>
                    </>
                )}
            </section>
        </div>
    );
};

export default SiteBannerSettings;
