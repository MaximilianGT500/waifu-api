import { getEnv } from '../utils/env';
import { Mail, MessageSquare, Phone, MapPin, Scale } from 'lucide-react';
import { MetaTags } from '../hooks/useMetaTags';
import ExternalLink from '../components/ExternalLink';
import { safeExternalUrl } from '../utils/safeUrl';

const Imprint = () => {
    const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';
    const discordServerUrl = safeExternalUrl(getEnv('VITE_DISCORD_SERVER_URL'));
    const githubUrl = safeExternalUrl(getEnv('VITE_GITHUB_URL'));

    return (
        <div className="page-narrow">
            <MetaTags
                title="Imprint"
                description={`Legal notice for ${appTitle}.`}
            />
            <header className="mb-10">
                <h1 className="page-title">Imprint</h1>
                <p className="page-lede">
                    Information pursuant to § 5 DDG (German Digital Services Act)
                </p>
            </header>

            <div className="space-y-10">
                <section className="panel p-6 sm:p-8">
                    <h2 className="section-title mb-4">Operator</h2>
                    <div className="grid sm:grid-cols-2 gap-6 text-sm">
                        <div className="space-y-2">
                            <div>
                                <p className="font-bold">Lunoxia, Inh. Maximilian Stumpf</p>
                                <p className="text-muted-foreground">Legal form: Sole proprietorship (Einzelunternehmen)</p>
                            </div>
                            <div>
                                <p className="mb-1 text-xs font-semibold text-muted-foreground">Owner</p>
                                <p>Maximilian Stumpf</p>
                            </div>
                            <div className="flex items-start gap-2">
                                <MapPin size={16} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden="true" />
                                <div>
                                    <p>Leutragraben 1</p>
                                    <p>07743 Jena</p>
                                    <p>Germany</p>
                                </div>
                            </div>
                            <div>
                                <p className="mb-1 text-xs font-semibold text-muted-foreground">Branch office</p>
                                <p>Frankenstraße 152</p>
                                <p>90461 Nürnberg</p>
                                <p>Bavaria, Germany</p>
                            </div>
                        </div>
                        <div className="space-y-3">
                            <div>
                                <p className="mb-1 text-xs font-semibold text-muted-foreground">VAT ID (USt-IdNr.)</p>
                                <p>DE456123333</p>
                            </div>
                            <a href="mailto:support@lunoxia.de" className="flex items-center gap-3 font-medium text-brand-ink hover:underline">
                                <Mail size={17} aria-hidden="true" />
                                support@lunoxia.de
                            </a>
                            <a href="tel:+498008002200" className="flex items-center gap-3 text-foreground hover:underline">
                                <Phone size={17} aria-hidden="true" />
                                +49 800 8002200
                            </a>
                            <div className="flex items-center gap-3 text-muted-foreground">
                                <Phone size={17} aria-hidden="true" />
                                <span>+49 3641 5541001 <span className="text-xs">(emergency)</span></span>
                            </div>
                            {discordServerUrl && (
                                <ExternalLink href={discordServerUrl} trusted className="flex items-center gap-3 text-[#5865F2] hover:underline">
                                    <MessageSquare size={17} aria-hidden="true" />
                                    Discord Community
                                </ExternalLink>
                            )}
                        </div>
                    </div>
                </section>

                <section>
                    <h2 className="section-title mb-2">Legal Notice</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        Lunoxia is a sole proprietorship operating under German law. The company is
                        subject to the law of the Federal Republic of Germany.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">Liability for Content</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        As a service provider, we are responsible for our own content on these pages in
                        accordance with § 7 (1) DDG and general laws. Pursuant to §§ 8 to 10 DDG, however,
                        we are not obliged as a service provider to monitor transmitted or stored third-party
                        information or to investigate circumstances that indicate illegal activity.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">Liability for Links</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        Our service contains links to external websites of third parties over whose content
                        we have no influence. Therefore we cannot assume any liability for such third-party
                        content. The respective provider or operator of the linked pages is always responsible
                        for the content of those pages.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">Copyright</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        The content and works created by the site operators on these pages are subject to
                        German copyright law. Reproduction, editing, distribution and any kind of use outside
                        the limits of copyright law require the written consent of the respective author or creator.
                    </p>
                </section>

                {githubUrl && (
                    <section>
                    <h2 className="section-title mb-2">Project</h2>
                        <ExternalLink href={githubUrl} trusted className="inline-flex items-center gap-2 font-medium text-brand-ink hover:underline">
                            <Scale size={16} aria-hidden="true" />
                            Source code on GitHub
                        </ExternalLink>
                    </section>
                )}
            </div>
        </div>
    );
};

export default Imprint;
