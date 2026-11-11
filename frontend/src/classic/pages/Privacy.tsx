import { getEnv } from '../../utils/env';
import { MetaTags } from '../../hooks/useMetaTags';

const Privacy = () => {
    const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';

    return (
        <div className="page-narrow">
            <MetaTags
                title="Privacy Policy"
                description={`Privacy policy for ${appTitle}.`}
            />
            <header className="mb-10">
                <h1 className="page-title">Privacy Policy</h1>
                <p className="page-lede">
                    Information on the processing of personal data
                </p>
            </header>

            <div className="space-y-10">
                <section className="panel p-6 sm:p-8">
                    <h2 className="section-title mb-3">1. Data Controller</h2>
                    <div className="space-y-1 text-sm">
                        <p className="font-bold">Lunoxia, Inh. Maximilian Stumpf</p>
                        <p className="text-muted-foreground">Owner: Maximilian Stumpf</p>
                        <p>Leutragraben 1<br />07743 Jena<br />Germany</p>
                        <p className="text-muted-foreground mt-2">Branch office: Frankenstraße 152, 90461 Nürnberg, Bavaria, Germany</p>
                        <p className="mt-2">
                            Email: <a href="mailto:support@lunoxia.de" className="font-medium text-brand-ink hover:underline">support@lunoxia.de</a>
                        </p>
                    </div>
                </section>

                <section>
                    <h2 className="section-title mb-2">2. Hosting and Server Logs</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        When you visit this website, the hosting provider automatically stores information
                        in so-called server log files (IP address, timestamp, requested page, user agent).
                        This data is required to ensure the stability and security of the service and is
                        not merged with other data sources.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">3. Sign-in via Discord (OAuth2)</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        We use Discord as an identity provider for authentication. When you sign in, we
                        process your Discord ID, your Discord username and your avatar image. This data is
                        required to maintain your account (e.g. favorites, albums, uploads, API keys) and is
                        stored exclusively for this purpose. It is not linked to any further Discord data.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">4. Local Storage and Cookies in Your Browser</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        This website stores data locally in your browser (localStorage/sessionStorage): your
                        sign-in token, your display preferences (theme, accent color, animations, NSFW consent, dismissed banners,
                        tag view, blurred NSFW previews, slideshow speed, preferred code language), your saved searches, the images you opened recently (shown under “Recently viewed” on the
                        home page, where you can remove them), the list you opened an image from (for this tab only, to step
                        through it) and short-lived technical values used for the login. Apart from the sign-in token, which is
                        sent to the API to authenticate your requests, this data never leaves your browser. The sign-in token
                        is deleted when you sign out; everything else when you clear your browser storage.
                    </p>
                    <p className="mt-3 text-muted-foreground leading-relaxed">
                        If you switch the design (Collector or Classic), your choice is stored in a cookie named
                        waifu_design for one year. It is only set after you pick a design, is sent only to this
                        website so that the chosen design can be delivered, contains no personal data and is not
                        used for tracking. You can delete it at any time in your browser settings.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">5. Content and Uploads</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        Uploaded images and their metadata (tags, artists, source) are stored on the servers
                        of the service and its storage provider (Cloudflare R2) and are publicly displayed
                        where applicable. By uploading, you confirm that you are entitled to publish the
                        content.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">6. Your Rights</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        You have the right to access, rectification, erasure, restriction of processing,
                        data portability and to object to the processing of your personal data. To exercise
                        these rights, contact the address stated above.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">7. Right to Lodge a Complaint</h2>
                    <p className="text-muted-foreground leading-relaxed">
                        You have the right to lodge a complaint with a data protection supervisory authority
                        if you believe that the processing of your personal data infringes the GDPR.
                    </p>
                </section>

                <section>
                    <h2 className="section-title mb-2">8. Security</h2>
                    <p className="text-muted-foreground leading-relaxed">
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
