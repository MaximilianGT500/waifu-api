import { AnchorHTMLAttributes, ReactNode } from 'react';
import { safeExternalUrl } from '../utils/safeUrl';

interface ExternalLinkProps extends Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href' | 'target' | 'rel'> {
    href: string | null | undefined;
    children?: ReactNode;
    fallback?: ReactNode;
    trusted?: boolean;
}

const ExternalLink = ({ href, children, fallback = null, trusted = false, ...rest }: ExternalLinkProps) => {
    const safe = safeExternalUrl(href);
    if (!safe) return <>{fallback}</>;
    return (
        <a {...rest} href={safe} target="_blank" rel={trusted ? 'noopener noreferrer' : 'noopener noreferrer nofollow ugc'}>
            {children ?? safe}
        </a>
    );
};

export default ExternalLink;
