import { Outlet, Link, useLocation, useNavigationType } from 'react-router';
import { useTheme } from '../../context/ThemeContext';
import { useAuth } from '../../context/AuthContext';
import {
    Sun, Moon, Menu, X, LogOut, Upload as UploadIcon,
    Home, Image as ImageIcon, Tag as TagIcon,
    User as UserIcon, Library, ChevronDown, Palette, Key, Users as UsersIcon, Flag, BarChart, Monitor, HardDrive, GitPullRequest, Megaphone,
    LucideIcon, Sparkles, History, WandSparkles, Search, Info, Shield, Mail, EyeOff, BookOpen,
} from 'lucide-react';
import { useState, useEffect, useRef, Suspense, ReactNode, ButtonHTMLAttributes, lazy } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import GlobalErrorHandler from '../GlobalErrorHandler';
import { Dropdown, DropdownTrigger, DropdownItem, DropdownLabel, DropdownSeparator, DropdownSwatch } from '../Dropdown';
import { getEnv } from '../../utils/env';
import { DESIGNS, Design, switchDesign } from '../../utils/design';
import { useDesign } from '../../hooks/useDesign';
import { ACCENTS } from '../../utils/accent';
import { Role } from '../../types';
import { useNavBadges, invalidateNavBadges } from '../../hooks/useNavBadges';
import { usePermissions } from '../../hooks/useBanner';
import SiteBanner from '../SiteBanner';
import SessionErrorBanner from '../SessionErrorBanner';
import SafeExternalLink from '../ExternalLink';
import ErrorBoundary from '../ErrorBoundary';
import PageLoader from '../PageLoader';
import { safeExternalUrl } from '../../utils/safeUrl';
import { useHotkeys, dialogOpen, modKeyLabel } from '../../hooks/useHotkeys';
import { readNsfwBlur, setNsfwBlur } from '../../utils/nsfwBlur';
import type { PaletteDestination } from '../CommandPalette';
import ThemeHint from '../ThemeHint';
import { WELCOME_EVENT, shouldWelcome, takeDesignHint } from '../../utils/welcome';

const CommandPalette = lazy(() => import('../CommandPalette'));
const ShortcutsDialog = lazy(() => import('../ShortcutsDialog'));
const WelcomeDialog = lazy(() => import('../WelcomeDialog'));

interface NavEntry {
    key: string;
    label: string;
    icon: LucideIcon;
    to: string;
}

interface FooterLink {
    key: string;
    label: string;
    to?: string;
    href?: string;
}

const roleLabel = (role: Role) => role === Role.Admin ? 'Admin' : role === Role.Moderator ? 'Moderator' : 'Member';

const isActive = (pathname: string, to: string) => to === '/' ? pathname === '/' : pathname === to || pathname.startsWith(`${to}/`);

const CountBadge = ({ count, tone }: { count: number; tone: 'brand' | 'danger' }) => {
    if (count <= 0) return null;
    return (
        <span className={`ml-auto min-w-5 rounded-full px-1.5 py-0.5 text-center text-[0.6875rem] font-black leading-none tabular ${tone === 'danger' ? 'bg-destructive text-destructive-foreground' : 'bg-vivid text-vivid-foreground'}`}>
            {count > 99 ? '99+' : count}
        </span>
    );
};

const Brand = ({ title, compact = false }: { title: string; compact?: boolean }) => (
    <Link to="/" aria-label={`${title} home`} className="flex min-w-0 items-center gap-2.5 rounded-full outline-offset-4">
        <img
            src="/logo-96.png?v=20261024-logo-v2"
            srcSet="/logo-96.png?v=20261024-logo-v2 2x, /logo-144.png?v=20261024-logo-v2 3x"
            alt=""
            width={38}
            height={38}
            className="size-[2.375rem] shrink-0 -rotate-6 rounded-[0.75rem] bg-card object-cover p-0.5 shadow-card"
        />
        <span className={`brand-title truncate text-lg font-black tracking-[-0.01em] ${compact ? 'hidden sm:block' : ''}`}>{title}</span>
    </Link>
);

const DesignIcon = ({ design }: { design: Design }) => design === 'collector' ? <Sparkles size={16} /> : design === 'manga' ? <BookOpen size={16} /> : <History size={16} />;

const HeaderIconButton = ({ children, ...rest }: { children: ReactNode } & ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button
        type="button"
        {...rest}
        className="grid size-10 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-card hover:text-foreground"
    >
        {children}
    </button>
);

const Layout = () => {
    const { theme, setTheme, resolvedTheme, motionEnabled, setMotionEnabled, accent, setAccent } = useTheme();
    const design = useDesign();
    const { user, token, logout } = useAuth();
    const location = useLocation();
    const navigationType = useNavigationType();

    const [drawerPath, setDrawerPath] = useState<string | null>(null);
    const isMobileMenuOpen = drawerPath === location.pathname;
    const closeDrawer = () => setDrawerPath(null);
    const menuButtonRef = useRef<HTMLButtonElement>(null);
    const drawerRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (navigationType !== 'POP') window.scrollTo({ top: 0, behavior: 'instant' as ScrollBehavior });
    }, [location.pathname, navigationType]);

    useEffect(() => {
        if (!isMobileMenuOpen) return;
        const drawer = drawerRef.current;
        drawer?.querySelector<HTMLElement>('button, a[href]')?.focus();
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                setDrawerPath(null);
                return;
            }
            if (e.key !== 'Tab' || !drawer) return;
            const items = Array.from(drawer.querySelectorAll<HTMLElement>('button:not([disabled]), a[href]'));
            if (items.length === 0) return;
            const first = items[0];
            const last = items[items.length - 1];
            if (e.shiftKey && document.activeElement === first) {
                e.preventDefault();
                last.focus();
            } else if (!e.shiftKey && document.activeElement === last) {
                e.preventDefault();
                first.focus();
            }
        };
        document.addEventListener('keydown', onKeyDown);
        const menuButton = menuButtonRef.current;
        return () => {
            document.removeEventListener('keydown', onKeyDown);
            if (menuButton && menuButton.getClientRects().length > 0) menuButton.focus({ preventScroll: true });
        };
    }, [isMobileMenuOpen]);

    const [welcome, setWelcome] = useState<'first' | 'revisit' | null>(() => (shouldWelcome(location.pathname) ? 'first' : null));
    const [isHintOpen, setIsHintOpen] = useState(false);

    useEffect(() => {
        if (takeDesignHint()) setIsHintOpen(true);
        const open = () => setWelcome('revisit');
        window.addEventListener(WELCOME_EVENT, open);
        return () => window.removeEventListener(WELCOME_EVENT, open);
    }, []);

    const [isPaletteOpen, setIsPaletteOpen] = useState(false);
    const [isShortcutsOpen, setIsShortcutsOpen] = useState(false);
    const [modLabel] = useState(modKeyLabel);
    const [blurNsfw, setBlurNsfw] = useState(readNsfwBlur);

    const toggleBlurNsfw = () => {
        setNsfwBlur(!blurNsfw);
        setBlurNsfw(!blurNsfw);
    };

    useHotkeys({
        'mod+k': () => {
            if (isPaletteOpen) {
                setIsPaletteOpen(false);
                return;
            }
            if (dialogOpen()) return false;
            setIsPaletteOpen(true);
        },
    }, { inDialogs: true });

    useHotkeys({
        '/': () => setIsPaletteOpen(true),
        '?': () => setIsShortcutsOpen(true),
    });

    const { pendingCount, reportCount } = useNavBadges();
    const queryClient = useQueryClient();
    useEffect(() => { invalidateNavBadges(queryClient); }, [location.pathname, queryClient]);

    const handleLogout = () => {
        logout();
        window.location.assign('/');
    };

    const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';
    const contactEmail = getEnv('VITE_CONTACT_EMAIL');
    const discordServerUrl = getEnv('VITE_DISCORD_SERVER_URL');
    const appVersion = getEnv('VITE_APP_VERSION');
    const showContact = !!contactEmail || !!safeExternalUrl(discordServerUrl);

    const tabs: NavEntry[] = [
        { key: 'home', to: '/', label: 'Home', icon: Home },
        { key: 'gallery', to: '/gallery', label: 'Gallery', icon: ImageIcon },
        { key: 'albums', to: '/albums', label: 'Albums', icon: Library },
        { key: 'tags', to: '/tags', label: 'Tags', icon: TagIcon },
        { key: 'artists', to: '/artists', label: 'Artists', icon: Palette },
        { key: 'upload', to: '/upload', label: 'Upload', icon: UploadIcon },
    ];

    const footerLinks: FooterLink[] = [
        { key: 'docs', href: getEnv('VITE_DOCS_URL'), label: 'Documentation' },
        { key: 'github', href: getEnv('VITE_GITHUB_URL'), label: 'Source code' },
        { key: 'status', href: getEnv('VITE_STATUS_URL'), label: 'Status' },
        { key: 'tos', href: getEnv('VITE_TOS_URL'), label: 'Terms of Service' },
        { key: 'imprint', to: '/imprint', label: 'Imprint' },
        { key: 'privacy', to: '/privacy', label: 'Privacy' },
        ...(showContact ? [{ key: 'contact', to: '/contact', label: 'Contact' }] : []),
    ].filter((link) => link.to || !!safeExternalUrl(link.href));

    const isModOrAdmin = user && (user.role === Role.Moderator || user.role === Role.Admin);
    const isAdmin = user && user.role === Role.Admin;
    const { canManageBanner } = usePermissions();

    const destinations: PaletteDestination[] = [
        ...tabs,
        ...(user ? [
            { key: 'account', to: '/account', label: 'Account', icon: UserIcon },
            { key: 'my-albums', to: '/albums', label: 'My Albums', icon: Library },
            { key: 'api-keys', to: '/api-keys', label: 'API Keys', icon: Key },
            ...(!isModOrAdmin ? [
                { key: 'my-submissions', to: '/review', label: 'My Submissions', icon: GitPullRequest },
                { key: 'my-reports', to: '/my-reports', label: 'My Reports', icon: Flag },
            ] : []),
        ] : [{ key: 'login', to: '/login', label: 'Log in', icon: UserIcon }]),
        ...(canManageBanner ? [{ key: 'site-banner', to: '/site-banner', label: 'Site Banner', icon: Megaphone }] : []),
        ...(isModOrAdmin ? [
            { key: 'review', to: '/review', label: 'Review queue', icon: GitPullRequest },
            { key: 'reports', to: '/reports', label: 'Reports', icon: Flag },
        ] : []),
        ...(isAdmin ? [
            { key: 'users', to: '/users', label: 'User Management', icon: UsersIcon },
            { key: 'stats', to: '/stats', label: 'Statistics', icon: BarChart },
            { key: 'storage-diff', to: '/storage-diff', label: 'Storage Diff', icon: HardDrive },
        ] : []),
        { key: 'privacy', to: '/privacy', label: 'Privacy', icon: Shield },
        { key: 'imprint', to: '/imprint', label: 'Imprint', icon: Info },
        ...(showContact ? [{ key: 'contact', to: '/contact', label: 'Contact', icon: Mail }] : []),
    ].filter((entry, index, all) => all.findIndex((other) => other.to === entry.to && other.label === entry.label) === index);

    const renderFooterLink = (link: FooterLink, className: string, onClick?: () => void) => link.href ? (
        <SafeExternalLink key={link.key} href={link.href} trusted onClick={onClick} className={className}>
            {link.label}
        </SafeExternalLink>
    ) : (
        <Link key={link.key} to={link.to!} onClick={onClick} aria-current={location.pathname === link.to ? 'page' : undefined} className={className}>
            {link.label}
        </Link>
    );

    return (
        <div className="min-h-dvh bg-canvas text-foreground">
            <a
                href="#main-content"
                onClick={(e) => {
                    e.preventDefault();
                    document.getElementById('main-content')?.focus();
                }}
                className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-200 focus:rounded-full focus:bg-primary focus:px-4 focus:py-2 focus:text-sm focus:font-bold focus:text-primary-foreground focus:shadow-float"
            >
                Skip to content
            </a>
            <GlobalErrorHandler />

            <header className="sticky top-0 z-40 bg-canvas/88 backdrop-blur-md lg:static lg:bg-transparent lg:backdrop-blur-none">
                <div className="mx-auto flex h-16 max-w-[100rem] items-center justify-between gap-3 px-3 sm:px-5 lg:h-[4.75rem] lg:px-8">
                    <div className="flex min-w-0 items-center gap-1.5">
                        <button
                            ref={menuButtonRef}
                            type="button"
                            aria-label="Open navigation menu"
                            aria-expanded={isMobileMenuOpen}
                            aria-controls="mobile-navigation"
                            className="grid size-10 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-card hover:text-foreground lg:hidden"
                            onClick={() => setDrawerPath(isMobileMenuOpen ? null : location.pathname)}
                        >
                            <Menu size={22} aria-hidden="true" />
                        </button>
                        <Brand title={appTitle} compact />
                    </div>

                    <div className="flex items-center gap-1.5 sm:gap-2">
                        <button
                            type="button"
                            onClick={() => setIsPaletteOpen(true)}
                            aria-label={`Quick search (${modLabel} K)`}
                            aria-haspopup="dialog"
                            className="grid size-10 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-card hover:text-foreground md:flex md:h-10 md:w-56 md:items-center md:justify-start md:gap-2.5 md:bg-card/70 md:px-4 md:text-sm md:font-bold md:shadow-card md:ring-1 md:ring-border/60 lg:w-64 dark:md:ring-white/6"
                        >
                            <Search size={18} aria-hidden="true" className="shrink-0" />
                            <span className="hidden flex-1 text-left md:inline">Search…</span>
                            <kbd className="hidden rounded-md border border-border px-1.5 py-px font-mono text-[0.6875rem] text-muted-foreground md:inline" aria-hidden="true">{modLabel} K</kbd>
                        </button>
                        <div className="relative">
                            <Dropdown
                                width="w-[17rem]"
                                label="Change color theme, accent, design and effects"
                                onOpen={() => setIsHintOpen(false)}
                                trigger={
                                    <DropdownTrigger className="theme-menu-trigger grid size-10 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-card hover:text-foreground">
                                        {resolvedTheme === 'light' ? <Sun size={19} aria-hidden="true" /> : <Moon size={19} aria-hidden="true" />}
                                    </DropdownTrigger>
                                }
                            >
                                <div role="group" aria-label="Color theme">
                                    <DropdownLabel>Color theme</DropdownLabel>
                                    <DropdownItem onClick={() => setTheme('light')} active={theme === 'light'} icon={<Sun size={16} />}>Light</DropdownItem>
                                    <DropdownItem onClick={() => setTheme('dark')} active={theme === 'dark'} icon={<Moon size={16} />}>Dark</DropdownItem>
                                    <DropdownItem onClick={() => setTheme('system')} active={theme === 'system'} icon={<Monitor size={16} />}>System</DropdownItem>
                                </div>
                                <DropdownSeparator />
                                <div role="group" aria-label="Accent color">
                                    <DropdownLabel>Accent color</DropdownLabel>
                                    <div role="none" className="flex flex-wrap gap-2 px-3 pb-2 pt-1">
                                        {ACCENTS.map(({ id, label, swatch }) => (
                                            <DropdownSwatch key={id} color={swatch} label={label} active={accent === id} onClick={() => setAccent(id)} />
                                        ))}
                                    </div>
                                </div>
                                <DropdownSeparator />
                                <div role="group" aria-label="Design">
                                    <DropdownLabel>Design</DropdownLabel>
                                    {DESIGNS.map(({ id, label }) => (
                                        <DropdownItem key={id} onClick={() => switchDesign(id)} active={design === id} icon={<DesignIcon design={id} />}>{label}</DropdownItem>
                                    ))}
                                </div>
                                <DropdownSeparator />
                                <div role="group" aria-label="Effects">
                                    <DropdownLabel>Effects</DropdownLabel>
                                    <DropdownItem checkbox onClick={() => setMotionEnabled(!motionEnabled)} active={motionEnabled} icon={<WandSparkles size={16} />}>Animations and effects</DropdownItem>
                                    <DropdownItem checkbox onClick={toggleBlurNsfw} active={blurNsfw} icon={<EyeOff size={16} />}>Blur NSFW previews</DropdownItem>
                                </div>
                            </Dropdown>
                            {isHintOpen && <ThemeHint onClose={() => setIsHintOpen(false)} />}
                        </div>

                        {user ? (
                            <Dropdown
                                width="w-64"
                                label={`Account menu (${user.name})`}
                                trigger={
                                    <DropdownTrigger className="flex max-w-[14rem] items-center gap-2.5 rounded-full bg-card py-1 pl-1 pr-3 shadow-card transition-colors hover:bg-secondary dark:ring-1 dark:ring-white/6">
                                        {user.avatarUrl ? (
                                            <img src={user.avatarUrl} alt="" referrerPolicy="no-referrer" className="size-8 shrink-0 rounded-full object-cover" />
                                        ) : (
                                            <span className="grid size-8 shrink-0 place-items-center rounded-full bg-brand-soft text-brand-ink" aria-hidden="true">
                                                <UserIcon size={16} />
                                            </span>
                                        )}
                                        <span className="hidden min-w-0 text-left leading-tight md:block">
                                            <span className="block max-w-[8rem] truncate text-sm font-bold">{user.name}</span>
                                            <span className="block text-xs text-muted-foreground">{roleLabel(user.role)}</span>
                                        </span>
                                        <ChevronDown size={15} aria-hidden="true" className="shrink-0 text-muted-foreground" />
                                    </DropdownTrigger>
                                }
                            >
                                <div className="border-b border-border px-3 pb-2.5 pt-1.5 md:hidden">
                                    <p className="truncate font-bold">{user.name}</p>
                                    <p className="text-xs text-muted-foreground">{roleLabel(user.role)}</p>
                                </div>

                                <DropdownLabel>Your collection</DropdownLabel>
                                <DropdownItem to="/account" icon={<UserIcon size={16} />}>Account</DropdownItem>
                                <DropdownItem to="/upload" icon={<UploadIcon size={16} />}>Upload Image</DropdownItem>
                                <DropdownItem to="/albums" icon={<Library size={16} />}>My Albums</DropdownItem>
                                <DropdownItem to="/api-keys" icon={<Key size={16} />}>API Keys</DropdownItem>
                                {!isModOrAdmin && (
                                    <>
                                        <DropdownItem to="/review" icon={<GitPullRequest size={16} />}>My Submissions<CountBadge count={pendingCount} tone="brand" /></DropdownItem>
                                        <DropdownItem to="/my-reports" icon={<Flag size={16} />}>My Reports</DropdownItem>
                                    </>
                                )}

                                {(isModOrAdmin || canManageBanner) && (
                                    <>
                                        <DropdownSeparator />
                                        <DropdownLabel>Administration</DropdownLabel>
                                        {canManageBanner && (
                                            <DropdownItem to="/site-banner" icon={<Megaphone size={16} />}>Site Banner</DropdownItem>
                                        )}
                                        {isModOrAdmin && (
                                            <>
                                                <DropdownItem to="/review" icon={<GitPullRequest size={16} />}>Review<CountBadge count={pendingCount} tone="brand" /></DropdownItem>
                                                <DropdownItem to="/reports" icon={<Flag size={16} />}>Reports<CountBadge count={reportCount} tone="danger" /></DropdownItem>
                                            </>
                                        )}
                                        {isAdmin && (
                                            <>
                                                <DropdownItem to="/users" icon={<UsersIcon size={16} />}>User Management</DropdownItem>
                                                <DropdownItem to="/stats" icon={<BarChart size={16} />}>Statistics</DropdownItem>
                                                <DropdownItem to="/storage-diff" icon={<HardDrive size={16} />}>Storage Diff</DropdownItem>
                                            </>
                                        )}
                                    </>
                                )}

                                <DropdownSeparator />
                                <DropdownItem onClick={handleLogout} icon={<LogOut size={16} />} danger>Log out</DropdownItem>
                            </Dropdown>
                        ) : token ? (
                            <div className="size-9 animate-pulse rounded-full bg-card" aria-hidden="true" />
                        ) : (
                            <Link to="/login" className="btn btn-primary btn-sm px-5">
                                Login
                            </Link>
                        )}
                    </div>
                </div>
            </header>

            {welcome && (
                <Suspense fallback={null}>
                    <WelcomeDialog
                        revisit={welcome === 'revisit'}
                        onDone={(changed) => {
                            setWelcome(null);
                            if (changed || welcome === 'first') setIsHintOpen(true);
                        }}
                    />
                </Suspense>
            )}

            {(isPaletteOpen || isShortcutsOpen) && (
                <Suspense fallback={null}>
                    {isPaletteOpen && (
                        <CommandPalette
                            onClose={() => setIsPaletteOpen(false)}
                            destinations={destinations}
                            onShowShortcuts={() => setIsShortcutsOpen(true)}
                        />
                    )}
                    {isShortcutsOpen && <ShortcutsDialog isOpen onClose={() => setIsShortcutsOpen(false)} />}
                </Suspense>
            )}

            {isMobileMenuOpen && (
                <div className="fixed inset-0 z-50 lg:hidden">
                    <div className="absolute inset-0 bg-zinc-950/55 backdrop-blur-[2px] animate-in fade-in" aria-hidden="true" onClick={closeDrawer} />
                    <div
                        ref={drawerRef}
                        id="mobile-navigation"
                        role="dialog"
                        aria-modal="true"
                        aria-label="Navigation menu"
                        className="absolute inset-y-0 left-0 flex w-[min(20rem,88vw)] flex-col rounded-r-[var(--radius-sheet)] bg-background shadow-float animate-in slide-in-from-left duration-300"
                    >
                        <div className="flex h-16 shrink-0 items-center justify-between px-4">
                            <span className="truncate text-lg font-black">{appTitle}</span>
                            <HeaderIconButton aria-label="Close navigation menu" onClick={closeDrawer}>
                                <X size={20} aria-hidden="true" />
                            </HeaderIconButton>
                        </div>
                        <nav aria-label="Main navigation" className="flex-1 space-y-1 overflow-y-auto overscroll-contain px-3 py-2">
                            {tabs.map(({ key, to, label, icon: Icon }) => (
                                <Link key={key} to={to} onClick={closeDrawer} aria-current={isActive(location.pathname, to) ? 'page' : undefined} className="nav-item">
                                    <Icon size={20} strokeWidth={2} aria-hidden="true" className="shrink-0" />
                                    <span className="truncate">{label}</span>
                                </Link>
                            ))}
                            <p className="nav-group" aria-hidden="true">About</p>
                            {footerLinks.map((link) => renderFooterLink(link, 'nav-item', closeDrawer))}
                        </nav>
                        {appVersion && <p className="px-6 py-4 text-xs text-muted-foreground tabular">Version {appVersion}</p>}
                    </div>
                </div>
            )}

            <div className="mx-auto flex max-w-[100rem] items-start px-2 pb-4 sm:px-4 lg:px-8 lg:pb-8">
                <nav aria-label="Main navigation" className="sticky top-8 z-10 -mr-px hidden w-[10.5rem] shrink-0 flex-col gap-1.5 pt-10 lg:flex">
                    {tabs.map(({ key, to, label, icon: Icon }) => (
                        <Link key={key} to={to} aria-current={isActive(location.pathname, to) ? 'page' : undefined} className="index-tab">
                            <Icon size={19} strokeWidth={2.1} aria-hidden="true" className="shrink-0" />
                            <span className="truncate">{label}</span>
                        </Link>
                    ))}
                </nav>

                <main
                    id="main-content"
                    tabIndex={-1}
                    className="relative min-w-0 flex-1 rounded-[1.25rem] bg-background shadow-sheet outline-hidden ring-1 ring-border/70 sm:rounded-[var(--radius-sheet)] dark:ring-white/5"
                >
                    <div className="min-h-[calc(100dvh-4rem)] lg:min-h-[calc(100dvh-4.75rem)]">
                        <SessionErrorBanner />
                        <SiteBanner />
                        <ErrorBoundary resetKey={location.pathname}>
                            <Suspense fallback={<PageLoader />}>
                                <Outlet />
                            </Suspense>
                        </ErrorBoundary>
                    </div>
                    <footer className="flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-dashed border-border px-5 py-5 text-[0.8125rem] text-muted-foreground sm:px-7 lg:px-10">
                        {footerLinks.map((link) => renderFooterLink(link, 'font-bold transition-colors hover:text-foreground'))}
                        {appVersion && <span className="ml-auto tabular">Version {appVersion}</span>}
                    </footer>
                </main>
            </div>
        </div>
    );
};

export default Layout;
