import { getEnv } from '../utils/env';
import { Mail, MessageSquare, Phone, MapPin, Scale } from 'lucide-react';
import { MetaTags } from '../hooks/useMetaTags';

const Imprint = () => {
    const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';
    const discordServerUrl = getEnv('VITE_DISCORD_SERVER_URL');
    const githubUrl = getEnv('VITE_GITHUB_URL');

    return (
        <div className="container mx-auto px-4 py-12 max-w-4xl">
            <MetaTags
                title="Imprint"
                description={`Legal notice for ${appTitle}.`}
            />
            <div className="text-center mb-12">
                <h1 className="text-4xl font-black mb-4 bg-gradient-to-r from-primary to-purple-600 bg-clip-text text-transparent">
                    Imprint
                </h1>
                <p className="text-xl text-muted-foreground">
                    Information pursuant to § 5 DDG (German Digital Services Act)
                </p>
            </div>

            <div className="space-y-6">
                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">Operator</h2>
                    <div className="grid sm:grid-cols-2 gap-6 text-sm">
                        <div className="space-y-2">
                            <div>
                                <p className="font-bold">Lunoxia, Inh. Maximilian Stumpf</p>
                                <p className="text-muted-foreground">Legal form: Sole proprietorship (Einzelunternehmen)</p>
                            </div>
                            <div>
                                <p className="font-semibold text-muted-foreground uppercase text-xs tracking-wider mb-1">Owner</p>
                                <p>Maximilian Stumpf</p>
                            </div>
                            <div className="flex items-start gap-2">
                                <MapPin size={16} className="mt-0.5 shrink-0 text-primary" />
                                <div>
                                    <p>Leutragraben 1</p>
                                    <p>07743 Jena</p>
                                    <p>Germany</p>
                                </div>
                            </div>
                            <div>
                                <p className="font-semibold text-muted-foreground uppercase text-xs tracking-wider mb-1">Branch office</p>
                                <p>Frankenstraße 152</p>
                                <p>90461 Nürnberg</p>
                                <p>Bavaria, Germany</p>
                            </div>
                        </div>
                        <div className="space-y-3">
                            <div>
                                <p className="font-semibold text-muted-foreground uppercase text-xs tracking-wider mb-1">VAT ID (USt-IdNr.)</p>
                                <p>DE456123333</p>
                            </div>
                            <a href="mailto:support@lunoxia.de" className="flex items-center gap-3 text-primary hover:underline">
                                <Mail size={18} />
                                support@lunoxia.de
                            </a>
                            <a href="tel:+498008002200" className="flex items-center gap-3 text-foreground hover:underline">
                                <Phone size={18} />
                                +49 800 8002200
                            </a>
                            <div className="flex items-center gap-3 text-muted-foreground">
                                <Phone size={18} />
                                <span>+49 3641 5541001 <span className="text-xs">(emergency)</span></span>
                            </div>
                            {discordServerUrl && (
                                <a href={discordServerUrl} target="_blank" rel="noopener noreferrer" className="flex items-center gap-3 text-[#5865F2] hover:underline">
                                    <MessageSquare size={18} />
                                    Discord Community
                                </a>
                            )}
                        </div>
                    </div>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">Legal Notice</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        Lunoxia is a sole proprietorship operating under German law. The company is
                        subject to the law of the Federal Republic of Germany.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">Liability for Content</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        As a service provider, we are responsible for our own content on these pages in
                        accordance with § 7 (1) DDG and general laws. Pursuant to §§ 8 to 10 DDG, however,
                        we are not obliged as a service provider to monitor transmitted or stored third-party
                        information or to investigate circumstances that indicate illegal activity.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">Liability for Links</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        Our service contains links to external websites of third parties over whose content
                        we have no influence. Therefore we cannot assume any liability for such third-party
                        content. The respective provider or operator of the linked pages is always responsible
                        for the content of those pages.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">Copyright</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        The content and works created by the site operators on these pages are subject to
                        German copyright law. Reproduction, editing, distribution and any kind of use outside
                        the limits of copyright law require the written consent of the respective author or creator.
                    </p>
                </section>

                {githubUrl && (
                    <section className="p-8 bg-card border border-border rounded-2xl">
                        <h2 className="text-xl font-bold mb-4">Project</h2>
                        <a href={githubUrl} target="_blank" rel="noopener noreferrer" className="flex items-center gap-3 text-primary hover:underline">
                            <Scale size={18} />
                            Source code on GitHub
                        </a>
                    </section>
                )}
            </div>
        </div>
    );
};

export default Imprint;
