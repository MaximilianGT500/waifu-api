import { getEnv } from '../../utils/env';
import { Mail, MessageSquare } from 'lucide-react';
import { MetaTags } from '../../hooks/useMetaTags';
import ExternalLink from '../components/ExternalLink';
import { safeExternalUrl, safeMailto } from '../../utils/safeUrl';

const Contact = () => {
    const contactEmail = getEnv('VITE_CONTACT_EMAIL')?.trim();
    const mailto = safeMailto(contactEmail);
    const discordServerUrl = safeExternalUrl(getEnv('VITE_DISCORD_SERVER_URL'));

    if (!mailto && !discordServerUrl) {
        return (
            <div className="page-narrow">
                <h1 className="page-title">Contact</h1>
                <p className="page-lede">No contact details have been configured for this instance.</p>
            </div>
        );
    }

    return (
        <div className="page-narrow">
            <MetaTags
              title="Contact"
              description="Get in touch with us for questions, suggestions, or support."
            />
            <header className="mb-10">
                <h1 className="page-title">Get in Touch</h1>
                <p className="page-lede">Questions about the API, a takedown request or an idea? Reach us here.</p>
            </header>

            <div className="grid gap-4 sm:grid-cols-2">
                {mailto && (
                    <a href={mailto} className="panel group flex flex-col p-6 transition-[border-color,box-shadow] hover:border-brand/40 hover:shadow-float">
                        <span className="mb-5 grid size-11 place-items-center rounded-2xl bg-brand-soft text-brand-ink" aria-hidden="true">
                            <Mail size={20} />
                        </span>
                        <h2 className="section-title">Email</h2>
                        <p className="mt-1 flex-1 text-sm text-muted-foreground">We usually answer within a few days.</p>
                        <span className="mt-4 font-semibold text-brand-ink break-all group-hover:underline">{contactEmail}</span>
                    </a>
                )}

                {discordServerUrl && (
                    <ExternalLink
                        href={discordServerUrl}
                        trusted
                        className="panel group flex flex-col p-6 transition-[border-color,box-shadow] hover:border-[#5865F2]/50 hover:shadow-float"
                    >
                        <span className="mb-5 grid size-11 place-items-center rounded-2xl bg-[#5865F2]/12 text-[#5865F2]" aria-hidden="true">
                            <MessageSquare size={20} />
                        </span>
                        <h2 className="section-title">Discord community</h2>
                        <p className="mt-1 flex-1 text-sm text-muted-foreground">Real-time help and discussions with other users.</p>
                        <span className="mt-4 font-semibold text-[#5865F2] group-hover:underline dark:text-[#8b95f7]">Join the server</span>
                    </ExternalLink>
                )}
            </div>
        </div>
    );
};

export default Contact;
