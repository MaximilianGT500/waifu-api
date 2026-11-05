import { useState, useEffect, useRef, useCallback, useId } from 'react';
import { useForm } from 'react-hook-form';
import { useNavigate } from 'react-router';
import api from '../../services/api';
import SearchableSelect from '../components/SearchableSelect';
import { ImageUp, Info, Loader2, X } from 'lucide-react';
import { useNotification } from '../../context/NotificationContext';
import TagModal from '../components/modals/TagModal';
import ArtistModal from '../components/modals/ArtistModal';
import { useMetadata } from '../../hooks/useMetadata';
import { usePermissions } from '../../hooks/useBanner';
import { ImageDto, ReviewStatus, Role } from '../../types';
import { useRequireAuth } from '../../hooks/useRequireAuth';
import { getEnv } from '../../utils/env';
import ExternalLink from '../components/ExternalLink';
import { MetaTags } from '../../hooks/useMetaTags';
import { safeExternalUrl } from '../../utils/safeUrl';
import { acceptedTypes, checkImageFile, describeTypes, firstImageFile, formatBytes, ImageFileInfo } from '../../utils/imageFile';

interface UploadForm {
  source: string;
  isNsfw: boolean;
}

const Upload = () => {
  const user = useRequireAuth();
  const { register, handleSubmit, formState: { errors } } = useForm<UploadForm>({ defaultValues: { source: '', isNsfw: false } });
  const navigate = useNavigate();
  const { showNotification } = useNotification();
  const { uploadLimits } = usePermissions();

  const [file, setFile] = useState<File | null>(null);
  const [fileInfo, setFileInfo] = useState<ImageFileInfo | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);
  const [submitAttempted, setSubmitAttempted] = useState(false);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const [progress, setProgress] = useState<number | null>(null);
  const [agreedToTos, setAgreedToTos] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dragDepth = useRef(0);
  const checkSeq = useRef(0);
  const fileHintId = useId();
  const sourceId = useId();

  const tosUrl = safeExternalUrl(getEnv('VITE_TOS_URL'));
  const isReviewMode = !(user?.role === Role.Moderator || user?.role === Role.Admin);
  const types = acceptedTypes(uploadLimits);

  const {
      selectedTags, setSelectedTags,
      selectedArtists, setSelectedArtists,
      showCreateTagModal, setShowCreateTagModal,
      newTagName, setNewTagName,
      showCreateArtistModal, setShowCreateArtistModal,
      newArtistName, setNewArtistName,
      loadTags, loadArtists,
      handleCreateTag, handleCreateArtist,
      slugify
  } = useMetadata({ isReviewMode });

  useEffect(() => {
    if (!file) {
      setPreviewUrl(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  const selectFile = useCallback(async (candidate: File | null) => {
    if (!candidate) return;
    const seq = ++checkSeq.current;
    setIsChecking(true);
    const { error, info } = await checkImageFile(candidate, uploadLimits);
    if (seq !== checkSeq.current) return;
    setIsChecking(false);
    if (error) {
      setFile(null);
      setFileInfo(null);
      setFileError(error);
      return;
    }
    setFileError(null);
    setFile(candidate);
    setFileInfo(info);
  }, [uploadLimits]);

  const clearFile = () => {
    checkSeq.current++;
    setFile(null);
    setFileInfo(null);
    setFileError(null);
    setIsChecking(false);
    if (fileInputRef.current) fileInputRef.current.value = '';
  };

  useEffect(() => {
    const hasFiles = (e: DragEvent) => Array.from(e.dataTransfer?.types ?? []).includes('Files');
    const prevent = (e: DragEvent) => {
      if (hasFiles(e)) e.preventDefault();
    };
    const onPaste = (e: ClipboardEvent) => {
      if (isUploading) return;
      const target = e.target as HTMLElement | null;
      if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) return;
      const pasted = firstImageFile(e.clipboardData?.files);
      if (pasted) {
        e.preventDefault();
        selectFile(pasted);
      }
    };
    window.addEventListener('dragover', prevent);
    window.addEventListener('drop', prevent);
    window.addEventListener('paste', onPaste);
    return () => {
      window.removeEventListener('dragover', prevent);
      window.removeEventListener('drop', prevent);
      window.removeEventListener('paste', onPaste);
    };
  }, [selectFile, isUploading]);

  const onSubmit = async (data: UploadForm) => {
    setSubmitAttempted(true);
    if (!file || isChecking || isUploading) return;
    if (tosUrl && !agreedToTos) return;
    if (uploadLimits && selectedTags.length > uploadLimits.maxTags) {
      showNotification('warning', `An image can have at most ${uploadLimits.maxTags} tags.`);
      return;
    }
    if (uploadLimits && selectedArtists.length > uploadLimits.maxArtists) {
      showNotification('warning', `An image can have at most ${uploadLimits.maxArtists} artists.`);
      return;
    }

    setIsUploading(true);
    setProgress(0);
    const formData = new FormData();
    formData.append('file', file);

    selectedTags.forEach(tag => formData.append('tags', tag.slug || slugify(tag.name.trim())));
    selectedArtists.forEach(artist => formData.append('artists', String(artist.id)));

    const source = data.source.trim();
    if (source) formData.append('source', source);
    formData.append('isNsfw', String(!!data.isNsfw));

    try {
      const { data: uploadedImage } = await api.post<ImageDto>('/images/upload', formData, {
        onUploadProgress: (event) => {
          if (event.total) setProgress(Math.min(100, Math.round((event.loaded / event.total) * 100)));
        },
      });
      const pending = uploadedImage.reviewStatus === ReviewStatus.Pending;
      showNotification(pending ? 'info' : 'success', pending ? 'Upload pending review.' : 'Image uploaded!');
      navigate(pending ? '/review?type=Image' : `/images/${uploadedImage.id}`);
    } catch {
      setIsUploading(false);
      setProgress(null);
    }
  };

  if (!user) return null;

  const missingFile = submitAttempted && !file && !fileError;
  const limitsHint = [
    describeTypes(types),
    uploadLimits ? `up to ${uploadLimits.maxFileSizeMb} MB` : null,
    uploadLimits ? `at least ${uploadLimits.minWidth}×${uploadLimits.minHeight} px` : null,
  ].filter(Boolean) as string[];

  const dropZoneClasses = isDragging
      ? 'border-brand bg-brand-soft ring-4 ring-brand/15'
      : fileError || missingFile
          ? 'border-destructive/60 bg-destructive/5'
          : previewUrl
              ? 'border-transparent bg-secondary'
              : 'border-input bg-card hover:border-brand/60 hover:bg-brand-soft/40';

  return (
      <div className="page">
        <MetaTags title="Upload" description="Upload a new image to the collection." />
        <div className="page-header">
          <div>
            <h1 className="page-title">Upload Image</h1>
            <p className="page-lede">Add an illustration to the collection. Credit the artist and link the original source.</p>
          </div>
        </div>

        {isReviewMode && (
          <div className="mb-8 flex items-start gap-3 rounded-2xl bg-info/10 p-4">
            <Info className="mt-0.5 shrink-0 text-info" size={18} aria-hidden="true" />
            <div>
              <h2 className="text-sm font-bold">Uploads are reviewed</h2>
              <p className="mt-0.5 text-sm text-muted-foreground">
                New uploads may require review approval before appearing publicly.
              </p>
            </div>
          </div>
        )}

        <div className="grid grid-cols-1 items-start gap-8 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)] xl:gap-12">
          <div className="lg:sticky lg:top-24">
            <div
              onDragEnter={(e) => {
                e.preventDefault();
                dragDepth.current += 1;
                setIsDragging(true);
              }}
              onDragOver={(e) => {
                e.preventDefault();
                e.dataTransfer.dropEffect = 'copy';
              }}
              onDragLeave={() => {
                dragDepth.current = Math.max(0, dragDepth.current - 1);
                if (dragDepth.current === 0) setIsDragging(false);
              }}
              onDrop={(e) => {
                e.preventDefault();
                dragDepth.current = 0;
                setIsDragging(false);
                if (!isUploading) selectFile(firstImageFile(e.dataTransfer.files));
              }}
            >
              <label
                className={`group relative flex h-[22rem] w-full cursor-pointer flex-col items-center justify-center overflow-hidden rounded-[1.5rem] border-2 border-dashed transition-[border-color,background-color,box-shadow] duration-200 focus-within:ring-4 focus-within:ring-brand/25 sm:h-[30rem] ${dropZoneClasses}`}
              >
                {previewUrl ? (
                    <>
                      <img src={previewUrl} className="size-full object-contain p-3" alt="Preview of the selected file" />
                      <span className="absolute inset-x-3 bottom-3 rounded-xl bg-zinc-950/65 px-3 py-2 text-center text-sm font-medium text-white opacity-0 backdrop-blur-md transition-opacity group-hover:opacity-100" aria-hidden="true">
                        Click or drop to change the image
                      </span>
                    </>
                ) : (
                    <span className="flex flex-col items-center px-6 text-center">
                      <span className="mb-5 grid size-14 place-items-center rounded-2xl bg-brand-soft text-brand-ink" aria-hidden="true">
                        {isChecking
                          ? <Loader2 size={26} className="animate-spin" />
                          : <ImageUp size={26} strokeWidth={1.8} />}
                      </span>
                      <span className="text-lg font-bold text-foreground">{isDragging ? 'Drop the image here' : 'Click, drop or paste an image'}</span>
                      <span id={fileHintId} className="mt-2 max-w-sm text-sm text-muted-foreground">
                        {limitsHint.join(', ')}
                      </span>
                    </span>
                )}
                <input
                  ref={fileInputRef}
                  type="file"
                  accept={types.join(',')}
                  disabled={isUploading}
                  aria-describedby={fileHintId}
                  aria-invalid={!!fileError || missingFile}
                  onChange={(e) => {
                    selectFile(e.target.files?.[0] ?? null);
                    e.target.value = '';
                  }}
                  className="sr-only"
                />
              </label>
            </div>

            {file && (
              <div className="mt-3 flex items-center justify-between gap-3 text-sm text-muted-foreground">
                <p className="flex min-w-0 flex-wrap gap-x-3" title={file.name}>
                  <span className="truncate font-semibold text-foreground">{file.name}</span>
                  <span className="tabular">{formatBytes(file.size)}</span>
                  {fileInfo && <span className="tabular">{fileInfo.width} × {fileInfo.height}</span>}
                </p>
                <button type="button" onClick={clearFile} disabled={isUploading} className="btn btn-ghost btn-sm shrink-0">
                  <X size={14} aria-hidden="true" /> Remove
                </button>
              </div>
            )}
            {fileError && <p role="alert" className="field-error text-center">{fileError}</p>}
            {missingFile && <p role="alert" className="field-error text-center">Please choose an image.</p>}
          </div>

          <form onSubmit={handleSubmit(onSubmit, () => setSubmitAttempted(true))} className="panel space-y-6 p-5 sm:p-7" noValidate>
            <SearchableSelect
                label="Artists"
                placeholder="Search artists…"
                loadOptions={loadArtists}
                selectedOptions={selectedArtists}
                onSelect={(o) => setSelectedArtists(p => p.some(a => a.id === o.id) ? p : [...p, o])}
                onRemove={(o) => setSelectedArtists(p => p.filter(a => a.id !== o.id))}
                onCreate={(name) => { setNewArtistName(name.trim()); setShowCreateArtistModal(true); }}
                isMulti={true}
            />
            <SearchableSelect
                label="Tags"
                placeholder="Add tags…"
                loadOptions={loadTags}
                selectedOptions={selectedTags}
                onSelect={(o) => setSelectedTags(p => p.some(t => t.id === o.id) ? p : [...p, o])}
                onRemove={(o) => setSelectedTags(p => p.filter(t => t.id !== o.id))}
                onCreate={(name) => { setNewTagName(name.trim()); setShowCreateTagModal(true); }}
                isMulti={true}
            />
            <div>
              <label htmlFor={sourceId} className="field-label">Source URL</label>
              <input
                id={sourceId}
                type="url"
                inputMode="url"
                autoComplete="off"
                spellCheck={false}
                {...register('source', {
                  validate: (value) => {
                    const v = value.trim();
                    if (!v) return true;
                    if ([...v].length > 500) return 'The source URL must not exceed 500 characters.';
                    return !!safeExternalUrl(v) || 'Please enter a valid http(s) URL.';
                  },
                })}
                aria-invalid={!!errors.source}
                aria-describedby={errors.source ? `${sourceId}-error` : `${sourceId}-hint`}
                className="input"
                placeholder="https://www.pixiv.net/artworks/…"
              />
              {errors.source
                ? <p id={`${sourceId}-error`} role="alert" className="field-error">{errors.source.message}</p>
                : <p id={`${sourceId}-hint`} className="field-hint">Where the artwork was first published.</p>}
            </div>

            <div className="space-y-3">
              <label className="grid cursor-pointer grid-cols-[auto_1fr] items-start gap-x-3 rounded-xl border border-border p-4 transition-colors hover:bg-secondary/60 has-[:checked]:border-destructive/40 has-[:checked]:bg-destructive/5">
                <input type="checkbox" {...register('isNsfw')} className="row-span-2 mt-0.5 size-[1.125rem] shrink-0 accent-rose-600" />
                <span className="text-sm font-semibold">NSFW Content</span>
                <span className="col-start-2 text-sm text-muted-foreground">Contains adult material</span>
              </label>
              {tosUrl && (
                <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-border p-4 transition-colors hover:bg-secondary/60">
                  <input
                    type="checkbox"
                    checked={agreedToTos}
                    onChange={e => setAgreedToTos(e.target.checked)}
                    className="mt-0.5 size-[1.125rem] shrink-0 accent-violet-600"
                  />
                  <span className="text-sm text-muted-foreground">
                    By uploading, I agree to the{' '}
                    <ExternalLink href={tosUrl} trusted className="font-semibold text-brand-ink hover:underline">
                      Terms of Service
                    </ExternalLink>
                  </span>
                </label>
              )}
            </div>

            <div className="space-y-3">
              <button
                type="submit"
                disabled={isUploading || isChecking || (!!tosUrl && !agreedToTos)}
                aria-busy={isUploading}
                className="btn btn-primary btn-lg w-full"
              >
                {isUploading ? (
                    <>
                        <Loader2 size={18} className="animate-spin" aria-hidden="true" />
                        {progress !== null && progress < 100 ? `Uploading… ${progress}%` : 'Processing…'}
                    </>
                ) : (
                    'Upload Now'
                )}
              </button>
              {isUploading && progress !== null && (
                <progress
                  value={progress}
                  max={100}
                  aria-label="Upload progress"
                  className="block h-1.5 w-full appearance-none overflow-hidden rounded-full bg-secondary [&::-moz-progress-bar]:bg-brand [&::-webkit-progress-bar]:bg-secondary [&::-webkit-progress-value]:bg-brand [&::-webkit-progress-value]:transition-[width]"
                />
              )}
            </div>
          </form>
        </div>

        <TagModal
            isOpen={showCreateTagModal}
            onClose={() => setShowCreateTagModal(false)}
            onSubmit={handleCreateTag}
            initialData={{ name: newTagName }}
            title="New Tag"
            isReviewMode={isReviewMode}
            submitLabel="Create"
        />

        <ArtistModal
            isOpen={showCreateArtistModal}
            onClose={() => setShowCreateArtistModal(false)}
            onSubmit={handleCreateArtist}
            initialData={{ name: newArtistName }}
            title="New Artist"
            isReviewMode={isReviewMode}
            submitLabel="Create"
        />
      </div>
  );
};

export default Upload;
