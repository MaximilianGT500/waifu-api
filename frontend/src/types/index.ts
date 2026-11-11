export interface ImageDto {
  id: number;
  signature: string;
  extension: string;
  dominantColor: string;
  source?: string;
  artist?: Artist;
  artists: Artist[];
  uploadedAt: string;
  isNsfw: boolean;
  isAnimated: boolean;
  width: number;
  height: number;
  byteSize: number;
  url: string;
  tags: Tag[];
  albums?: AlbumDto[];
  favorites: number;
  likedAt?: string;
  uploaderId?: number;
  reviewStatus: ReviewStatus;
}

export interface Artist {
  id: number;
  name: string;
  patreon?: string;
  pixiv?: string;
  twitter?: string;
  deviantArt?: string;
  reviewStatus?: ReviewStatus;
  imageCount?: number;
  creatorId?: number;
}

export interface Tag {
  id: number;
  name: string;
  slug: string;
  description: string;
  reviewStatus?: ReviewStatus;
  imageCount?: number;
  creatorId?: number;
}
export interface User {
  id: number;
  name: string;
  role: Role;
  discordId?: string;
  avatarUrl?: string;
  isBlacklisted: boolean;
  blacklistReason?: string;
  requestCount?: number;
  apiKeyRequestCount?: number;
  jwtRequestCount?: number;
  uploadedImageCount?: number;
  albumImageCount?: number;
}

export interface AlbumDto {
  id: number;
  name: string;
  description: string;
  isDefault: boolean;
  imageCount?: number;
}

export interface ApiKeyDto {
  id: number;
  key?: string;
  description: string;
  createdAt: string;
  lastUsedAt?: string;
  expirationDate?: string;
}

export enum ReportStatus {
  Pending = 'Pending',
  Resolved = 'Resolved',
  Rejected = 'Rejected',
  Cancelled = 'Cancelled',
}

export interface Report {
  id: number;
  userId: number;
  user?: User;
  imageId?: number | null;
  image?: ImageDto;
  description?: string;
  status: ReportStatus;
  reviewerNote?: string;
  createdAt: string;
}

export enum Role {
  User = 'User',
  TrustedUser = 'TrustedUser',
  Moderator = 'Moderator',
  Admin = 'Admin'
}

export enum ReviewStatus {
  Pending = 'Pending',
  Accepted = 'Accepted',
  Rejected = 'Rejected'
}

export enum ReviewStatusFilter {
  Accepted = 'Accepted',
  Pending = 'Pending',
  All = 'All'
}

export enum NsfwMode {
  False = 'False',
  True = 'True',
  All = 'All'
}

export enum AnimatedMode {
  False = 'False',
  True = 'True',
  All = 'All'
}

export enum Orientation {
  All = 'All',
  Landscape = 'Landscape',
  Portrait = 'Portrait',
  Square = 'Square'
}

export enum ImageOrderBy {
  Random = 'Random',
  UploadedAt = 'UploadedAt',
  Favorites = 'Favorites',
  Trending = 'Trending',
  AddedToAlbum = 'AddedToAlbum'
}

export interface PaginatedList<T> {
  items: T[];
  pageNumber: number;
  totalPages: number;
  totalCount: number;
  hasPreviousPage: boolean;
  hasNextPage: boolean;
  maxPageSize: number;
  defaultPageSize: number;
}

export interface ImageFormData {
  file?: File;
  source?: string;
  isNsfw: boolean;
  tags: string[];
  artists: number[];
  userId?: number;
  reviewStatus?: ReviewStatus;
}

export interface PublicStats {
    totalRequests: number;
    totalImages: number;
    totalTags: number;
    totalArtists: number;
}

export interface ProblemDetails {
    type?: string;
    title?: string;
    status?: number;
    detail?: string;
    instance?: string;
    errors?: Record<string, string[]>;
    message?: string;
}

export enum ReviewTaskStatus {
  Pending = 'Pending',
  Approved = 'Approved',
  Rejected = 'Rejected',
  Cancelled = 'Cancelled'
}

export enum ReviewTaskKind {
  NewContent = 'NewContent',
  Edit = 'Edit'
}

export enum ReviewableContentType {
  Image = 'Image',
  Tag = 'Tag',
  Artist = 'Artist'
}

export interface UserMinimal {
  id: number;
  name: string;
  avatarUrl?: string;
}

export interface ReviewTaskTarget {
  image?: ImageDto;
  tag?: Tag;
  artist?: Artist;
}

export interface ImageEditPayload {
  source?: string;
  isNsfw?: boolean;
  addTagSlugs?: string[];
  removeTagSlugs?: string[];
  addArtistIds?: number[];
  removeArtistIds?: number[];
}

export interface TagEditPayload {
  name?: string;
  description?: string;
}

export interface ArtistEditPayload {
  name?: string;
  patreon?: string;
  pixiv?: string;
  twitter?: string;
  deviantArt?: string;
}

export type EditPayload = ImageEditPayload | TagEditPayload | ArtistEditPayload;

export interface ReviewTask {
  id: number;
  kind: ReviewTaskKind;
  submitterId?: number;
  submitter?: UserMinimal;
  targetType: ReviewableContentType;
  targetId: number;
  payload?: EditPayload;
  target?: ReviewTaskTarget;
  reason?: string;
  status: ReviewTaskStatus;
  reviewerId?: number;
  reviewer?: UserMinimal;
  reviewerNote?: string;
  moderatorEditedAt?: string;
  createdAt: string;
  resolvedAt?: string;
}

export interface SubmitEditBody {
  targetType: ReviewableContentType;
  targetId: number;
  reason?: string;
  payload: EditPayload;
}

export enum BannerVariant {
  Info = 'Info',
  Success = 'Success',
  Warning = 'Warning',
  Danger = 'Danger'
}

export interface Banner {
  message: string;
  variant: BannerVariant;
  updatedAt: string;
}

export interface BannerSettings {
  message: string;
  variant: BannerVariant;
  isEnabled: boolean;
  updatedAt?: string;
  updatedBy?: UserMinimal;
}

export interface UploadLimits {
  maxFileSizeMb: number;
  allowedContentTypes: string[];
  minWidth: number;
  minHeight: number;
  maxWidth: number;
  maxHeight: number;
  animatedMinWidth: number;
  animatedMinHeight: number;
  animatedMaxWidth: number;
  animatedMaxHeight: number;
  maxPixels: number;
  maxTags: number;
  maxArtists: number;
}

export interface UserPermissions {
  canManageBanner: boolean;
  upload?: UploadLimits;
}

export interface FacetEntry {
  id: number;
  name: string;
  slug?: string;
  count: number;
}

export interface ImageFacets {
  totalCount: number;
  tags: FacetEntry[];
  artists: FacetEntry[];
}

export interface AlbumBatchResult {
  added: number;
  removed: number;
  skipped: number[];
  imageCount: number;
}

export interface AccountSummary {
  uploads: { accepted: number; pending: number; rejected: number };
  favorites: number;
  albums: number;
  albumImages: number;
  apiKeys: number;
  reports: { pending: number; total: number };
  submissions: { pending: number; total: number };
  requests: { total: number; apiKey: number; jwt: number };
}
