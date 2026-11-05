import { useState } from 'react';
import api, { httpStatus } from '../services/api';
import { Tag, Artist, PaginatedList, ReviewStatusFilter } from '../types';
import { Option } from '../components/SearchableSelect';
import { useNotification } from '../context/NotificationContext';
import { useAuth } from '../context/AuthContext';
import { canModerate } from '../utils/roles';
import { TagFormData } from '../components/modals/TagModal';
import { ArtistFormData } from '../components/modals/ArtistModal';

import { slugify } from '../utils/slug';

interface UseMetadataOptions {
    isReviewMode?: boolean;
    reviewStatus?: ReviewStatusFilter;
}

export const useMetadata = ({
    isReviewMode = false,
    reviewStatus = ReviewStatusFilter.Accepted,
}: UseMetadataOptions = {}) => {
    const tagsPageSize = 30;
    const artistsPageSize = 30;
    const { showNotification } = useNotification();
    const { user } = useAuth();
    const includeMine = user && !canModerate(user) ? true : undefined;

    const [selectedTags, setSelectedTags] = useState<Option[]>([]);
    const [showCreateTagModal, setShowCreateTagModal] = useState(false);
    const [newTagName, setNewTagName] = useState('');

    const [selectedArtists, setSelectedArtists] = useState<Option[]>([]);
    const [showCreateArtistModal, setShowCreateArtistModal] = useState(false);
    const [newArtistName, setNewArtistName] = useState('');

    const loadTags = async (query: string, page: number = 1) => {
        const { data } = await api.get<PaginatedList<Tag>>('/tags', {
            params: { name: query, pageSize: tagsPageSize, page, reviewStatus, includeMyPending: includeMine },
            skipGlobalErrorHandler: true
        });
        return data.items.map(t => ({ id: t.id, name: t.name, slug: t.slug, description: t.description, reviewStatus: t.reviewStatus }));
    };

    const loadArtists = async (query: string, page: number = 1) => {
        const { data } = await api.get<PaginatedList<Artist>>('/artists', {
            params: { name: query, pageSize: artistsPageSize, page, reviewStatus, includeMyPending: includeMine },
            skipGlobalErrorHandler: true
        });
        return data.items.map(a => ({ id: a.id, name: a.name, reviewStatus: a.reviewStatus }));
    };

    const handleCreateTag = async (data: TagFormData) => {
        try {
            const payload = { ...data, description: data.description || '' };
            const res = await api.post<Tag>('/tags', payload);
            const newOpt = { id: res.data.id, name: res.data.name, slug: res.data.slug, description: res.data.description, reviewStatus: res.data.reviewStatus };

            setSelectedTags(p => p.some(t => t.id === newOpt.id) ? p : [...p, newOpt]);
            showNotification(isReviewMode ? 'info' : 'success', isReviewMode ? 'Tag submitted for review' : 'Tag created');
            setShowCreateTagModal(false);
        } catch (err) {
            if (httpStatus(err) === 409) {
                try {
                    const targetSlug = data.slug || slugify(data.name);
                    const existingRes = await api.get<Tag>(`/tags/by-slug/${encodeURIComponent(targetSlug)}`, { skipGlobalErrorHandler: true });
                    const existing = existingRes.data;
                    const existingOpt = { id: existing.id, name: existing.name, slug: existing.slug, description: existing.description, reviewStatus: existing.reviewStatus };

                    setSelectedTags(p => p.some(t => t.id === existingOpt.id) ? p : [...p, existingOpt]);
                    showNotification('info', 'Tag already exists, added to selection.');
                    setShowCreateTagModal(false);
                } catch {
                    showNotification('error', 'Tag already exists but could not be retrieved.');
                }
            }
        }
    };

    const handleCreateArtist = async (data: ArtistFormData) => {
        try {
            const res = await api.post<Artist>('/artists', data);
            const newOption = { id: res.data.id, name: res.data.name, reviewStatus: res.data.reviewStatus };
            setSelectedArtists(p => p.some(a => a.id === newOption.id) ? p : [...p, newOption]);
            showNotification(isReviewMode ? 'info' : 'success', isReviewMode ? 'Artist submitted for review' : 'Artist created');
            setShowCreateArtistModal(false);
        } catch (error) {
            if (httpStatus(error) === 409) {
                try {
                    const existingRes = await api.get<Artist>(`/artists/by-name/${encodeURIComponent(data.name)}`, { skipGlobalErrorHandler: true });
                    const existing = existingRes.data;
                    const existingOpt = { id: existing.id, name: existing.name, reviewStatus: existing.reviewStatus };

                    setSelectedArtists(p => p.some(a => a.id === existingOpt.id) ? p : [...p, existingOpt]);
                    showNotification('info', 'Artist already exists, selected.');
                    setShowCreateArtistModal(false);
                } catch {
                    showNotification('error', 'Artist exists but could not be retrieved.');
                }
            }
        }
    };

    return {
        selectedTags, setSelectedTags,
        selectedArtists, setSelectedArtists,
        showCreateTagModal, setShowCreateTagModal,
        newTagName, setNewTagName,
        showCreateArtistModal, setShowCreateArtistModal,
        newArtistName, setNewArtistName,
        loadTags, loadArtists,
        handleCreateTag, handleCreateArtist,
        slugify
    };
};