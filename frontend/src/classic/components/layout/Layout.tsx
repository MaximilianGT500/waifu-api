import { Outlet, Link, useLocation } from 'react-router';
import { useTheme } from '../../../context/ThemeContext';
import { useAuth } from '../../../context/AuthContext';
import {
    Sun, Moon, Menu, X, LogOut, Upload as UploadIcon,
    Home, Image as ImageIcon, Tag as TagIcon, PanelLeftClose, PanelLeftOpen,
    User as UserIcon, Library, ChevronDown, Palette, Key, Users as UsersIcon, Flag, BarChart, Monitor, Book, Mail, Activity, HardDrive, FolderGit2, FileText, GitPullRequest, Megaphone, Scale, ShieldCheck,
    LucideIcon, Sparkles, History, WandSparkles, BookOpen,
} from 'lucide-react';
import { useState, useEffect, useRef, Suspense, ReactNode, ButtonHTMLAttributes } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import GlobalErrorHandler from '../GlobalErrorHandler';
import { Dropdown, DropdownTrigger, DropdownItem, DropdownLabel, DropdownSeparator, DropdownSwatch } from '../Dropdown';
import { getEnv } from '../../../utils/env';
import { DESIGNS, currentDesign, switchDesign } from '../../../utils/design';
import { ACCENTS } from '../../../utils/accent';
import { flashCoachMark, takeDesignHint } from '../../../utils/welcome';
import { useNotification } from '../../../context/NotificationContext';
import { Role } from '../../../types';
import { useNavBadges, invalidateNavBadges } from '../../../hooks/useNavBadges';
import { usePermissions } from '../../../hooks/useBanner';
import SiteBanner from '../SiteBanner';
import SessionErrorBanner from '../SessionErrorBanner';
import SafeExternalLink from '../ExternalLink';
import ErrorBoundary from '../ErrorBoundary';
import PageLoader from '../PageLoader';
import { safeExternalUrl } from '../../../utils/safeUrl';

interface NavEntry {
    key: string;
    label: string;
    icon: LucideIcon;
    to?: string;
    href?: string;
}

interface NavGroup {
    label: string;
    entries: NavEntry[];
}

const roleLabel = (role: Role) => role === Role.Admin ? 'Admin' : role === Role.Moderator ? 'Moderator' : 'Member';

const CountBadge = ({ count, tone }: { count: number; tone: 'brand' | 'danger' }) => {
    if (count <= 0) return null;
    return (
        <span className={`ml-auto min-w-5 rounded-full px-1.5 py-0.5 text-center text-[0.6875rem] font-bold leading-none tabular ${tone === 'danger' ? 'bg-destructive text-destructive-foreground' : 'bg-brand text-white dark:text-zinc-950'}`}>
            {count > 99 ? '99+' : count}
        </span>
    );
};

const NavList = ({ groups, collapsed, pathname, onNavigate }: {
    groups: NavGroup[];
    collapsed: boolean;
    pathname: string;
    onNavigate?: () => void;
}) => (
    <>
        {groups.map((group, index) => (
            <div key={group.label} role="group" aria-label={group.label} className="space-y-0.5">
                {collapsed
                    ? index > 0 && <div className="mx-3 my-3 h-px bg-border" aria-hidden="true" />
                    : <p className={`nav-group ${index === 0 ? 'pt-1' : ''}`} aria-hidden="true">{group.label}</p>}
                {group.entries.map(({ key, label, icon: Icon, to, href }) => {
                    const body = (
                        <>
                            <Icon size={20} strokeWidth={1.9} aria-hidden="true" className="shrink-0" />
                            {!collapsed && <span className="truncate">{label}</span>}
                        </>
                    );
                    const cls = `nav-item ${collapsed ? 'justify-center px-0' : ''}`;
                    if (href) {
                        return (
                            <SafeExternalLink
                                key={key}
                                href={href}
                                trusted
                                onClick={onNavigate}
                                className={cls}
                                title={collapsed ? label : undefined}
                                aria-label={collapsed ? `${label} (opens in a new tab)` : undefined}
                            >
                                {body}
                            </SafeExternalLink>
                        );
                    }
                    return (
                        <Link
                            key={key}
                            to={to!}
                            onClick={onNavigate}
                            aria-current={pathname === to ? 'page' : undefined}
                            className={cls}
                            title={collapsed ? label : undefined}
                            aria-label={collapsed ? label : undefined}
                        >
                            {body}
                        </Link>
                    );
                })}
            </div>
        ))}
    </>
);

const HeaderIconButton = ({ children, ...rest }: { children: ReactNode } & ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button
        type="button"
        {...rest}
        className="grid size-10 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
    >
        {children}
    </button>
);

const Layout = () => {
    const { theme, setTheme, resolvedTheme, motionEnabled, setMotionEnabled, accent, setAccent } = useTheme();
    const design = currentDesign();
    const { user, token, logout } = useAuth();
    const location = useLocation();
    const { showNotification } = useNotification();

    const [isDesignHintShown, setIsDesignHintShown] = useState(false);

    useEffect(() => {
        if (!takeDesignHint()) return;
        setIsDesignHintShown(true);
        showNotification('info', 'Classic design selected. Switch the design, colors and effects at any time with the sun/moon button at the top right.');
    }, [showNotification]);

    useEffect(() => (isDesignHintShown ? flashCoachMark(12_000) : undefined), [isDesignHintShown]);

    const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
    const [drawerPath, setDrawerPath] = useState<string | null>(null);
    const isMobileMenuOpen = drawerPath === location.pathname;
    const closeDrawer = () => setDrawerPath(null);
    const menuButtonRef = useRef<HTMLButtonElement>(null);
    const drawerRef = useRef<HTMLDivElement>(null);

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

    const externalEntries: NavEntry[] = [
        { key: 'docs', href: getEnv('VITE_DOCS_URL'), label: 'Documentation', icon: Book },
        { key: 'github', href: getEnv('VITE_GITHUB_URL'), label: 'Source code', icon: FolderGit2 },
        { key: 'status', href: getEnv('VITE_STATUS_URL'), label: 'Status', icon: Activity },
        { key: 'tos', href: getEnv('VITE_TOS_URL'), label: 'Terms of Service', icon: FileText },
    ].filter((entry) => !!safeExternalUrl(entry.href));

    const navGroups: NavGroup[] = [
        {
            label: 'Browse',
            entries: [
                { key: 'home', to: '/', label: 'Home', icon: Home },
                { key: 'gallery', to: '/gallery', label: 'Gallery', icon: ImageIcon },
                { key: 'albums', to: '/albums', label: 'Albums', icon: Library },
                { key: 'tags', to: '/tags', label: 'Tags', icon: TagIcon },
                { key: 'artists', to: '/artists', label: 'Artists', icon: Palette },
            ],
        },
        {
            label: 'Contribute',
            entries: [{ key: 'upload', to: '/upload', label: 'Upload', icon: UploadIcon }],
        },
        {
            label: 'About',
            entries: [
                ...externalEntries,
                { key: 'imprint', to: '/imprint', label: 'Imprint', icon: Scale },
                { key: 'privacy', to: '/privacy', label: 'Privacy', icon: ShieldCheck },
                ...(showContact ? [{ key: 'contact', to: '/contact', label: 'Contact', icon: Mail }] : []),
            ],
        },
    ];

    const isModOrAdmin = user && (user.role === Role.Moderator || user.role === Role.Admin);
    const isAdmin = user && user.role === Role.Admin;
    const { canManageBanner } = usePermissions();

    return (
        <div className="flex h-dvh flex-col overflow-hidden bg-background text-foreground">
            <a
                href="#main-content"
                onClick={(e) => {
                    e.preventDefault();
                    document.getElementById('main-content')?.focus();
                }}
                className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-200 focus:rounded-xl focus:bg-primary focus:px-4 focus:py-2 focus:text-sm focus:font-semibold focus:text-primary-foreground focus:shadow-float"
            >
                Skip to content
            </a>
            <GlobalErrorHandler />

            <div className="relative flex min-h-0 flex-1 overflow-hidden">
                <aside
                    className={`hidden shrink-0 flex-col border-r border-border bg-background transition-[width] duration-300 ease-out lg:flex ${isSidebarCollapsed ? 'w-[4.5rem]' : 'w-60'}`}
                >
                    <Link
                        to="/"
                        aria-label={`${appTitle} home`}
                        className={`flex h-16 shrink-0 items-center gap-3 border-b border-border ${isSidebarCollapsed ? 'justify-center px-0' : 'px-5'}`}
                    >
                        <img
                            src="/logo-96.png?v=20261024-logo-v2"
                            srcSet="/logo-96.png?v=20261024-logo-v2 2x, /logo-144.png?v=20261024-logo-v2 3x"
                            alt=""
                            width={36}
                            height={36}
                            className="size-9 shrink-0 rounded-[0.7rem] object-cover ring-1 ring-border"
                        />
                        {!isSidebarCollapsed && <span className="truncate text-[1.0625rem] font-extrabold tracking-[-0.01em]">{appTitle}</span>}
                    </Link>

                    <nav aria-label="Main navigation" className="flex-1 overflow-y-auto px-3 py-3">
                        <NavList groups={navGroups} collapsed={isSidebarCollapsed} pathname={location.pathname} />
                    </nav>

                    <div className={`flex shrink-0 items-center gap-2 border-t border-border px-3 py-3 ${isSidebarCollapsed ? 'flex-col' : ''}`}>
                        {appVersion && (
                            <p className={`select-none text-xs text-muted-foreground tabular ${isSidebarCollapsed ? 'text-center' : 'flex-1 pl-3'}`}>
                                {isSidebarCollapsed ? `v${appVersion}` : `Version ${appVersion}`}
                            </p>
                        )}
                        <button
                            type="button"
                            aria-label={isSidebarCollapsed ? 'Expand navigation menu' : 'Collapse navigation menu'}
                            aria-expanded={!isSidebarCollapsed}
                            onClick={() => setIsSidebarCollapsed(!isSidebarCollapsed)}
                            className={`grid size-9 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground ${appVersion || isSidebarCollapsed ? '' : 'ml-auto'}`}
                        >
                            {isSidebarCollapsed ? <PanelLeftOpen size={18} aria-hidden="true" /> : <PanelLeftClose size={18} aria-hidden="true" />}
                        </button>
                    </div>
                </aside>

                {isMobileMenuOpen && (
                    <div className="fixed inset-0 z-50 lg:hidden">
                        <div className="absolute inset-0 bg-zinc-950/55 backdrop-blur-[2px] animate-in fade-in" aria-hidden="true" onClick={closeDrawer} />
                        <div
                            ref={drawerRef}
                            id="mobile-navigation"
                            role="dialog"
                            aria-modal="true"
                            aria-label="Navigation menu"
                            className="absolute inset-y-0 left-0 flex w-[min(20rem,88vw)] flex-col border-r border-border bg-background shadow-float animate-in slide-in-from-left duration-300"
                        >
                            <div className="flex h-16 shrink-0 items-center justify-between border-b border-border px-4">
                                <span className="text-[1.0625rem] font-extrabold tracking-[-0.01em]">{appTitle}</span>
                                <HeaderIconButton aria-label="Close navigation menu" onClick={closeDrawer}>
                                    <X size={20} aria-hidden="true" />
                                </HeaderIconButton>
                            </div>
                            <nav aria-label="Main navigation" className="flex-1 overflow-y-auto overscroll-contain px-3 py-3">
                                <NavList groups={navGroups} collapsed={false} pathname={location.pathname} onNavigate={closeDrawer} />
                            </nav>
                            {appVersion && <p className="border-t border-border px-6 py-3 text-xs text-muted-foreground tabular">Version {appVersion}</p>}
                        </div>
                    </div>
                )}

                <main id="main-content" tabIndex={-1} className="relative flex-1 overflow-y-auto overflow-x-hidden bg-canvas outline-hidden">
                    <header className="sticky top-0 z-40 flex h-16 w-full items-center justify-between gap-3 border-b border-border bg-background/85 px-3 backdrop-blur-md supports-[backdrop-filter]:bg-background/70 sm:px-4 lg:px-6">
                        <div className="flex min-w-0 items-center gap-2">
                            <button
                                ref={menuButtonRef}
                                type="button"
                                aria-label="Open navigation menu"
                                aria-expanded={isMobileMenuOpen}
                                aria-controls="mobile-navigation"
                                className="grid size-10 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground lg:hidden"
                                onClick={() => setDrawerPath(isMobileMenuOpen ? null : location.pathname)}
                            >
                                <Menu size={22} aria-hidden="true" />
                            </button>

                            <Link to="/" aria-label={`${appTitle} home`} className="flex min-w-0 items-center gap-2.5 lg:hidden">
                                <img
                                    src="/logo-96.png?v=20261024-logo-v2"
                                    srcSet="/logo-96.png?v=20261024-logo-v2 2x, /logo-144.png?v=20261024-logo-v2 3x"
                                    alt={`${appTitle} logo`}
                                    width={36}
                                    height={36}
                                    className="size-9 shrink-0 rounded-[0.7rem] object-cover ring-1 ring-border"
                                />
                                <span className="hidden truncate text-[1.0625rem] font-extrabold tracking-[-0.01em] sm:block">{appTitle}</span>
                            </Link>
                        </div>

                        <div className="flex items-center gap-1.5 sm:gap-2">
                            <Dropdown
                                width="w-[17rem]"
                                label="Change color theme, accent, design and effects"
                                trigger={
                                    <DropdownTrigger className="theme-menu-trigger grid size-10 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground">
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
                                        <DropdownItem key={id} onClick={() => switchDesign(id)} active={design === id} icon={id === 'collector' ? <Sparkles size={16} /> : id === 'manga' ? <BookOpen size={16} /> : <History size={16} />}>{label}</DropdownItem>
                                    ))}
                                </div>
                                <DropdownSeparator />
                                <div role="group" aria-label="Effects">
                                    <DropdownLabel>Effects</DropdownLabel>
                                    <DropdownItem checkbox onClick={() => setMotionEnabled(!motionEnabled)} active={motionEnabled} icon={<WandSparkles size={16} />}>Animations and effects</DropdownItem>
                                </div>
                            </Dropdown>

                            {user ? (
                                <Dropdown
                                    width="w-64"
                                    label={`Account menu (${user.name})`}
                                    trigger={
                                        <DropdownTrigger className="flex max-w-[14rem] items-center gap-2.5 rounded-full border border-transparent py-1 pl-1 pr-2.5 transition-colors hover:border-border hover:bg-secondary">
                                            {user.avatarUrl ? (
                                                <img src={user.avatarUrl} alt="" referrerPolicy="no-referrer" className="size-8 shrink-0 rounded-full object-cover ring-1 ring-border" />
                                            ) : (
                                                <span className="grid size-8 shrink-0 place-items-center rounded-full bg-brand-soft text-brand-ink" aria-hidden="true">
                                                    <UserIcon size={16} />
                                                </span>
                                            )}
                                            <span className="hidden min-w-0 text-left leading-tight md:block">
                                                <span className="block max-w-[8rem] truncate text-sm font-semibold">{user.name}</span>
                                                <span className="block text-xs text-muted-foreground">{roleLabel(user.role)}</span>
                                            </span>
                                            <ChevronDown size={15} aria-hidden="true" className="shrink-0 text-muted-foreground" />
                                        </DropdownTrigger>
                                    }
                                >
                                    <div className="border-b border-border px-3 pb-2.5 pt-1.5 md:hidden">
                                        <p className="truncate font-semibold">{user.name}</p>
                                        <p className="text-xs text-muted-foreground">{roleLabel(user.role)}</p>
                                    </div>

                                    <DropdownLabel>Your content</DropdownLabel>
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
                                <div className="size-9 animate-pulse rounded-full bg-muted" aria-hidden="true" />
                            ) : (
                                <Link to="/login" className="btn btn-primary btn-sm px-4">
                                    Login
                                </Link>
                            )}
                        </div>
                    </header>
                    <SessionErrorBanner />
                    <SiteBanner />
                    <ErrorBoundary resetKey={location.pathname}>
                        <Suspense fallback={<PageLoader />}>
                            <Outlet />
                        </Suspense>
                    </ErrorBoundary>
                </main>
            </div>
        </div>
    );
};

export default Layout;
