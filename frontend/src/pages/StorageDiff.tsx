import { useState, useEffect, useEffectEvent } from 'react';
import api from '../services/api';
import { ImageDto, ImageFormData, PaginatedList } from '../types';
import { useNotification } from '../context/NotificationContext';
import { Trash2, Edit2, Check, FileQuestion, Database } from 'lucide-react';
import StatusTabs from '../components/StatusTabs';
import ImageModal from '../components/modals/ImageModal';
import ConfirmModal from '../components/modals/ConfirmModal';
import Pagination from '../components/Pagination';
import { useRequireAuth } from '../hooks/useRequireAuth';
import { useImageUpdate } from '../hooks/useImageUpdate';

interface StorageDiffDto {
    id: number | null;
    fileName: string;
    image: ImageDto | null;
}

type DiffMode = 'OrphansInS3' | 'MissingFromS3';

const defaultPageSize = 30;

const StorageDiff = () => {
    const user = useRequireAuth();
    const { showNotification } = useNotification();
    const { updateImage } = useImageUpdate();
    const [tab, setTab] = useState<DiffMode>('OrphansInS3');
    const [loading, setLoading] = useState(false);

    const [items, setItems] = useState<StorageDiffDto[]>([]);
    const [page, setPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);
    const [totalCount, setTotalCount] = useState(0);
    const [pageSize, setPageSize] = useState(defaultPageSize);

    const [selectedItems, setSelectedItems] = useState<Set<string>>(new Set());
    const [confirmingBulkDelete, setConfirmingBulkDelete] = useState(false);
    const [deletingSingle, setDeletingSingle] = useState<StorageDiffDto | null>(null);
    const [editingImage, setEditingImage] = useState<ImageDto | null>(null);

    const fetchData = async () => {
        if (!user) return;
        setLoading(true);
        try {
            const { data } = await api.get<PaginatedList<StorageDiffDto>>(`/storage-diff?mode=${tab}&page=${page}&pageSize=${pageSize}`);
            setItems(data.items);
            setTotalPages(data.totalPages);
            setTotalCount(data.totalCount);
            if (data.maxPageSize > 0 && pageSize > data.maxPageSize) {
                setPageSize(data.maxPageSize);
            }
        } catch {
            return;
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        setPage(1);
        setSelectedItems(new Set());
    }, [tab]);

    useEffect(() => {
        setPage(1);
        setSelectedItems(new Set());
    }, [pageSize]);

    useEffect(() => {
        setSelectedItems(new Set());
    }, [page]);

    const load = useEffectEvent(() => { fetchData(); });

    useEffect(() => {
        if (user) load();
    }, [tab, page, pageSize, user]);

    const getItemKey = (item: StorageDiffDto) =>
        tab === 'OrphansInS3' ? item.fileName : String(item.id);

    const toggleSelect = (key: string) => {
        setSelectedItems(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    };

    const toggleSelectAll = () => {
        if (selectedItems.size === items.length) {
            setSelectedItems(new Set());
        } else {
            setSelectedItems(new Set(items.map(getItemKey)));
        }
    };

    const handleBulkDelete = async () => {
        try {
            if (tab === 'OrphansInS3') {
                await api.delete('/storage-diff/s3', { data: { fileNames: Array.from(selectedItems) } });
            } else {
                await api.delete('/storage-diff/db', { data: { imageIds: Array.from(selectedItems).map(Number) } });
            }
            showNotification('success', `Deleted ${selectedItems.size} item(s)`);
            setSelectedItems(new Set());
            setConfirmingBulkDelete(false);
            fetchData();
        } catch {
            return;
        }
    };

    const handleSingleDelete = async () => {
        if (!deletingSingle) return;
        try {
            if (tab === 'OrphansInS3') {
                await api.delete('/storage-diff/s3', { data: { fileNames: [deletingSingle.fileName] } });
            } else {
                await api.delete('/storage-diff/db', { data: { imageIds: [deletingSingle.id] } });
            }
            showNotification('success', 'Item deleted');
            setDeletingSingle(null);
            fetchData();
        } catch {
            return;
        }
    };

    const handleUpdateImage = async (data: ImageFormData) => {
        if (!editingImage) return;
        try {
            await updateImage(editingImage.id, data);
            showNotification('success', 'Image updated');
            setEditingImage(null);
            fetchData();
        } catch {
            return;
        }
    };

    if (!user) return null;

    const checkbox = 'size-4 shrink-0 cursor-pointer rounded-sm accent-violet-600';
    const rowAction = 'grid size-8 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground';

    return (
        <div className="page flex h-full flex-col">
            <div className="page-header">
                <div>
                    <h1 className="page-title">Storage Diff</h1>
                    <p className="page-lede">Files and database records that no longer match. Review them before you delete anything.</p>
                </div>
                <StatusTabs
                    value={tab}
                    onChange={setTab}
                    label="Kind of discrepancy"
                    options={[{ value: 'OrphansInS3', label: 'Orphans in S3' }, { value: 'MissingFromS3', label: 'Missing from S3' }]}
                />
            </div>

            {items.length > 0 && (
                <div className="sticky top-20 z-10 mb-4 flex flex-wrap items-center gap-3 rounded-2xl border border-border bg-card/90 px-4 py-2.5 backdrop-blur-md">
                    <label className="flex cursor-pointer items-center gap-2.5">
                        <input
                            type="checkbox"
                            checked={selectedItems.size === items.length && items.length > 0}
                            onChange={toggleSelectAll}
                            className={checkbox}
                        />
                        <span className="whitespace-nowrap text-sm font-medium">
                            {selectedItems.size === 0
                                ? 'Select all on this page'
                                : selectedItems.size === items.length
                                    ? `All ${items.length} selected`
                                    : `${selectedItems.size} selected`}
                        </span>
                    </label>
                    <span className="ml-auto whitespace-nowrap text-sm text-muted-foreground tabular">
                        {totalCount} discrepanc{totalCount === 1 ? 'y' : 'ies'}
                    </span>
                    <button
                        type="button"
                        onClick={() => setConfirmingBulkDelete(true)}
                        disabled={selectedItems.size === 0}
                        className="btn btn-danger btn-sm"
                    >
                        <Trash2 size={14} aria-hidden="true" /> Delete selected
                    </button>
                </div>
            )}

            {loading ? (
                <div className="flex flex-1 items-center justify-center text-muted-foreground" role="status">Comparing storage and database…</div>
            ) : items.length === 0 ? (
                <div className="flex h-64 flex-col items-center justify-center rounded-2xl border-2 border-dashed border-border px-6 text-center text-muted-foreground">
                    <Check size={40} className="mb-3 opacity-30" aria-hidden="true" />
                    <p className="font-semibold text-foreground">Everything is in sync</p>
                    <p className="mt-1 text-sm">No discrepancies found for this check.</p>
                </div>
            ) : (
                <div className="flex-1">
                    <ul className="panel divide-y divide-border overflow-hidden">
                        {items.map(item => {
                            const key = getItemKey(item);
                            const img = item.image;
                            const selected = selectedItems.has(key);
                            const label = tab === 'OrphansInS3' ? item.fileName : `image #${item.id}`;
                            return (
                                <li key={key} className={`flex items-center gap-4 px-4 py-3 transition-colors ${selected ? 'bg-brand-soft' : 'hover:bg-secondary/40'}`}>
                                    <input
                                        type="checkbox"
                                        checked={selected}
                                        onChange={() => toggleSelect(key)}
                                        aria-label={`Select ${label}`}
                                        className={checkbox}
                                    />
                                    <span className="grid size-9 shrink-0 place-items-center rounded-2xl bg-secondary text-muted-foreground" aria-hidden="true">
                                        {tab === 'OrphansInS3' ? <FileQuestion size={17} className="text-warning" /> : <Database size={17} />}
                                    </span>
                                    {tab === 'OrphansInS3' ? (
                                        <span className="min-w-0 flex-1 truncate font-mono text-sm">{item.fileName}</span>
                                    ) : (
                                        <div className="min-w-0 flex-1">
                                            <p className="text-sm font-semibold tabular">Image #{item.id} <span className="font-normal text-muted-foreground">{img?.extension}</span></p>
                                            {img && (
                                                <p className="mt-0.5 flex flex-wrap gap-x-3 text-xs text-muted-foreground tabular">
                                                    <span>{img.width} × {img.height}</span>
                                                    <span>{(img.byteSize / 1024).toFixed(1)} KB</span>
                                                    {img.tags.length > 0 && <span className="truncate">{img.tags.slice(0, 3).map(t => t.name).join(', ')}{img.tags.length > 3 ? ` +${img.tags.length - 3}` : ''}</span>}
                                                </p>
                                            )}
                                        </div>
                                    )}
                                    <div className="flex shrink-0 gap-0.5">
                                        {img && (
                                            <button type="button" onClick={() => setEditingImage(img)} className={rowAction} title="Edit" aria-label={`Edit ${label}`}>
                                                <Edit2 size={15} aria-hidden="true" />
                                            </button>
                                        )}
                                        <button
                                            type="button"
                                            onClick={() => setDeletingSingle(item)}
                                            className={`${rowAction} hover:bg-destructive/10 hover:text-destructive`}
                                            title={tab === 'OrphansInS3' ? 'Delete from S3' : 'Delete from the database'}
                                            aria-label={`Delete ${label}`}
                                        >
                                            <Trash2 size={15} aria-hidden="true" />
                                        </button>
                                    </div>
                                </li>
                            );
                        })}
                    </ul>

                    <Pagination
                        currentPage={page}
                        totalPages={totalPages}
                        setPage={setPage}
                        pageSize={pageSize}
                        setPageSize={setPageSize}
                    />
                </div>
            )}

            {editingImage && (
                <ImageModal
                    isOpen={!!editingImage}
                    onClose={() => setEditingImage(null)}
                    onSubmit={handleUpdateImage}
                    initialData={editingImage}
                />
            )}

            <ConfirmModal
                isOpen={!!deletingSingle}
                onClose={() => setDeletingSingle(null)}
                onConfirm={handleSingleDelete}
                title={tab === 'OrphansInS3' ? 'Delete S3 File' : 'Delete DB Record'}
                message={
                    tab === 'OrphansInS3'
                        ? `Delete "${deletingSingle?.fileName}" from S3? This cannot be undone.`
                        : `Delete image #${deletingSingle?.id} from the database? This cannot be undone.`
                }
            />

            <ConfirmModal
                isOpen={confirmingBulkDelete}
                onClose={() => setConfirmingBulkDelete(false)}
                onConfirm={handleBulkDelete}
                title={`Delete ${selectedItems.size} item(s)`}
                message={
                    tab === 'OrphansInS3'
                        ? `Delete ${selectedItems.size} file(s) from S3? This cannot be undone.`
                        : `Delete ${selectedItems.size} record(s) from the database? This cannot be undone.`
                }
            />
        </div>
    );
};

export default StorageDiff;
