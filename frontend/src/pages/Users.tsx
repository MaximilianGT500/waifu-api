import { useState } from 'react';
import api from '../services/api';
import { User, Role } from '../types';
import { useNotification } from '../context/NotificationContext';
import { Shield, Ban, CircleCheck, Upload, Image as ImagesIcon, Info } from 'lucide-react';
import StatusBadge from '../components/StatusBadge';
import { Link } from 'react-router';
import ConfirmModal from '../components/modals/ConfirmModal';
import ReasonModal from '../components/modals/ReasonModal';
import SearchableSelect from '../components/SearchableSelect';
import SearchInput from '../components/SearchInput';
import Pagination from '../components/Pagination';
import { useResource } from '../hooks/useResource';
import { useRequireAuth } from '../hooks/useRequireAuth';

const Users = () => {
    const user = useRequireAuth();
    const { showNotification } = useNotification();

    const {
        items: users,
        loading,
        page,
        setPage,
        totalPages,
        search,
        setSearch,
        searchType,
        setSearchType,
        refresh
    } = useResource<User>('/users', 20);

    const [selectedUser, setSelectedUser] = useState<User | null>(null);
    const [isBanModalOpen, setIsBanModalOpen] = useState(false);
    const [isUnbanModalOpen, setIsUnbanModalOpen] = useState(false);
    const [reasonPopoverId, setReasonPopoverId] = useState<number | null>(null);

    const handleRoleChange = async (userId: number, newRole: string) => {
        try {
            await api.put(`/users/${userId}/role`, { role: newRole });
            showNotification('success', 'User role updated');
            refresh();
        } catch {
            return;
        }
    };

    const handleBan = async (reason: string) => {
        if (!selectedUser) return;
        try {
            await api.put(`/users/${selectedUser.id}/ban`, {
                isBlacklisted: true,
                reason: reason || undefined
            });
            showNotification('success', `User banned`);
            setIsBanModalOpen(false);
            refresh();
        } catch {
            return;
        }
    };

    const handleUnban = async () => {
        if (!selectedUser) return;
        try {
            await api.put(`/users/${selectedUser.id}/ban`, { isBlacklisted: false });
            showNotification('success', `User unbanned`);
            setIsUnbanModalOpen(false);
            refresh();
        } catch {
            return;
        }
    };

    const openBanModal = (account: User) => {
        setSelectedUser(account);
        if (account.isBlacklisted) {
            setIsUnbanModalOpen(true);
        } else {
            setIsBanModalOpen(true);
        }
    };

    const roleOptions = [
        { id: Role.User.toString(), name: 'User' },
        { id: Role.TrustedUser.toString(), name: 'Trusted' },
        { id: Role.Moderator.toString(), name: 'Moderator' },
        { id: Role.Admin.toString(), name: 'Admin' }
    ];

    if (!user) return null;

    const th = 'px-4 py-3 text-left text-xs font-semibold text-muted-foreground whitespace-nowrap';
    const td = 'px-4 py-3 align-middle';

    return (
        <div className="page">
            <div className="page-header">
                <div>
                    <h1 className="page-title">User Management</h1>
                    <p className="page-lede">Roles, bans and request counts of every account.</p>
                </div>

                <SearchInput
                    value={search}
                    onChange={setSearch}
                    searchType={searchType}
                    onSearchTypeChange={setSearchType}
                    className="w-full md:w-80"
                />
            </div>

            <div className="panel overflow-hidden">
                <div className="min-h-[24rem] overflow-x-auto">
                    <table className="w-full border-collapse text-left text-sm">
                        <thead>
                        <tr className="border-b border-border bg-secondary/40">
                            <th scope="col" className={th}>ID</th>
                            <th scope="col" className={th}>User</th>
                            <th scope="col" className={th}>Discord ID</th>
                            <th scope="col" className={th}>Uploads</th>
                            <th scope="col" className={th}>Album images</th>
                            <th scope="col" className={th}>Requests</th>
                            <th scope="col" className={th}>Via API key</th>
                            <th scope="col" className={th}>Via web</th>
                            <th scope="col" className={`${th} w-44`}>Role</th>
                            <th scope="col" className={th}>Status</th>
                            <th scope="col" className={`${th} text-right`}><span className="sr-only">Actions</span></th>
                        </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                        {loading ? (
                            <tr>
                                <td colSpan={11} className="space-y-3 px-4 py-3">
                                    <span className="sr-only">Loading users…</span>
                                    {[...Array(5)].map((_, i) => <div key={i} className="h-9 animate-pulse rounded-full bg-sleeve" aria-hidden="true" />)}
                                </td>
                            </tr>
                        ) : users.map(account => (
                            <tr key={account.id} className="transition-colors hover:bg-secondary/40">
                                <td className={`${td} tabular text-muted-foreground`}>{account.id}</td>
                                <td className={`${td} font-semibold`}>{account.name}</td>
                                <td className={`${td} font-mono text-xs text-muted-foreground`}>{account.discordId}</td>
                                <td className={td}>
                                    <Link to={`/gallery?uploaderId=${account.id}`} className="inline-flex items-center gap-1.5 font-semibold text-brand-ink tabular hover:underline" title="View uploaded images">
                                        <Upload size={13} aria-hidden="true" />
                                        {account.uploadedImageCount?.toLocaleString() || 0}
                                    </Link>
                                </td>
                                <td className={td}>
                                    <Link to={`/users/${account.id}/albums`} className="inline-flex items-center gap-1.5 font-semibold text-brand-ink tabular hover:underline" title="View the user's albums">
                                        <ImagesIcon size={13} aria-hidden="true" />
                                        {account.albumImageCount?.toLocaleString() || 0}
                                    </Link>
                                </td>
                                <td className={`${td} tabular`}>{account.requestCount?.toLocaleString() || 0}</td>
                                <td className={`${td} tabular text-muted-foreground`}>{account.apiKeyRequestCount?.toLocaleString() || 0}</td>
                                <td className={`${td} tabular text-muted-foreground`}>{account.jwtRequestCount?.toLocaleString() || 0}</td>
                                <td className={td}>
                                    <div className="w-40">
                                        <SearchableSelect
                                            options={roleOptions}
                                            selectedOptions={roleOptions.filter(r => r.id === account.role.toString())}
                                            onSelect={(opt) => handleRoleChange(account.id, opt.id as string)}
                                            onRemove={() => {}}
                                            isMulti={false}
                                            clearable={false}
                                            placeholder={`Role of ${account.name}`}
                                        />
                                    </div>
                                </td>
                                <td className={td}>
                                    {account.isBlacklisted ? (
                                        <div className="max-w-56">
                                            <div className="flex items-center gap-1">
                                                <StatusBadge label="Banned" tone="danger" icon={<Ban size={11} aria-hidden="true" />} />
                                                {account.blacklistReason && (
                                                    <button
                                                        type="button"
                                                        onClick={() => setReasonPopoverId(reasonPopoverId === account.id ? null : account.id)}
                                                        aria-expanded={reasonPopoverId === account.id}
                                                        className="grid size-7 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                                                        title="Show the ban reason"
                                                        aria-label={`Ban reason for ${account.name}`}
                                                    >
                                                        <Info size={14} aria-hidden="true" />
                                                    </button>
                                                )}
                                            </div>
                                            {reasonPopoverId === account.id && account.blacklistReason && (
                                                <p className="mt-1.5 text-xs text-muted-foreground wrap-break-word">{account.blacklistReason}</p>
                                            )}
                                        </div>
                                    ) : (
                                        <StatusBadge label="Active" tone="success" icon={<CircleCheck size={11} aria-hidden="true" />} />
                                    )}
                                </td>
                                <td className={`${td} text-right`}>
                                    <button
                                        type="button"
                                        onClick={() => openBanModal(account)}
                                        className={`btn btn-sm ${account.isBlacklisted ? 'btn-secondary' : 'text-destructive hover:bg-destructive/10'}`}
                                        aria-label={account.isBlacklisted ? `Unban ${account.name}` : `Ban ${account.name}`}
                                    >
                                        {account.isBlacklisted ? <Shield size={14} aria-hidden="true" /> : <Ban size={14} aria-hidden="true" />}
                                        {account.isBlacklisted ? 'Unban' : 'Ban'}
                                    </button>
                                </td>
                            </tr>
                        ))}
                        </tbody>
                    </table>
                </div>
            </div>

            {!loading && (
                <Pagination currentPage={page} totalPages={totalPages} setPage={setPage} />
            )}

            <ReasonModal
                isOpen={isBanModalOpen}
                onClose={() => setIsBanModalOpen(false)}
                onSubmit={handleBan}
                title="Ban User"
                description={`Ban ${selectedUser?.name}? The account loses access to the site and the API. The reason is shown to the user.`}
                placeholder="Reason for banning (optional)..."
                submitText="Ban"
                required={false}
                maxLength={200}
            />

            <ConfirmModal
                isOpen={isUnbanModalOpen}
                onClose={() => setIsUnbanModalOpen(false)}
                onConfirm={handleUnban}
                title="Unban User"
                message={<p>Unban <strong className="text-foreground">{selectedUser?.name}</strong>? The account can log in and use the API again.</p>}
                confirmText="Unban"
                variant="warning"
            />
        </div>
    );
};

export default Users;
