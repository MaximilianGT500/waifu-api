package server

import (
	"strconv"
	"time"
)

type userMinimalDto struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatarUrl"`
}

type userDto struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	DiscordID          string  `json:"discordId"`
	AvatarURL          string  `json:"avatarUrl"`
	Role               string  `json:"role"`
	IsBlacklisted      bool    `json:"isBlacklisted"`
	BlacklistReason    *string `json:"blacklistReason"`
	RequestCount       int64   `json:"requestCount"`
	APIKeyRequestCount int64   `json:"apiKeyRequestCount"`
	JWTRequestCount    int64   `json:"jwtRequestCount"`
	UploadedImageCount int64   `json:"uploadedImageCount"`
	AlbumImageCount    int64   `json:"albumImageCount"`
}

type tagDto struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Description  string `json:"description"`
	ReviewStatus string `json:"reviewStatus"`
	CreatorID    *int64 `json:"creatorId"`
	ImageCount   int64  `json:"imageCount"`
}

type artistDto struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Patreon      string `json:"patreon"`
	Pixiv        string `json:"pixiv"`
	Twitter      string `json:"twitter"`
	DeviantArt   string `json:"deviantArt"`
	ReviewStatus string `json:"reviewStatus"`
	CreatorID    *int64 `json:"creatorId"`
	ImageCount   int64  `json:"imageCount"`
}

type artistEntity struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Patreon      *string    `json:"patreon"`
	Pixiv        *string    `json:"pixiv"`
	Twitter      *string    `json:"twitter"`
	DeviantArt   *string    `json:"deviantArt"`
	ReviewStatus string     `json:"reviewStatus"`
	CreatorID    *int64     `json:"creatorId"`
	Images       []imageDto `json:"images"`
}

type imageAlbumRef struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	UserID      int64  `json:"userId"`
	IsDefault   bool   `json:"isDefault"`
	Description string `json:"description"`
	ImageCount  int64  `json:"imageCount"`
}

type imageDto struct {
	ID             int64           `json:"id"`
	PerceptualHash string          `json:"perceptualHash"`
	Extension      string          `json:"extension"`
	DominantColor  string          `json:"dominantColor"`
	Source         *string         `json:"source"`
	Artists        []artistDto     `json:"artists"`
	UploaderID     *int64          `json:"uploaderId"`
	UploadedAt     time.Time       `json:"uploadedAt"`
	IsNsfw         bool            `json:"isNsfw"`
	IsAnimated     bool            `json:"isAnimated"`
	Width          int64           `json:"width"`
	Height         int64           `json:"height"`
	ByteSize       int64           `json:"byteSize"`
	URL            string          `json:"url"`
	Tags           []tagDto        `json:"tags"`
	ReviewStatus   string          `json:"reviewStatus"`
	Favorites      int64           `json:"favorites"`
	LikedAt        *time.Time      `json:"likedAt"`
	AddedToAlbumAt *time.Time      `json:"addedToAlbumAt"`
	Albums         []imageAlbumRef `json:"albums"`
}

type albumDto struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"isDefault"`
	UserID      int64  `json:"userId"`
	ImageCount  int64  `json:"imageCount"`
}

type reviewTaskDto struct {
	ID                int64           `json:"id"`
	Kind              string          `json:"kind"`
	SubmitterID       *int64          `json:"submitterId"`
	Submitter         *userMinimalDto `json:"submitter"`
	TargetType        string          `json:"targetType"`
	TargetID          int64           `json:"targetId"`
	Payload           map[string]any  `json:"payload"`
	Target            map[string]any  `json:"target"`
	Reason            *string         `json:"reason"`
	Status            string          `json:"status"`
	ReviewerID        *int64          `json:"reviewerId"`
	Reviewer          *userMinimalDto `json:"reviewer"`
	ReviewerNote      *string         `json:"reviewerNote"`
	ModeratorEditedAt *time.Time      `json:"moderatorEditedAt"`
	CreatedAt         time.Time       `json:"createdAt"`
	ResolvedAt        *time.Time      `json:"resolvedAt"`
}

type reportDto struct {
	ID           int64           `json:"id"`
	UserID       int64           `json:"userId"`
	User         *userMinimalDto `json:"user"`
	ImageID      *int64          `json:"imageId"`
	Image        *imageDto       `json:"image"`
	Description  string          `json:"description"`
	Status       string          `json:"status"`
	ReviewerNote *string         `json:"reviewerNote"`
	CreatedAt    time.Time       `json:"createdAt"`
}

type bannerSettingsDto struct {
	Message   string          `json:"message"`
	Variant   string          `json:"variant"`
	IsEnabled bool            `json:"isEnabled"`
	UpdatedAt *time.Time      `json:"updatedAt"`
	UpdatedBy *userMinimalDto `json:"updatedBy"`
}

type storageDiffDto struct {
	ID       *int64    `json:"id"`
	FileName string    `json:"fileName"`
	Image    *imageDto `json:"image"`
}

func reviewStatusString(s int) string {
	if s == 1 {
		return "Accepted"
	}
	return "Pending"
}

func reviewTaskKindString(k int) string {
	if k == 1 {
		return "Edit"
	}
	return "NewContent"
}

func reviewTaskStatusString(s int) string {
	switch s {
	case 1:
		return "Approved"
	case 2:
		return "Rejected"
	case 3:
		return "Cancelled"
	default:
		return "Pending"
	}
}

func reportStatusString(s int) string {
	switch s {
	case 1:
		return "Resolved"
	case 2:
		return "Rejected"
	case 3:
		return "Cancelled"
	default:
		return "Pending"
	}
}

func bannerVariantString(v int) string {
	switch v {
	case 1:
		return "Success"
	case 2:
		return "Warning"
	case 3:
		return "Danger"
	default:
		return "Info"
	}
}

func targetTypeString(t int) string {
	switch t {
	case 1:
		return "Tag"
	case 2:
		return "Artist"
	default:
		return "Image"
	}
}

func parseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
