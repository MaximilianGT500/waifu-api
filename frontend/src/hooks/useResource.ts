import { useState, useEffect, useCallback, useEffectEvent, Dispatch, SetStateAction, useRef } from 'react';
import { useSearchParams } from 'react-router';
import api from '../services/api';
import { PaginatedList } from '../types';
import { useNotification } from '../context/NotificationContext';

export interface UseResourceResult<T> {
    items: T[];
    loading: boolean;
    page: number;
    setPage: Dispatch<SetStateAction<number>>;
    totalPages: number;
    search: string;
    setSearch: (value: string) => void;
    searchType: 'name' | 'id';
    setSearchType: (type: 'name' | 'id') => void;
    createItem: (data: unknown, successMessage?: string) => Promise<boolean>;
    updateItem: (id: number | string, data: unknown, successMessage?: string) => Promise<boolean>;
    deleteItem: (id: number | string, successMessage?: string) => Promise<boolean>;
    refresh: () => Promise<void>;
}

export function useResource<T extends { id: number | string }>(
    endpoint: string,
    pageSize: number = 30,
): UseResourceResult<T> {
    const { showNotification } = useNotification();
    const [searchParams, setSearchParams] = useSearchParams();

    const [items, setItems] = useState<T[]>([]);
    const [loading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);

    const [search, _setSearch] = useState(searchParams.get('name') || searchParams.get('includedIds') || '');
    const [searchType, _setSearchType] = useState<'name' | 'id'>(searchParams.get('includedIds') ? 'id' : 'name');

    const ignoreUrlSync = useRef(false);

    const requestSeq = useRef(0);

    const setSearch = (value: string) => {
        if (value !== search) {
            ignoreUrlSync.current = true;
            _setSearch(value);
        }
    };

    const setSearchType = (type: 'name' | 'id') => {
        if (type !== searchType) {
            ignoreUrlSync.current = true;
            _setSearchType(type);
        }
    };

    const [debouncedSearch, setDebouncedSearch] = useState(search);

    useEffect(() => {
        if (search === '') {
            setDebouncedSearch('');
            return;
        }

        const handler = setTimeout(() => {
            setDebouncedSearch(search);
        }, 500);

        return () => {
            clearTimeout(handler);
        };
    }, [search]);

    const fetchData = useCallback(async () => {
        if (searchType === 'id' && debouncedSearch && !/^\d+$/.test(debouncedSearch)) {
            return;
        }

        const seq = ++requestSeq.current;
        setLoading(true);
        try {
            const params: Record<string, string | number> = { page, pageSize };

            if (debouncedSearch) {
                if (searchType === 'id') {
                    if (/^\d+$/.test(debouncedSearch)) {
                        params.includedIds = debouncedSearch;
                    }
                } else {
                    params.name = debouncedSearch;
                }
            }

            const { data } = await api.get<PaginatedList<T>>(endpoint, { params });
            if (seq !== requestSeq.current) return;
            setItems(data.items);
            setTotalPages(data.totalPages);
        } catch {
            return;
        } finally {
            if (seq === requestSeq.current) {
                setLoading(false);
            }
        }
    }, [endpoint, page, pageSize, debouncedSearch, searchType]);

    useEffect(() => {
        setPage(1);
    }, [debouncedSearch, searchType]);

    const syncUrl = useEffectEvent((value: string, type: 'name' | 'id') => {
        const currentParams = new URLSearchParams(searchParams);
        const currentNameParam = currentParams.get('name') || '';
        const currentIdParam = currentParams.get('includedIds') || '';

        if (value) {
            if (type === 'id') {
                if (/^\d+$/.test(value) && (currentIdParam !== value || currentParams.has('name'))) {
                    currentParams.set('includedIds', value);
                    currentParams.delete('name');
                    setSearchParams(currentParams, { replace: true });
                }
            } else if (currentNameParam !== value || currentParams.has('includedIds')) {
                currentParams.set('name', value);
                currentParams.delete('includedIds');
                setSearchParams(currentParams, { replace: true });
            }
        } else if (currentNameParam || currentIdParam) {
            currentParams.delete('name');
            currentParams.delete('includedIds');
            setSearchParams(currentParams, { replace: true });
        }
    });

    useEffect(() => {
        syncUrl(debouncedSearch, searchType);
    }, [debouncedSearch, searchType]);

    const urlName = searchParams.get('name') || '';
    const urlId = searchParams.get('includedIds') || '';
    const urlSearch = urlId || urlName;

    useEffect(() => {
        if (ignoreUrlSync.current) {
            ignoreUrlSync.current = false;
            return;
        }

        if (urlSearch !== search) {
            _setSearch(urlSearch);
        }

        if (urlId && searchType !== 'id') {
            _setSearchType('id');
        } else if (urlName && searchType !== 'name') {
            _setSearchType('name');
        }
    }, [urlSearch, urlId, urlName, searchType, search]);

    useEffect(() => {
        fetchData();
    }, [fetchData]);

    const createItem = async (data: unknown, successMessage = 'Created successfully') => {
        try {
            await api.post(endpoint, data);
            showNotification('success', successMessage);
            fetchData();
            return true;
        } catch {
            return false;
        }
    };

    const updateItem = async (id: number | string, data: unknown, successMessage = 'Updated successfully') => {
        try {
            await api.patch(`${endpoint}/${id}`, data);
            showNotification('success', successMessage);
            fetchData();
            return true;
        } catch {
            return false;
        }
    };

    const deleteItem = async (id: number | string, successMessage = 'Deleted successfully') => {
        try {
            await api.delete(`${endpoint}/${id}`);
            showNotification('success', successMessage);
            fetchData();
            return true;
        } catch {
            return false;
        }
    };

    return {
        items,
        loading,
        page,
        setPage,
        totalPages,
        search,
        setSearch,
        searchType,
        setSearchType,
        createItem,
        updateItem,
        deleteItem,
        refresh: fetchData
    };
}