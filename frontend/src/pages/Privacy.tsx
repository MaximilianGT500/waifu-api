import { getEnv } from '../utils/env';
import { ShieldCheck } from 'lucide-react';
import { MetaTags } from '../hooks/useMetaTags';

const Privacy = () => {
    const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';

    return (
        <div className="container mx-auto px-4 py-12 max-w-4xl">
            <MetaTags
                title="Privacy Policy"
                description={`Privacy policy for ${appTitle}.`}
            />
            <div className="text-center mb-12">
                <h1 className="text-4xl font-black mb-4 bg-gradient-to-r from-primary to-purple-600 bg-clip-text text-transparent">
                    Privacy Policy
                </h1>
                <p className="text-xl text-muted-foreground">
                    Information on the processing of personal data
                </p>
            </div>

            <div className="space-y-6">
                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">1. Data Controller</h2>
                    <div className="space-y-1 text-sm">
                        <p className="font-bold">Lunoxia, Inh. Maximilian Stumpf</p>
                        <p className="text-muted-foreground">Owner: Maximilian Stumpf</p>
                        <p>Leutragraben 1<br />07743 Jena<br />Germany</p>
                        <p className="text-muted-foreground mt-2">Branch office: Frankenstraße 152, 90461 Nürnberg, Bavaria, Germany</p>
                        <p className="mt-2">
                            Email: <a href="mailto:support@lunoxia.de" className="text-primary hover:underline">support@lunoxia.de</a>
                        </p>
                    </div>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">2. Hosting and Server Logs</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        When you visit this website, the hosting provider automatically stores information
                        in so-called server log files (IP address, timestamp, requested page, user agent).
                        This data is required to ensure the stability and security of the service and is
                        not merged with other data sources.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">3. Sign-in via Discord (OAuth2)</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        We use Discord as an identity provider for authentication. When you sign in, we
                        process your Discord ID, your Discord username and your avatar image. This data is
                        required to maintain your account (e.g. favorites, albums, uploads, API keys) and is
                        stored exclusively for this purpose. It is not linked to any further Discord data.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">4. Local Storage in Your Browser</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        This website stores data exclusively locally in your browser
                        (localStorage/sessionStorage): your sign-in token, your theme preference and the
                        OAuth state value used for the login. This data never leaves your browser and is
                        deleted when you sign out or clear your browser storage.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">5. Content and Uploads</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        Uploaded images and their metadata (tags, artists, source) are stored on the servers
                        of the service and its storage provider (Cloudflare R2) and are publicly displayed
                        where applicable. By uploading, you confirm that you are entitled to publish the
                        content.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">6. Your Rights</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        You have the right to access, rectification, erasure, restriction of processing,
                        data portability and to object to the processing of your personal data. To exercise
                        these rights, contact the address stated above.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">7. Right to Lodge a Complaint</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        You have the right to lodge a complaint with a data protection supervisory authority
                        if you believe that the processing of your personal data infringes the GDPR.
                    </p>
                </section>

                <section className="p-8 bg-card border border-border rounded-2xl">
                    <h2 className="text-xl font-bold mb-4">8. Security</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        <span className="flex items-center gap-2 mb-2"><ShieldCheck size={18} className="text-primary" /> Measures</span>
                        All data transmission is encrypted (HTTPS). API keys are stored exclusively as hashes
                        and cannot be viewed again after creation. Access is protected against abuse by a
                        rate limit.
                    </p>
                </section>
            </div>
        </div>
    );
};

export default Privacy;
