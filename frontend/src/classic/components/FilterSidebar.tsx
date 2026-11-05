import { useEffect, useState, useRef, useId } from 'react';
import SearchableSelect, { Option } from './SearchableSelect';
import api from '../../services/api';
import { PaginatedList, Tag, Artist, User, NsfwMode, AnimatedMode, Orientation, Role, ImageOrderBy, ReviewStatusFilter } from '../../types';
import { X } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { resolveTagsBySlugs, resolveArtistsByIds } from '../../utils/metadataResolve';

interface FilterSidebarProps {
    searchParams: URLSearchParams;
    setSearchParams: (params: URLSearchParams | ((prev: URLSearchParams) => URLSearchParams)) => void;
    showFilters: boolean;
    setShowFilters: (show: boolean) => void;
    sortOptions: Option[];
}

const FilterSidebar = ({ searchParams, setSearchParams, showFilters, setShowFilters, sortOptions }: FilterSidebarProps) => {
    const { user } = useAuth();
    const isModeratorOrAdmin = user?.role === Role.Moderator || user?.role === Role.Admin;

    const tagsPageSize = 30;
    const artistsPageSize = 30;
    const usersPageSize = 30;
    const [initialTags, setInitialTags] = useState<Option[]>([]);
    const [initialArtists, setInitialArtists] = useState<Option[]>([]);
    const [initialUsers, setInitialUsers] = useState<Option[]>([]);
    const [initialLoadDone, setInitialLoadDone] = useState(false);
    const fetchedArtistIds = useRef<Set<string>>(new Set());
    const fetchedTagSlugs = useRef<Set<string>>(new Set());
    const fetchedUserIds = useRef<Set<string>>(new Set());

    const idPrefix = useId();
    const [includedIdInput, setIncludedIdInput] = useState('');
    const [excludedIdInput, setExcludedIdInput] = useState('');

    useEffect(() => {
        const loadInitialData = async () => {
            try {
                const [tagsRes, artistsRes] = await Promise.all([
                    api.get<PaginatedList<Tag>>('/tags', { params: { pageSize: tagsPageSize } }),
                    api.get<PaginatedList<Artist>>('/artists', { params: { pageSize: artistsPageSize } })
                ]);

                setInitialTags(tagsRes.data.items.map(t => ({ id: t.id, name: t.name, slug: t.slug, reviewStatus: t.reviewStatus })));
                setInitialArtists(artistsRes.data.items.map(a => ({ id: a.id, name: a.name, reviewStatus: a.reviewStatus })));

                if (isModeratorOrAdmin) {
                    const usersRes = await api.get<PaginatedList<User>>('/users', { params: { pageSize: usersPageSize } });
                    setInitialUsers(usersRes.data.items.map(u => ({ id: u.id, name: u.name })));
                }
            } catch (error) {
                console.error(error);
            } finally {
                setInitialLoadDone(true);
            }
        };

        loadInitialData();
    }, [isModeratorOrAdmin]);

    useEffect(() => {
        if (!initialLoadDone) return;

        const artistIdsFromUrl = [...searchParams.getAll('includedArtists'), ...searchParams.getAll('excludedArtists')];
        const missingIds = artistIdsFromUrl.filter(id =>
            !initialArtists.some(a => a.id.toString() === id) && !fetchedArtistIds.current.has(id)
        );

        if (missingIds.length > 0) {
            missingIds.forEach(id => fetchedArtistIds.current.add(id));

            resolveArtistsByIds(missingIds.map(Number)).then(artists => {
                const newArtists = artists.map(a => ({ id: a.id, name: a.name, reviewStatus: a.reviewStatus } as Option));
                if (newArtists.length > 0) {
                    setInitialArtists(prev => [...prev, ...newArtists]);
                }
            });
        }
    }, [searchParams, initialArtists, initialLoadDone]);

    useEffect(() => {
        if (!initialLoadDone) return;

        const tagSlugsFromUrl = [...searchParams.getAll('includedTags'), ...searchParams.getAll('excludedTags')];
        const missingSlugs = tagSlugsFromUrl.filter(slug =>
            !initialTags.some(t => t.slug === slug) && !fetchedTagSlugs.current.has(slug)
        );

        if (missingSlugs.length > 0) {
            missingSlugs.forEach(slug => fetchedTagSlugs.current.add(slug));

            resolveTagsBySlugs(missingSlugs).then(tags => {
                const newTags = tags.map(t => ({ id: t.id, name: t.name, slug: t.slug, reviewStatus: t.reviewStatus } as Option));
                if (newTags.length > 0) {
                    setInitialTags(prev => [...prev, ...newTags]);
                }
            });
        }
    }, [searchParams, initialTags, initialLoadDone]);

    useEffect(() => {
        if (!initialLoadDone) return;

        const uploaderIdFromUrl = searchParams.get('uploaderId');
        if (uploaderIdFromUrl &&
            !initialUsers.some(u => u.id.toString() === uploaderIdFromUrl) &&
            !fetchedUserIds.current.has(uploaderIdFromUrl)) {

            fetchedUserIds.current.add(uploaderIdFromUrl);

            api.get<PaginatedList<User>>('/users', { params: { includedIds: [uploaderIdFromUrl] } })
                .then(res => {
                    if (res.data.items.length > 0) {
                        const uploader = res.data.items[0];
                        setInitialUsers(prev => [...prev, { id: uploader.id, name: uploader.name } as Option]);
                    }
                })
                .catch(() => {});
        }
    }, [searchParams, initialUsers, initialLoadDone]);

    const loadTags = async (query: string, page: number = 1) => {
        const { data } = await api.get<PaginatedList<Tag>>('/tags', { params: { name: query, pageSize: tagsPageSize, page } });
        return data.items.map(t => ({ id: t.id, name: t.name, slug: t.slug, description: t.description, reviewStatus: t.reviewStatus }));
    };

    const loadArtists = async (query: string, page: number = 1) => {
        const { data } = await api.get<PaginatedList<Artist>>('/artists', { params: { name: query, pageSize: artistsPageSize, page } });
        return data.items.map(a => ({ id: a.id, name: a.name, reviewStatus: a.reviewStatus }));
    };

    const loadUsers = async (query: string, page: number = 1) => {
        const params: Record<string, string | number> = { pageSize: usersPageSize, page };
        if (query) {
            if (/^\d+$/.test(query)) {
                params.includedIds = query;
            } else {
                params.name = query;
            }
        }
        const { data } = await api.get<PaginatedList<User>>('/users', { params });
        return data.items.map(u => ({ id: u.id, name: u.name }));
    };

    const updateFilter = (key: string, value: string | null) => {
        setSearchParams(prev => {
            if (value) prev.set(key, value);
            else prev.delete(key);
            return prev;
        });
    };

    const handleTagChange = (key: 'includedTags' | 'excludedTags', tag: Option, action: 'add' | 'remove') => {
        const value = tag.slug || tag.id.toString();

        if (action === 'add' && !initialTags.some(t => t.slug === value)) {
            setInitialTags(prev => [...prev, tag]);
        }

        setSearchParams(prev => {
            const current = prev.getAll(key);
            prev.delete(key);
            if (action === 'add' && !current.includes(value)) {
                [...current, value].forEach(t => prev.append(key, t));
            } else if (action === 'remove') {
                current.filter(t => t !== value).forEach(t => prev.append(key, t));
            } else {
                current.forEach(t => prev.append(key, t));
            }
            return prev;
        });
    };

    const handleArtistChange = (key: 'includedArtists' | 'excludedArtists', artist: Option, action: 'add' | 'remove') => {
        const value = artist.id.toString();

        if (action === 'add' && !initialArtists.some(a => a.id.toString() === value)) {
            setInitialArtists(prev => [...prev, artist]);
        }

        setSearchParams(prev => {
            const current = prev.getAll(key);
            prev.delete(key);
            if (action === 'add' && !current.includes(value)) {
                [...current, value].forEach(t => prev.append(key, t));
            } else if (action === 'remove') {
                current.filter(t => t !== value).forEach(t => prev.append(key, t));
            } else {
                current.forEach(t => prev.append(key, t));
            }
            return prev;
        });
    };

    const handleIdChange = (key: 'includedIds' | 'excludedIds', id: string, action: 'add' | 'remove') => {
        setSearchParams(prev => {
            const current = prev.getAll(key);
            prev.delete(key);
            if (action === 'add' && !current.includes(id)) {
                [...current, id].forEach(t => prev.append(key, t));
            } else if (action === 'remove') {
                current.filter(t => t !== id).forEach(t => prev.append(key, t));
            } else {
                current.forEach(t => prev.append(key, t));
            }
            return prev;
        });
    };

    const handleIdKeyDown = (e: React.KeyboardEvent<HTMLInputElement>, key: 'includedIds' | 'excludedIds', input: string, setInput: (val: string) => void) => {
        if (e.key === 'Enter' && input.trim()) {
            e.preventDefault();
            if (!/^\d+$/.test(input.trim())) {
                return;
            }
            handleIdChange(key, input.trim(), 'add');
            setInput('');
        }
    };

    const handleIdInputChange = (e: React.ChangeEvent<HTMLInputElement>, setInput: (val: string) => void) => {
        const val = e.target.value;
        if (val === '' || /^\d+$/.test(val)) {
            setInput(val);
        }
    };

    const orderBy = searchParams.get('orderBy') || ImageOrderBy.UploadedAt;
    const isNsfw = searchParams.get('isNsfw') || NsfwMode.False;
    const orientation = searchParams.get('orientation') || Orientation.All;
    const isAnimated = searchParams.get('isAnimated') || AnimatedMode.All;
    const reviewStatus = searchParams.get('reviewStatus') || ReviewStatusFilter.Accepted;

    const includedTagSlugs = searchParams.getAll('includedTags');
    const excludedTagSlugs = searchParams.getAll('excludedTags');
    const includedArtistIds = searchParams.getAll('includedArtists');
    const excludedArtistIds = searchParams.getAll('excludedArtists');
    const includedIds = searchParams.getAll('includedIds');
    const excludedIds = searchParams.getAll('excludedIds');
    const uploaderId = searchParams.get('uploaderId') || '';
    const width = searchParams.get('width') || '';
    const height = searchParams.get('height') || '';
    const byteSize = searchParams.get('byteSize') || '';

    const getSelectedTags = (slugs: string[]) => {
        return slugs.map(slug => {
            const found = initialTags.find(t => t.slug === slug);
            return found || { id: slug, name: slug, slug: slug };
        });
    };

    const getSelectedArtists = (ids: string[]) => {
        return ids.map(id => {
            const found = initialArtists.find(a => a.id.toString() === id);
            return found || { id: id, name: `ID: ${id}` };
        });
    };

    const getSelectedUploader = (id: string): Option[] => {
        if (!id) return [];
        const found = initialUsers.find(u => u.id.toString() === id);
        return [found || { id: id, name: `ID: ${id}` }];
    };

    const ratingOptions = [
        { id: NsfwMode.False, name: 'Safe' },
        { id: NsfwMode.True, name: 'NSFW (18+)' },
        { id: NsfwMode.All, name: 'All Content' }
    ];

    const orientationOptions = [
        { id: Orientation.All, name: 'Any' },
        { id: Orientation.Landscape, name: 'Landscape' },
        { id: Orientation.Portrait, name: 'Portrait' },
        { id: Orientation.Square, name: 'Square' }
    ];

    const animationOptions = [
        { id: AnimatedMode.All, name: 'Any' },
        { id: AnimatedMode.False, name: 'Static Image' },
        { id: AnimatedMode.True, name: 'Animated (GIF)' }
    ];

    const reviewStatusOptions = [
        { id: ReviewStatusFilter.Accepted, name: 'Accepted' },
        { id: ReviewStatusFilter.Pending, name: 'Pending' },
        { id: ReviewStatusFilter.All, name: 'All' }
    ];

    const idChips = (key: 'includedIds' | 'excludedIds', ids: string[], tone: string) => ids.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
            {ids.map(id => (
                <span key={id} className={`inline-flex items-center gap-1 rounded-lg py-0.5 pl-2 pr-1 text-xs font-semibold tabular ${tone}`}>
                    #{id}
                    <button
                        type="button"
                        onClick={() => handleIdChange(key, id, 'remove')}
                        aria-label={`Remove ID ${id}`}
                        className="grid size-5 place-items-center rounded-md transition-colors hover:bg-foreground/10"
                    >
                        <X size={12} aria-hidden="true" />
                    </button>
                </span>
            ))}
        </div>
    );

    return (
        <aside
            aria-label="Filters"
            className={`
                fixed inset-0 z-60 flex flex-col bg-background transition-transform duration-300 ease-out
                md:sticky md:inset-auto md:top-16 md:z-30 md:h-[calc(100dvh-4rem)] md:shrink-0 md:translate-x-0 md:border-l md:border-border md:transition-[width]
                ${showFilters ? 'translate-x-0 md:w-80' : 'pointer-events-none translate-x-full md:pointer-events-auto md:w-0 md:overflow-hidden md:border-l-0'}
            `}
            inert={!showFilters || undefined}
        >
            <div className="flex h-16 shrink-0 items-center justify-between border-b border-border px-5 md:hidden">
                <h2 className="text-lg font-bold">Filters</h2>
                <button
                    type="button"
                    onClick={() => setShowFilters(false)}
                    aria-label="Close filters"
                    className="grid size-10 place-items-center rounded-full text-muted-foreground hover:bg-secondary hover:text-foreground"
                >
                    <X size={20} aria-hidden="true" />
                </button>
            </div>

            <div className="w-full flex-1 space-y-7 overflow-y-auto overscroll-contain px-5 py-6 md:w-80">
                <section className="space-y-4">
                    <SearchableSelect
                        label="Sort by"
                        options={sortOptions}
                        selectedOptions={sortOptions.filter(o => o.id === orderBy)}
                        onSelect={(o) => updateFilter('orderBy', o.id as string)}
                        onRemove={() => {}}
                        isMulti={false}
                        clearable={false}
                    />
                    <SearchableSelect
                        label="Content rating"
                        options={ratingOptions}
                        selectedOptions={ratingOptions.filter(o => o.id === isNsfw)}
                        onSelect={(o) => updateFilter('isNsfw', o.id as string)}
                        onRemove={() => { }}
                        isMulti={false}
                        clearable={false}
                    />
                </section>

                <section className="space-y-4 border-t border-border pt-6">
                    <SearchableSelect
                        label="Orientation"
                        options={orientationOptions}
                        selectedOptions={orientationOptions.filter(o => o.id === orientation)}
                        onSelect={(o) => updateFilter('orientation', o.id as string)}
                        onRemove={() => updateFilter('orientation', null)}
                        isMulti={false}
                    />
                    <SearchableSelect
                        label="Type"
                        options={animationOptions}
                        selectedOptions={animationOptions.filter(o => o.id === isAnimated)}
                        onSelect={(o) => updateFilter('isAnimated', o.id as string)}
                        onRemove={() => updateFilter('isAnimated', null)}
                        isMulti={false}
                    />
                    <fieldset>
                        <legend className="field-label">Dimensions in pixels</legend>
                        <div className="grid grid-cols-2 gap-2">
                            <input
                                type="text"
                                placeholder="Width, e.g. >1920"
                                aria-label="Width"
                                value={width}
                                onChange={e => updateFilter('width', e.target.value)}
                                className="input"
                            />
                            <input
                                type="text"
                                placeholder="Height, e.g. <1080"
                                aria-label="Height"
                                value={height}
                                onChange={e => updateFilter('height', e.target.value)}
                                className="input"
                            />
                        </div>
                    </fieldset>
                    <div>
                        <label htmlFor={`${idPrefix}-bytes`} className="field-label">File size in bytes</label>
                        <input
                            id={`${idPrefix}-bytes`}
                            type="text"
                            placeholder="e.g. <1000000"
                            value={byteSize}
                            onChange={e => updateFilter('byteSize', e.target.value)}
                            className="input"
                        />
                    </div>
                </section>

                <section className="space-y-4 border-t border-border pt-6">
                    <SearchableSelect
                        label="Include tags"
                        placeholder="Search tags…"
                        loadOptions={loadTags}
                        selectedOptions={getSelectedTags(includedTagSlugs)}
                        onSelect={(o) => handleTagChange('includedTags', o, 'add')}
                        onRemove={(o) => handleTagChange('includedTags', o, 'remove')}
                        isMulti={true}
                    />
                    <SearchableSelect
                        label="Exclude tags"
                        placeholder="Search tags…"
                        loadOptions={loadTags}
                        selectedOptions={getSelectedTags(excludedTagSlugs)}
                        onSelect={(o) => handleTagChange('excludedTags', o, 'add')}
                        onRemove={(o) => handleTagChange('excludedTags', o, 'remove')}
                        isMulti={true}
                    />
                </section>

                <section className="space-y-4 border-t border-border pt-6">
                    <SearchableSelect
                        label="Include artists"
                        placeholder="Search artists…"
                        loadOptions={loadArtists}
                        selectedOptions={getSelectedArtists(includedArtistIds)}
                        onSelect={(o) => handleArtistChange('includedArtists', o, 'add')}
                        onRemove={(o) => handleArtistChange('includedArtists', o, 'remove')}
                        isMulti={true}
                    />
                    <SearchableSelect
                        label="Exclude artists"
                        placeholder="Search artists…"
                        loadOptions={loadArtists}
                        selectedOptions={getSelectedArtists(excludedArtistIds)}
                        onSelect={(o) => handleArtistChange('excludedArtists', o, 'add')}
                        onRemove={(o) => handleArtistChange('excludedArtists', o, 'remove')}
                        isMulti={true}
                    />
                </section>

                {isModeratorOrAdmin && (
                    <section className="space-y-4 border-t border-border pt-6">
                        <SearchableSelect
                            label="Review status"
                            options={reviewStatusOptions}
                            selectedOptions={reviewStatusOptions.filter(o => o.id === reviewStatus)}
                            onSelect={(o) => updateFilter('reviewStatus', o.id as string)}
                            onRemove={() => updateFilter('reviewStatus', null)}
                            isMulti={false}
                            clearable={false}
                        />
                        <SearchableSelect
                            label="Uploader"
                            placeholder="Search users by name or ID…"
                            loadOptions={loadUsers}
                            selectedOptions={getSelectedUploader(uploaderId)}
                            onSelect={(o) => {
                                if (!initialUsers.some(u => u.id.toString() === o.id.toString())) {
                                    setInitialUsers(prev => [...prev, o]);
                                }
                                updateFilter('uploaderId', o.id.toString());
                            }}
                            onRemove={() => updateFilter('uploaderId', null)}
                            isMulti={false}
                        />
                    </section>
                )}

                <section className="space-y-4 border-t border-border pt-6">
                    <div>
                        <label htmlFor={`${idPrefix}-included-ids`} className="field-label">Include image IDs</label>
                        {idChips('includedIds', includedIds, 'bg-brand-soft text-brand-ink')}
                        <input
                            id={`${idPrefix}-included-ids`}
                            type="text"
                            inputMode="numeric"
                            placeholder="Type an ID and press Enter"
                            value={includedIdInput}
                            onChange={e => handleIdInputChange(e, setIncludedIdInput)}
                            onKeyDown={e => handleIdKeyDown(e, 'includedIds', includedIdInput, setIncludedIdInput)}
                            className="input"
                        />
                    </div>
                    <div>
                        <label htmlFor={`${idPrefix}-excluded-ids`} className="field-label">Exclude image IDs</label>
                        {idChips('excludedIds', excludedIds, 'bg-destructive/10 text-destructive')}
                        <input
                            id={`${idPrefix}-excluded-ids`}
                            type="text"
                            inputMode="numeric"
                            placeholder="Type an ID and press Enter"
                            value={excludedIdInput}
                            onChange={e => handleIdInputChange(e, setExcludedIdInput)}
                            onKeyDown={e => handleIdKeyDown(e, 'excludedIds', excludedIdInput, setExcludedIdInput)}
                            className="input"
                        />
                    </div>
                </section>
            </div>

            <div className="shrink-0 border-t border-border bg-background p-4 pb-[max(1rem,env(safe-area-inset-bottom))] md:hidden">
                <button type="button" onClick={() => setShowFilters(false)} className="btn btn-primary btn-lg w-full">Done</button>
            </div>
        </aside>
    );
};

export default FilterSidebar;