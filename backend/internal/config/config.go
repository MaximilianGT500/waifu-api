package config

import (
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIBasePath string
	APIBaseURL  string
	FrontendURL string
	AppName     string
	CDNBaseURL  string
	Environment string

	DBHost             string
	DBPort             int
	DBUser             string
	DBPassword         string
	DBName             string
	DBSSLMode          string
	DBMaxConns         int
	DBStatementTimeout time.Duration

	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURI  string
	DiscordServerURL    string

	JWTKey      string
	JWTIssuer   string
	JWTAudience string
	JWTLifetime time.Duration

	OpenAPISpecPath string

	S3 S3Config

	Cloudflare CloudflareConfig

	Image          ImageConfig
	Review         ReviewConfig
	Permissions    PermissionsConfig
	PageSizes      PageSizesConfig
	RateLimit      RateLimitConfig
	StorageDiff    StorageDiffConfig
	Limits         LimitsConfig
	Network        NetworkConfig
	MaxUploadMB    int
	ImgTransform   ImgTransformConfig
	ContactEmail   string
	GitHubURL      string
	StatusURL      string
	DocsURL        string
	AllowedOrigins []string
	CORSAllowAll   bool
	Cluster        ClusterConfig
	Cache          CacheConfig
}

type CacheConfig struct {
	PublicTTL time.Duration
}

type ClusterConfig struct {
	InstanceID      string
	RedisURL        string
	RedisPrefix     string
	RedisTimeout    time.Duration
	LockConnections int
	DrainDelay      time.Duration
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

	DefaultPageSize              int
	MaxPageSize                  int
	MaxOpenEditsPerUser          int
	MaxPendingSubmissionsPerUser int
	TaskRetentionDays            int
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

	AuthRequestsPerMinute   int
	AuthBurst               int
	UploadRequestsPerMinute int
	UploadBurst             int

	MaxTrackedClients int
}

type StorageDiffConfig struct {
	DefaultPageSize int
	MaxPageSize     int
}

type LimitsConfig struct {
	MaxPageSize                int
	MaxFilterValues            int
	MaxTagsPerImage            int
	MaxArtistsPerImage         int
	MaxAPIKeysPerUser          int
	MaxAlbumsPerUser           int
	MaxPendingReportsPerUser   int
	ImageProcessingConcurrency int
	MaxImagePixels             int64
	MaxDecodeBytes             int64
}

type NetworkConfig struct {
	TrustedProxies []netip.Prefix
	RealIPHeader   string
	HSTSMaxAge     int
}

type ImgTransformConfig struct {
	Enabled      bool
	Path         string
	WidthParam   string
	CardWidth    int
	PreviewWidth int
}

var knownPlaceholderJWTKeys = []string{
	"change-me-to-a-long-random-string-at-least-32-chars",
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
	c.FrontendURL = strings.TrimRight(env("FRONTEND_URL", "http://localhost:5173"), "/")
	c.AppName = env("APP_NAME", "Waifu API")
	c.CDNBaseURL = env("CDN_BASE_URL", "")
	c.Environment = env("APP_ENV", "production")

	c.DBHost = env("POSTGRES_HOST", "db")
	c.DBPort = intEnv("POSTGRES_PORT", 5432)
	c.DBUser = env("POSTGRES_USER", "postgres")
	c.DBPassword = env("POSTGRES_PASSWORD", "postgres")
	c.DBName = env("POSTGRES_DB", "waifuapi")
	c.DBSSLMode = env("POSTGRES_SSLMODE", "disable")
	c.DBMaxConns = intEnv("POSTGRES_MAX_CONNS", 20)
	c.DBStatementTimeout = time.Duration(intEnv("POSTGRES_STATEMENT_TIMEOUT_MS", 30000)) * time.Millisecond

	c.DiscordClientID = env("DISCORD_CLIENT_ID", "")
	c.DiscordClientSecret = env("DISCORD_CLIENT_SECRET", "")
	c.DiscordRedirectURI = env("DISCORD_REDIRECT_URI", c.FrontendURL+"/auth/callback")
	c.DiscordServerURL = env("DISCORD_SERVER_URL", "")

	c.JWTKey = env("JWT_KEY", "")
	c.OpenAPISpecPath = env("OPENAPI_SPEC_PATH", "openapi.json")
	c.JWTIssuer = env("JWT_ISSUER", "waifu-api")
	c.JWTAudience = env("JWT_AUDIENCE", "waifu-api-users")
	c.JWTLifetime = time.Duration(intEnv("JWT_LIFETIME_HOURS", 168)) * time.Hour

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
		MaxPendingSubmissionsPerUser:  intEnv("REVIEW_MAX_PENDING_SUBMISSIONS_PER_USER", 50),
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
		RequestsPerMinute:       intEnv("RATE_LIMIT_REQUESTS_PER_MINUTE", 240),
		Burst:                   intEnv("RATE_LIMIT_BURST", 40),
		AuthRequestsPerMinute:   intEnv("RATE_LIMIT_AUTH_REQUESTS_PER_MINUTE", 10),
		AuthBurst:               intEnv("RATE_LIMIT_AUTH_BURST", 5),
		UploadRequestsPerMinute: intEnv("RATE_LIMIT_UPLOAD_REQUESTS_PER_MINUTE", 20),
		UploadBurst:             intEnv("RATE_LIMIT_UPLOAD_BURST", 10),
		MaxTrackedClients:       intEnv("RATE_LIMIT_MAX_TRACKED_CLIENTS", 100000),
	}

	c.StorageDiff = StorageDiffConfig{
		DefaultPageSize: intEnv("STORAGE_DIFF_DEFAULT_PAGE_SIZE", 30),
		MaxPageSize:     intEnv("STORAGE_DIFF_MAX_PAGE_SIZE", 100),
	}

	c.MaxUploadMB = intEnv("IMAGE_MAX_UPLOAD_SIZE_MB", 10)
	if c.MaxUploadMB <= 0 {
		c.MaxUploadMB = 10
	}

	c.Limits = LimitsConfig{
		MaxPageSize:                positive(intEnv("MAX_PAGE_SIZE_LIMIT", 500), 500),
		MaxFilterValues:            positive(intEnv("MAX_FILTER_VALUES", 50), 50),
		MaxTagsPerImage:            positive(intEnv("IMAGE_MAX_TAGS", 100), 100),
		MaxArtistsPerImage:         positive(intEnv("IMAGE_MAX_ARTISTS", 25), 25),
		MaxAPIKeysPerUser:          positive(intEnv("API_KEY_MAX_PER_USER", 25), 25),
		MaxAlbumsPerUser:           positive(intEnv("ALBUM_MAX_PER_USER", 200), 200),
		MaxPendingReportsPerUser:   intEnv("REPORT_MAX_PENDING_PER_USER", 50),
		ImageProcessingConcurrency: positive(intEnv("IMAGE_PROCESSING_CONCURRENCY", 2), 2),
		MaxImagePixels:             int64(positive(intEnv("IMAGE_MAX_PIXELS", 0), 0)),
		MaxDecodeBytes:             int64(positive(intEnv("IMAGE_MAX_DECODE_MB", 512), 512)) << 20,
	}
	if c.Limits.MaxImagePixels == 0 {
		px := int64(c.Image.MaxWidth) * int64(c.Image.MaxHeight)
		if px <= 0 || px > 81_000_000 {
			px = 81_000_000
		}
		c.Limits.MaxImagePixels = px
	}

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

	origins := []string{normalizeOrigin(c.FrontendURL)}
	if extra := env("CORS_EXTRA_ORIGINS", ""); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if o = normalizeOrigin(o); o != "" {
				origins = append(origins, o)
			}
		}
	}
	if c.Environment != "production" {
		origins = append(origins, "http://localhost:5173", "http://127.0.0.1:5173")
	}
	c.AllowedOrigins = origins
	c.CORSAllowAll = boolEnv("CORS_ALLOW_ALL", false)

	trusted, err := parseTrustedProxies(env("TRUSTED_PROXIES", defaultTrustedProxies))
	if err != nil {
		return nil, err
	}
	c.Network = NetworkConfig{
		TrustedProxies: trusted,
		RealIPHeader:   env("REAL_IP_HEADER", ""),
		HSTSMaxAge:     intEnv("HSTS_MAX_AGE", 31536000),
	}

	redisURL := env("REDIS_URL", "")
	if redisURL == "" {
		redisURL = env("VALKEY_URL", "")
	}
	c.Cluster = ClusterConfig{
		InstanceID:      instanceID(env("INSTANCE_ID", "")),
		RedisURL:        redisURL,
		RedisPrefix:     env("REDIS_KEY_PREFIX", "waifu:"),
		RedisTimeout:    time.Duration(positive(intEnv("REDIS_TIMEOUT_MS", 150), 150)) * time.Millisecond,
		LockConnections: max(intEnv("CLUSTER_LOCK_CONNECTIONS", 4), 2),
		DrainDelay:      time.Duration(max(intEnv("SHUTDOWN_DRAIN_DELAY_SECONDS", 3), 0)) * time.Second,
	}

	c.Cache = CacheConfig{
		PublicTTL: time.Duration(max(intEnv("PUBLIC_CACHE_TTL_SECONDS", 30), 0)) * time.Second,
	}

	return c, c.Validate()
}

var instanceIDChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func instanceID(v string) string {
	if v == "" {
		v, _ = os.Hostname()
	}
	v = instanceIDChars.ReplaceAllString(v, "-")
	if len(v) > 40 {
		v = v[:40]
	}
	if v == "" {
		v = "api"
	}
	return v
}

const defaultTrustedProxies = "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"

func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.EqualFold(part, "none") {
			return nil, nil
		}
		if strings.Contains(part, "/") {
			p, err := netip.ParsePrefix(part)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: invalid CIDR %q: %w", part, err)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: invalid address %q: %w", part, err)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func normalizeOrigin(o string) string {
	o = strings.TrimSpace(o)
	o = strings.TrimRight(o, "/")
	if o == "" {
		return ""
	}
	u, err := url.Parse(o)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.ToLower(o)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
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
	lowerKey := strings.ToLower(c.JWTKey)
	for _, p := range knownPlaceholderJWTKeys {
		if lowerKey == p {
			return fmt.Errorf("JWT_KEY still contains the example placeholder; generate a random secret (e.g. `openssl rand -base64 48`)")
		}
	}
	if strings.HasPrefix(lowerKey, "change-me") || strings.HasPrefix(lowerKey, "changeme") {
		return fmt.Errorf("JWT_KEY looks like a placeholder; generate a random secret (e.g. `openssl rand -base64 48`)")
	}
	if c.JWTLifetime <= 0 {
		return fmt.Errorf("JWT_LIFETIME_HOURS must be positive")
	}
	for name, raw := range map[string]string{
		"API_BASE_URL":         c.APIBaseURL,
		"CDN_BASE_URL":         c.CDNBaseURL,
		"FRONTEND_URL":         c.FrontendURL,
		"S3_SERVICE_URL":       c.S3.ServiceURL,
		"DISCORD_REDIRECT_URI": c.DiscordRedirectURI,
	} {
		if !isHTTPURL(raw) {
			return fmt.Errorf("%s must be an absolute http(s) URL", name)
		}
	}
	switch c.DBSSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("POSTGRES_SSLMODE has an invalid value %q", c.DBSSLMode)
	}
	if c.DBMaxConns < 4 {
		c.DBMaxConns = 4
	}
	if c.Cluster.RedisURL != "" {
		u, err := url.Parse(c.Cluster.RedisURL)
		if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss" && u.Scheme != "valkey" && u.Scheme != "valkeys") || u.Hostname() == "" {
			return fmt.Errorf("REDIS_URL must look like redis://user:password@host:6379/0 (rediss:// for TLS)")
		}
		pass, _ := u.User.Password()
		if p := strings.ToLower(pass); strings.HasPrefix(p, "change-me") || strings.HasPrefix(p, "changeme") {
			return fmt.Errorf("the Valkey/Redis password is still the example placeholder; set VALKEY_PASSWORD (e.g. `openssl rand -hex 24`)")
		}
	}
	return nil
}

func (c *Config) Warnings() []string {
	var w []string
	if c.Environment == "production" {
		if c.DBPassword == "postgres" {
			w = append(w, "POSTGRES_PASSWORD uses the default value 'postgres'")
		}
		if c.CORSAllowAll {
			w = append(w, "CORS_ALLOW_ALL=true: every origin may call the API")
		}
	}
	if !c.S3.VerifySSL {
		w = append(w, "S3_VERIFY_SSL=false: TLS certificates of the storage endpoint are not verified")
	}
	if len(c.Network.TrustedProxies) == 0 && c.Network.RealIPHeader != "" {
		w = append(w, "REAL_IP_HEADER is set but TRUSTED_PROXIES is empty; the header is ignored")
	}
	if c.Cluster.RedisURL != "" {
		if u, err := url.Parse(c.Cluster.RedisURL); err == nil {
			pass, _ := u.User.Password()
			if u.User == nil || (pass == "" && u.User.Username() == "") {
				w = append(w, "REDIS_URL has no password; protect the rate limit store with an ACL user")
			}
		}
	}
	return w
}

func (c *Config) LogSummary(log *slog.Logger) {
	proxies := make([]string, 0, len(c.Network.TrustedProxies))
	for _, p := range c.Network.TrustedProxies {
		proxies = append(proxies, p.String())
	}
	log.Info("security settings",
		"trustedProxies", strings.Join(proxies, ","),
		"realIpHeader", c.Network.RealIPHeader,
		"corsOrigins", strings.Join(c.AllowedOrigins, ","),
		"corsAllowAll", c.CORSAllowAll,
		"rateLimitPerMinute", c.RateLimit.RequestsPerMinute,
		"maxPageSize", c.Limits.MaxPageSize,
		"jwtLifetime", c.JWTLifetime.String())
	redisHost := ""
	if u, err := url.Parse(c.Cluster.RedisURL); err == nil && c.Cluster.RedisURL != "" {
		redisHost = u.Host
	}
	log.Info("cluster settings",
		"instance", c.Cluster.InstanceID,
		"sharedRateLimits", redisHost != "",
		"redisHost", redisHost,
		"lockConnections", c.Cluster.LockConnections,
		"drainDelay", c.Cluster.DrainDelay.String(),
		"publicCacheTTL", c.Cache.PublicTTL.String())
	for _, w := range c.Warnings() {
		log.Warn("configuration warning", "detail", w)
	}
}

func (c *Config) DBURL() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.DBUser, c.DBPassword),
		Host:   fmt.Sprintf("%s:%d", c.DBHost, c.DBPort),
		Path:   "/" + c.DBName,
	}
	q := url.Values{}
	q.Set("sslmode", c.DBSSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func positive(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
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
