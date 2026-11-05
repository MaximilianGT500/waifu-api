package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testKey = []byte("0123456789abcdef0123456789abcdef-test")

func TestIssueAndValidate(t *testing.T) {
	tok, err := IssueJWT(testKey, "iss", "aud", 42, "max", RoleModerator, "123456789012345678", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ValidateJWT(tok, testKey, "iss", "aud")
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != 42 || c.Role != RoleModerator || c.Name != "max" {
		t.Fatalf("unexpected claims %+v", c)
	}
}

func TestValidateRejectsTamperedAndForeignTokens(t *testing.T) {
	tok, _ := IssueJWT(testKey, "iss", "aud", 1, "u", RoleUser, "1", time.Hour)
	if _, err := ValidateJWT(tok, []byte("another-key-another-key-another-key"), "iss", "aud"); err == nil {
		t.Fatal("token signed with another key must be rejected")
	}
	if _, err := ValidateJWT(tok, testKey, "other", "aud"); err == nil {
		t.Fatal("wrong issuer must be rejected")
	}
	if _, err := ValidateJWT(tok, testKey, "iss", "other"); err == nil {
		t.Fatal("wrong audience must be rejected")
	}

	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "1", "iss": "iss", "aud": "aud", "exp": time.Now().Add(time.Hour).Unix()})
	s, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := ValidateJWT(s, testKey, "iss", "aud"); err == nil {
		t.Fatal("alg=none must be rejected")
	}

	noExp := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "1", "iss": "iss", "aud": "aud"})
	s, _ = noExp.SignedString(testKey)
	if _, err := ValidateJWT(s, testKey, "iss", "aud"); err == nil {
		t.Fatal("tokens without expiry must be rejected")
	}

	tok, _ = IssueJWT(testKey, "iss", "aud", 1, "u", RoleUser, "1", time.Millisecond)
	exp := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "1", "iss": "iss", "aud": "aud", "exp": time.Now().Add(-time.Hour).Unix()})
	s, _ = exp.SignedString(testKey)
	if _, err := ValidateJWT(s, testKey, "iss", "aud"); err == nil {
		t.Fatal("expired token must be rejected")
	}
	_ = tok
}

func TestValidateRejectsOversizedToken(t *testing.T) {
	if _, err := ValidateJWT(strings.Repeat(".", 100000), testKey, "iss", "aud"); err == nil {
		t.Fatal("oversized token must be rejected")
	}
}

func TestDiscordAvatarValidation(t *testing.T) {
	u := &DiscordUser{ID: "123456789012345678", Avatar: "a_0123456789abcdef0123456789abcdef"}
	if got := u.AvatarURL(); got == nil || !strings.HasPrefix(*got, "https://cdn.discordapp.com/avatars/123456789012345678/") {
		t.Fatalf("unexpected avatar url %v", got)
	}
}
