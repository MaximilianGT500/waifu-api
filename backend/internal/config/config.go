package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	APIBasePath string
	APIBaseURL  string
	FrontendURL string
	AppName     string
	CDNBaseURL  string
	Environment string

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURI  string
	DiscordServerURL    string

	JWTKey      string
	JWTIssuer   string
	JWTAudience string

	OpenAPISpecPath string

	S3 S3Config

	Cloudflare CloudflareConfig

	Image          ImageConfig
	Review         ReviewConfig
	Permissions    PermissionsConfig
	PageSizes      PageSizesConfig
	RateLimit      RateLimitConfig
	StorageDiff    StorageDiffConfig
	MaxUploadMB    int
	ImgTransform   ImgTransformConfig
	ContactEmail   string
	GitHubURL      string
	StatusURL      string
	DocsURL        string
	AllowedOrigins []string
}

type S3Config struct {
	AccessKey      string
	SecretKey      string
	ServiceURL     string
	Region         string
	BucketName     string
	ForcePathStyle bool
	VerifySSL      bool
}

type CloudflareConfig struct {
	ZoneID   string
	APIToken string
}

type ImageConfig struct {
	MinWidth          int
	MinHeight         int
	MaxWidth          int
	MaxHeight         int
	AnimatedMinWidth  int
	AnimatedMinHeight int
	AnimatedMaxWidth  int
	AnimatedMaxHeight int
	DefaultPageSize   int
	MaxPageSize       int
}

type ReviewConfig struct {
	RequireImageReview       bool
	RequireArtistReview      bool
	RequireTagReview         bool
	RequireEditRequestReview bool

	ImageReviewBypassMinRole      string
	TagReviewBypassMinRole        string
	ArtistReviewBypassMinRole     string
	ImageEditReviewBypassMinRole  string
	TagEditReviewBypassMinRole    string
	ArtistEditReviewBypassMinRole string

	DefaultPageSize     int
	MaxPageSize         int
	MaxOpenEditsPerUser int
	TaskRetentionDays   int
}

type PermissionsConfig struct {
	TagCreationMinRole      string
	ArtistCreationMinRole   string
	ImageUploadMinRole      string
	SubmitEditMinRole       string
	BannerManagementMinRole string
}

type PageSizesConfig struct {
	TagDefault, TagMax       int
	ArtistDefault, ArtistMax int
	AlbumDefault, AlbumMax   int
	UserDefault, UserMax     int
	ReportDefault, ReportMax int
}

type RateLimitConfig struct {
	RequestsPerMinute int
	Burst             int
}

type StorageDiffConfig struct {
	DefaultPageSize int
	MaxPageSize     int
}

type ImgTransformConfig struct {
	Enabled      bool
	Path         string
	WidthParam   string
	CardWidth    int
	PreviewWidth int
}

func Load(envPath string) (*Config, error) {
	if envPath == "" {
		for _, p := range []string{".env", "../.env", "../../.env"} {
			if _, err := os.Stat(p); err == nil {
				envPath = p
				break
			}
		}
	}
	if envPath != "" {
		_ = godotenv.Load(envPath)
	}

	c := &Config{}
	c.APIBasePath = env("API_BASE_PATH", "/")
	c.APIBaseURL = env("API_BASE_URL", "")
	c.FrontendURL = env("FRONTEND_URL", "http://localhost:5173")
	c.AppName = env("APP_NAME", "Waifu API")
	c.CDNBaseURL = env("CDN_BASE_URL", "")
	c.Environment = env("APP_ENV", "production")

	c.DBHost = env("POSTGRES_HOST", "db")
	c.DBPort = intEnv("POSTGRES_PORT", 5432)
	c.DBUser = env("POSTGRES_USER", "postgres")
	c.DBPassword = env("POSTGRES_PASSWORD", "postgres")
	c.DBName = env("POSTGRES_DB", "waifuapi")

	c.DiscordClientID = env("DISCORD_CLIENT_ID", "")
	c.DiscordClientSecret = env("DISCORD_CLIENT_SECRET", "")
	c.DiscordRedirectURI = env("DISCORD_REDIRECT_URI", c.FrontendURL+"/auth/callback")
	c.DiscordServerURL = env("DISCORD_SERVER_URL", "")

	c.JWTKey = env("JWT_KEY", "")
	c.OpenAPISpecPath = env("OPENAPI_SPEC_PATH", "openapi.json")
	c.JWTIssuer = env("JWT_ISSUER", "waifu-api")
	c.JWTAudience = env("JWT_AUDIENCE", "waifu-api-users")

	c.S3 = S3Config{
		AccessKey:      env("S3_ACCESS_KEY", ""),
		SecretKey:      env("S3_SECRET_KEY", ""),
		ServiceURL:     env("S3_SERVICE_URL", ""),
		Region:         env("S3_REGION", "auto"),
		BucketName:     env("S3_BUCKET_NAME", ""),
		ForcePathStyle: boolEnv("S3_FORCE_PATH_STYLE", true),
		VerifySSL:      boolEnv("S3_VERIFY_SSL", true),
	}

	c.Cloudflare = CloudflareConfig{
		ZoneID:   env("CLOUDFLARE_ZONE_ID", ""),
		APIToken: env("CLOUDFLARE_API_TOKEN", ""),
	}

	c.Image = ImageConfig{
		MinWidth:          intEnv("IMAGE_MIN_WIDTH", 800),
		MinHeight:         intEnv("IMAGE_MIN_HEIGHT", 1000),
		MaxWidth:          intEnv("IMAGE_MAX_WIDTH", 9000),
		MaxHeight:         intEnv("IMAGE_MAX_HEIGHT", 9000),
		AnimatedMinWidth:  intEnv("IMAGE_ANIMATED_MIN_WIDTH", 400),
		AnimatedMinHeight: intEnv("IMAGE_ANIMATED_MIN_HEIGHT", 200),
		AnimatedMaxWidth:  intEnv("IMAGE_ANIMATED_MAX_WIDTH", 2000),
		AnimatedMaxHeight: intEnv("IMAGE_ANIMATED_MAX_HEIGHT", 2000),
		DefaultPageSize:   intEnv("IMAGE_DEFAULT_PAGE_SIZE", 1),
		MaxPageSize:       intEnv("IMAGE_MAX_PAGE_SIZE", -1),
	}

	c.Review = ReviewConfig{
		RequireImageReview:            boolEnv("REVIEW_REQUIRE_IMAGE_REVIEW", true),
		RequireArtistReview:           boolEnv("REVIEW_REQUIRE_ARTIST_REVIEW", true),
		RequireTagReview:              boolEnv("REVIEW_REQUIRE_TAG_REVIEW", true),
		RequireEditRequestReview:      boolEnv("REVIEW_REQUIRE_EDIT_REQUEST_REVIEW", true),
		ImageReviewBypassMinRole:      env("REVIEW_IMAGE_REVIEW_BYPASS_MIN_ROLE", "TrustedUser"),
		TagReviewBypassMinRole:        env("REVIEW_TAG_REVIEW_BYPASS_MIN_ROLE", "TrustedUser"),
		ArtistReviewBypassMinRole:     env("REVIEW_ARTIST_REVIEW_BYPASS_MIN_ROLE", "TrustedUser"),
		ImageEditReviewBypassMinRole:  env("REVIEW_IMAGE_EDIT_REVIEW_BYPASS_MIN_ROLE", "TrustedUser"),
		TagEditReviewBypassMinRole:    env("REVIEW_TAG_EDIT_REVIEW_BYPASS_MIN_ROLE", "TrustedUser"),
		ArtistEditReviewBypassMinRole: env("REVIEW_ARTIST_EDIT_REVIEW_BYPASS_MIN_ROLE", "TrustedUser"),
		DefaultPageSize:               intEnv("REVIEW_DEFAULT_PAGE_SIZE", 30),
		MaxPageSize:                   intEnv("REVIEW_MAX_PAGE_SIZE", -1),
		MaxOpenEditsPerUser:           intEnv("REVIEW_MAX_OPEN_EDITS_PER_USER", 20),
		TaskRetentionDays:             intEnv("REVIEW_TASK_RETENTION_DAYS", 90),
	}

	c.Permissions = PermissionsConfig{
		TagCreationMinRole:      env("PERMISSIONS_TAG_CREATION_MIN_ROLE", "User"),
		ArtistCreationMinRole:   env("PERMISSIONS_ARTIST_CREATION_MIN_ROLE", "User"),
		ImageUploadMinRole:      env("PERMISSIONS_IMAGE_UPLOAD_MIN_ROLE", "User"),
		SubmitEditMinRole:       env("PERMISSIONS_SUBMIT_EDIT_MIN_ROLE", "User"),
		BannerManagementMinRole: env("PERMISSIONS_BANNER_MANAGEMENT_MIN_ROLE", "Moderator"),
	}

	c.PageSizes = PageSizesConfig{
		TagDefault:    intEnv("TAG_DEFAULT_PAGE_SIZE", 30),
		TagMax:        intEnv("TAG_MAX_PAGE_SIZE", -1),
		ArtistDefault: intEnv("ARTIST_DEFAULT_PAGE_SIZE", 30),
		ArtistMax:     intEnv("ARTIST_MAX_PAGE_SIZE", -1),
		AlbumDefault:  intEnv("ALBUM_DEFAULT_PAGE_SIZE", 30),
		AlbumMax:      intEnv("ALBUM_MAX_PAGE_SIZE", -1),
		UserDefault:   intEnv("USER_DEFAULT_PAGE_SIZE", 30),
		UserMax:       intEnv("USER_MAX_PAGE_SIZE", -1),
		ReportDefault: intEnv("REPORT_DEFAULT_PAGE_SIZE", 30),
		ReportMax:     intEnv("REPORT_MAX_PAGE_SIZE", -1),
	}

	c.RateLimit = RateLimitConfig{
		RequestsPerMinute: intEnv("RATE_LIMIT_REQUESTS_PER_MINUTE", 240),
		Burst:             intEnv("RATE_LIMIT_BURST", 40),
	}

	c.StorageDiff = StorageDiffConfig{
		DefaultPageSize: intEnv("STORAGE_DIFF_DEFAULT_PAGE_SIZE", 30),
		MaxPageSize:     intEnv("STORAGE_DIFF_MAX_PAGE_SIZE", 100),
	}

	c.MaxUploadMB = intEnv("IMAGE_MAX_UPLOAD_SIZE_MB", 10)

	c.ImgTransform = ImgTransformConfig{
		Enabled:      boolEnv("IMG_TRANSFORM_ENABLED", false),
		Path:         env("IMG_TRANSFORM_PATH", ""),
		WidthParam:   env("IMG_WIDTH_PARAM", "width"),
		CardWidth:    intEnv("IMG_CARD_WIDTH", 640),
		PreviewWidth: intEnv("IMG_PREVIEW_WIDTH", 1280),
	}

	c.ContactEmail = env("CONTACT_EMAIL", "")
	c.GitHubURL = env("GITHUB_URL", "")
	c.StatusURL = env("STATUS_URL", "")
	c.DocsURL = env("DOCS_URL", "")

	origins := []string{c.FrontendURL}
	if extra := env("CORS_EXTRA_ORIGINS", ""); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	}
	if c.Environment != "production" {
		origins = append(origins, "http://localhost:5173", "http://127.0.0.1:5173")
	}
	c.AllowedOrigins = origins

	return c, c.Validate()
}

func (c *Config) Validate() error {
	required := map[string]string{
		"API_BASE_URL":          c.APIBaseURL,
		"CDN_BASE_URL":          c.CDNBaseURL,
		"S3_ACCESS_KEY":         c.S3.AccessKey,
		"S3_SECRET_KEY":         c.S3.SecretKey,
		"S3_SERVICE_URL":        c.S3.ServiceURL,
		"S3_BUCKET_NAME":        c.S3.BucketName,
		"DISCORD_CLIENT_ID":     c.DiscordClientID,
		"DISCORD_CLIENT_SECRET": c.DiscordClientSecret,
		"DISCORD_REDIRECT_URI":  c.DiscordRedirectURI,
		"JWT_KEY":               c.JWTKey,
		"JWT_ISSUER":            c.JWTIssuer,
		"JWT_AUDIENCE":          c.JWTAudience,
	}
	var missing []string
	for k, v := range required {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	if len(c.JWTKey) < 32 {
		return fmt.Errorf("JWT_KEY must be at least 32 characters long")
	}
	return nil
}

func (c *Config) DBURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func intEnv(key string, def int) int {
	v := env(key, "")
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return def
}

func boolEnv(key string, def bool) bool {
	v := env(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
