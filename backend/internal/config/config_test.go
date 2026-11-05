package config

import (
	"strings"
	"testing"
)

func validConfig() *Config {
	return &Config{
		APIBaseURL:          "https://api.example.com",
		CDNBaseURL:          "https://cdn.example.com",
		FrontendURL:         "https://example.com",
		DBSSLMode:           "disable",
		DBMaxConns:          20,
		DiscordClientID:     "id",
		DiscordClientSecret: "secret",
		DiscordRedirectURI:  "https://example.com/auth/callback",
		JWTKey:              "k4r8Vq0w2eTzYb7Nn1Lm5Pp9Ss3Dd6Ff0Gg2Hh4Jj",
		JWTIssuer:           "iss",
		JWTAudience:         "aud",
		JWTLifetime:         1,
		S3: S3Config{
			AccessKey: "a", SecretKey: "b", ServiceURL: "https://s3.example.com", BucketName: "bucket",
		},
	}
}

func TestValidateAcceptsSaneConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsPlaceholderJWTKey(t *testing.T) {
	c := validConfig()
	c.JWTKey = "change-me-to-a-long-random-string-at-least-32-chars"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("expected placeholder rejection, got %v", err)
	}
	c.JWTKey = "short"
	if err := c.Validate(); err == nil {
		t.Fatal("short keys must be rejected")
	}
}

func TestValidateRejectsPlaceholderValkeyPassword(t *testing.T) {
	c := validConfig()
	c.Cluster.RedisURL = "redis://waifu:change-me-generate-with-openssl-rand-hex-24@valkey:6379/0"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "VALKEY_PASSWORD") {
		t.Fatalf("expected placeholder rejection, got %v", err)
	}
	c.Cluster.RedisURL = "redis://waifu:9f3c1a7e5b2d4f60a8c1e3b5d7f9a2c4e6b8d0f1a3c5e7b9@valkey:6379/0"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsBadURLs(t *testing.T) {
	c := validConfig()
	c.CDNBaseURL = "javascript:alert(1)"
	if err := c.Validate(); err == nil {
		t.Fatal("non-http CDN URL must be rejected")
	}
}

func TestDBURLEscapesCredentials(t *testing.T) {
	c := validConfig()
	c.DBUser = "user@x"
	c.DBPassword = "p@ss:w/rd?#"
	c.DBHost = "db"
	c.DBPort = 5432
	c.DBName = "waifu"
	u := c.DBURL()
	if strings.Count(u, "@") != 1 || !strings.Contains(u, "sslmode=disable") {
		t.Fatalf("credentials not escaped: %s", u)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	p, err := parseTrustedProxies("10.0.0.0/8, 192.168.1.1, ::1")
	if err != nil || len(p) != 3 {
		t.Fatalf("unexpected result %v %v", p, err)
	}
	if _, err := parseTrustedProxies("not-an-ip"); err == nil {
		t.Fatal("invalid entries must fail")
	}
	if p, _ := parseTrustedProxies("none"); len(p) != 0 {
		t.Fatal("'none' must disable trusted proxies")
	}
}
